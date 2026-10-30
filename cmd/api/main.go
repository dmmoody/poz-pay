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
	"github.com/dmmoody/poz-pay/internal/broker/rabbitmq"
	"github.com/dmmoody/poz-pay/internal/checkpay"
	"github.com/dmmoody/poz-pay/internal/outbox"
	"github.com/dmmoody/poz-pay/internal/registerupload"
	"github.com/dmmoody/poz-pay/internal/store/postgres"
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
	db, err := postgres.Open(ctx, dbURL)
	if err != nil {
		log.Fatalf("database: %v", err)
	}
	defer db.Close()

	// RabbitMQ is the broker in this process. Publish and Listen are the seam
	// a different bus (for example Google Pub/Sub) would implement instead.
	broker, err := rabbitmq.New(rabbitURL, rabbitmq.Topology{
		Exchange:    "pozpay.events",
		Queue:       "pozpay.check-register.ingest",
		RoutingKeys: []string{registerupload.TopicUploadAccepted},
	})
	if err != nil {
		log.Fatalf("rabbitmq: %v", err)
	}
	defer broker.Close()

	tenants := postgres.NewTenantStore(db)
	registers := postgres.NewCheckRegisterStore(db)
	checkSettings := postgres.NewCheckSettingsStore(db)
	audits := postgres.NewAuditStore(db)
	checks := checkpay.NewService(tenants, registers, registers, checkSettings, audits)
	uploads := registerupload.NewService(tenants, postgres.NewRegisterRecords(db), maxUpload)
	relay := outbox.NewRelay(postgres.NewOutboxStore(db), broker, time.Second)

	workerCtx, stopWorkers := context.WithCancel(context.Background())
	defer stopWorkers()
	go func() {
		if err := registerupload.Consume(workerCtx, broker, uploads); err != nil && !errors.Is(err, context.Canceled) {
			log.Fatalf("ingest: %v", err)
		}
	}()
	go relay.Run(workerCtx)

	srv := api.NewServer(addr, &api.Handlers{
		Checks:         checks,
		Uploads:        uploads,
		DB:             db,
		MaxUploadBytes: maxUpload,
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
