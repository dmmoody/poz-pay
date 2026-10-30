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
func NewServer(addr string, routes Routes) *Server {
	return &Server{
		httpServer: &http.Server{
			Addr:              addr,
			Handler:           NewRouter(routes),
			ReadHeaderTimeout: 5 * time.Second,
			// Large CSV bodies are capped by the handler. This bounds a stalled client.
			ReadTimeout:  5 * time.Minute,
			WriteTimeout: time.Minute,
			IdleTimeout:  2 * time.Minute,
		},
	}
}

func (s *Server) ListenAndServe() error {
	return s.httpServer.ListenAndServe()
}

func (s *Server) Shutdown(ctx context.Context) error {
	return s.httpServer.Shutdown(ctx)
}
