package api

import "net/http"

// PlaceholderHandler is a stub for future Facebook endpoints.
func PlaceholderHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotImplemented)
		_, _ = w.Write([]byte("facebook handler not implemented"))
	})
}
