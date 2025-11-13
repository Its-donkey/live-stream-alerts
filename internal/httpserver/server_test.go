package httpserver

import (
	"io"
	"net/http"
	"testing"
	"time"
)

type testLogger struct {
	logs []string
}

func (l *testLogger) Printf(format string, args ...any) {
	l.logs = append(l.logs, format)
}

func TestNewAppliesDefaults(t *testing.T) {
	srv, err := New(Config{Port: ":8080"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if srv.httpServer == nil {
		t.Fatalf("expected underlying http server to be configured")
	}
	if srv.httpServer.Handler == nil {
		t.Fatalf("expected default handler to be applied")
	}
}

func TestNewRequiresPort(t *testing.T) {
	if _, err := New(Config{}); err == nil {
		t.Fatalf("expected error when port missing")
	}
}

func TestListenAndServeWithDefaultHandler(t *testing.T) {
	logger := &testLogger{}
	srv, err := New(Config{Port: ":0", Logger: logger})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	done := make(chan error, 1)
	go func() { done <- srv.ListenAndServe() }()

	deadline := time.Now().Add(2 * time.Second)
	for srv.listener == nil {
		if time.Now().After(deadline) {
			t.Fatal("server failed to start")
		}
		time.Sleep(10 * time.Millisecond)
	}

	url := "http://" + srv.listener.Addr().String()
	resp, err := http.Get(url)
	if err != nil {
		t.Fatalf("failed to query server: %v", err)
	}
	body, err := io.ReadAll(resp.Body)
	resp.Body.Close()
	if err != nil {
		t.Fatalf("read response: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	if string(body) != "OK" {
		t.Fatalf("unexpected body %q", string(body))
	}

	if err := srv.Close(); err != nil {
		t.Fatalf("close server: %v", err)
	}
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("listen returned error: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("server did not shut down")
	}
}
