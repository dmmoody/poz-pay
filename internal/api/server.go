package api

import (
	"context"
	"net/http"
	"time"
)

// Server owns the HTTP listener lifecycle.
type Server struct {
	httpServer *http.Server
}

// NewServer wires the router and timeouts. addr example: ":8080".
func NewServer(addr string, handlers *Handlers) *Server {
	return &Server{
		httpServer: &http.Server{
			Addr:              addr,
			Handler:           NewRouter(handlers),
			ReadHeaderTimeout: 5 * time.Second,
		},
	}
}

func (s *Server) ListenAndServe() error {
	return s.httpServer.ListenAndServe()
}

func (s *Server) Shutdown(ctx context.Context) error {
	return s.httpServer.Shutdown(ctx)
}
