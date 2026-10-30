package main

import (
	"context"
	"errors"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"github.com/dmmoody/poz-pay/internal/api"
	payhttp "github.com/dmmoody/poz-pay/internal/api/checkpay"
	"github.com/dmmoody/poz-pay/internal/api/health"
	reghttp "github.com/dmmoody/poz-pay/internal/api/registerupload"
	"github.com/dmmoody/poz-pay/internal/broker"
	"github.com/dmmoody/poz-pay/internal/checkpay"
	"github.com/dmmoody/poz-pay/internal/outbox"
	"github.com/dmmoody/poz-pay/internal/registerupload"
	"github.com/dmmoody/poz-pay/internal/store"
)

func main() {
	addr := envOr("API_ADDR", ":8080")
	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		log.Fatal("DATABASE_URL is required")
	}
	rabbitURL := os.Getenv("RABBITMQ_URL")
	if rabbitURL == "" {
		log.Fatal("RABBITMQ_URL is required")
	}
	maxUpload := envInt64("CHECK_REGISTER_MAX_UPLOAD_BYTES", registerupload.DefaultMaxUploadBytes)

	ctx := context.Background()
	db, err := store.Open(ctx, dbURL)
	if err != nil {
		log.Fatalf("database: %v", err)
	}
	defer db.Close()

	// RabbitMQ is the broker in this process.
	broker, err := broker.New(rabbitURL, broker.Topology{
		Exchange:    "pozpay.events",
		Queue:       "pozpay.check-register.ingest",
		RoutingKeys: []string{registerupload.TopicUploadAccepted},
	})
	if err != nil {
		log.Fatalf("rabbitmq: %v", err)
	}
	defer broker.Close()

	tenants := store.NewTenants(db)
	registers := store.NewCheckRegister(db)
	checkSettings := store.NewCheckSettings(db)
	audits := store.NewCheckAudit(db)
	checks := checkpay.NewService(tenants, registers, checkSettings, audits)
	uploads := registerupload.NewService(tenants, store.NewRegisterUploads(db), maxUpload)
	relay := outbox.NewRelay(store.NewOutbox(db), broker, time.Second)

	workerCtx, stopWorkers := context.WithCancel(context.Background())
	defer stopWorkers()
	go func() {
		if err := registerupload.Consume(workerCtx, broker, uploads); err != nil && !errors.Is(err, context.Canceled) {
			log.Fatalf("ingest: %v", err)
		}
	}()
	go relay.Run(workerCtx)

	srv := api.NewServer(addr, api.Routes{
		Health: health.Handler{DB: db},
		Checks: payhttp.Handler{Checks: checks},
		Registers: reghttp.Handler{
			Checks:         checks,
			Uploads:        uploads,
			MaxUploadBytes: maxUpload,
		},
	})

	go func() {
		log.Printf("listening on %s", addr)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("listen: %v", err)
		}
	}()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)
	<-stop

	stopWorkers()
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		log.Printf("shutdown: %v", err)
	}
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func envInt64(key string, fallback int64) int64 {
	v := os.Getenv(key)
	if v == "" {
		return fallback
	}
	n, err := strconv.ParseInt(v, 10, 64)
	if err != nil || n <= 0 {
		log.Fatalf("%s must be a positive integer", key)
	}
	return n
}
