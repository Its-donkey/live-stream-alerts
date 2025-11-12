// file name — /internal/server/server.go
package server

import (
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/http/httputil"
	"strings"
	"time"

	"live-stream-alerts/internal/logging"
	youtubehandlers "live-stream-alerts/internal/platforms/youtube/handlers"
)

const (
	defaultAddr        = "127.0.0.1"
	defaultReadTimeout = 10 * time.Second
)

type Config struct {
	Addr        string
	Port        string
	ReadTimeout time.Duration
	Logger      logging.Logger
	Handler     http.Handler
}

type Server struct {
	Config
	listener   net.Listener
	httpServer *http.Server
}

func (c Config) New() (*Server, error) {
	if c.Logger == nil {
		c.Logger = logging.New()
	}
	if c.Addr == "" {
		c.Addr = defaultAddr
	}
	if c.Port == "" {
		return nil, errors.New("no valid port must be set")
	}
	if c.ReadTimeout <= 0 {
		c.ReadTimeout = defaultReadTimeout
	}
	s := &Server{Config: c}
	handler := c.Handler
	if handler == nil {
		handler = http.HandlerFunc(s.handleHTTP)
	}
	s.httpServer = &http.Server{
		ReadHeaderTimeout: c.ReadTimeout,
		Handler:           handler,
		ErrorLog:          logging.AsStdLogger(c.Logger),
	}
	return s, nil
}

func (s *Server) ListenAndServe() error {
	addr := s.listenAddr()
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return fmt.Errorf("listen on %s: %w", addr, err)
	}
	s.listener = ln
	s.Logger.Printf("Listening on %s", ln.Addr())

	if err := s.httpServer.Serve(ln); err != nil && !errors.Is(err, http.ErrServerClosed) {
		return fmt.Errorf("serve: %w", err)
	}
	return nil
}

func (s *Server) listenAddr() string {
	port := strings.TrimPrefix(s.Port, ":")
	if port == "" {
		return s.Addr
	}
	return net.JoinHostPort(s.Addr, port)
}

func (s *Server) Close() error {
	if s.httpServer != nil {
		return s.httpServer.Close()
	}
	return nil
}

func (s *Server) handleHTTP(w http.ResponseWriter, r *http.Request) {
	dump, err := httputil.DumpRequest(r, true)
	if err != nil {
		s.Logger.Printf("dump request from %s: %v", r.RemoteAddr, err)
	} else {
		s.Logger.Printf("\n---- Incoming request from %s ----\n%s\n", r.RemoteAddr, dump)
	}

	if youtubehandlers.YouTubeSubscriptionConfirmation(w, r, s.Logger) {
		return
	}

	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("OK"))
}
