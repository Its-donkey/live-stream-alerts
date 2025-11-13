package httpserver

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

// Config describes how the HTTP server should be initialised.
type Config struct {
	Addr        string
	Port        string
	ReadTimeout time.Duration
	Logger      logging.Logger
	Handler     http.Handler
}

// Server wraps the configured http.Server alongside its listener for shutdown handling.
type Server struct {
	Config
	listener   net.Listener
	httpServer *http.Server
}

// New builds a Server from the supplied configuration.
func New(config Config) (*Server, error) {
	if config.Logger == nil {
		config.Logger = logging.New()
	}
	if config.Addr == "" {
		config.Addr = defaultAddr
	}
	if config.Port == "" {
		return nil, errors.New("no valid port must be set")
	}
	if config.ReadTimeout <= 0 {
		config.ReadTimeout = defaultReadTimeout
	}

	srv := &Server{Config: config}
	handler := config.Handler
	if handler == nil {
		handler = http.HandlerFunc(srv.defaultHandler)
	}

	srv.httpServer = &http.Server{
		ReadHeaderTimeout: config.ReadTimeout,
		Handler:           handler,
		ErrorLog:          logging.AsStdLogger(config.Logger),
	}
	return srv, nil
}

// ListenAndServe starts the HTTP server with the configured address and port.
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

// Close stops the underlying http.Server.
func (s *Server) Close() error {
	if s.httpServer != nil {
		return s.httpServer.Close()
	}
	return nil
}

func (s *Server) defaultHandler(w http.ResponseWriter, r *http.Request) {
	dump, err := httputil.DumpRequest(r, true)
	if err != nil {
		s.Logger.Printf("dump request from %s: %v", r.RemoteAddr, err)
	} else {
		s.Logger.Printf("\n---- Incoming request from %s ----\n%s\n", r.RemoteAddr, dump)
	}

	if youtubehandlers.HandleSubscriptionConfirmation(w, r, youtubehandlers.SubscriptionConfirmationOptions{
		Logger: s.Logger,
	}) {
		return
	}

	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("OK"))
}
