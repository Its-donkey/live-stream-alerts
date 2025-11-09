package v1

import (
	"net/http"
	"net/http/httputil"

	"live-stream-alerts/internal/youtube"
)

func New(logger youtube.Logger) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/alerts", func(w http.ResponseWriter, r *http.Request) {
		youtube.HandleAlertsVerification(w, r, logger)
	})

	mux.Handle("/api/V1/youtube/subscribe", youtube.NewSubscribeHandler(youtube.SubscribeOptions{
		Logger: logger,
	}))

	if logger == nil {
		return mux
	}

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if dump, err := httputil.DumpRequest(r, true); err == nil {
			logger.Printf("---- Incoming request from %s ----\n%s", r.RemoteAddr, dump)
		} else {
			logger.Printf("failed to dump request from %s: %v", r.RemoteAddr, err)
		}
		mux.ServeHTTP(w, r)
	})
}
