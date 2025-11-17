package v1

import (
	"errors"
	"fmt"
	"net/http"
	"os"
	"time"

	"live-stream-alerts/internal/logging"
)

const defaultWatchPollInterval = 2 * time.Second

type streamersWatchOptions struct {
	FilePath     string
	Logger       logging.Logger
	PollInterval time.Duration
}

func streamersWatchHandler(opts streamersWatchOptions) http.Handler {
	interval := opts.PollInterval
	if interval <= 0 {
		interval = defaultWatchPollInterval
	}

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			w.Header().Set("Allow", http.MethodGet)
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		if opts.FilePath == "" {
			http.Error(w, "streamers path not configured", http.StatusInternalServerError)
			return
		}

		flusher, ok := w.(http.Flusher)
		if !ok {
			http.Error(w, "streaming unsupported", http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("Connection", "keep-alive")
		w.WriteHeader(http.StatusOK)

		lastMod, _ := fileModTime(opts.FilePath)
		fmt.Fprintf(w, "event: ready\ndata: %d\n\n", lastMod.UnixMilli())
		flusher.Flush()

		ticker := time.NewTicker(interval)
		defer ticker.Stop()

		for {
			select {
			case <-r.Context().Done():
				return
			case <-ticker.C:
				mod, err := fileModTime(opts.FilePath)
				if err != nil {
					if !errors.Is(err, os.ErrNotExist) && opts.Logger != nil {
						opts.Logger.Printf("streamers watch: stat failed: %v", err)
					}
					continue
				}
				if mod.After(lastMod) {
					lastMod = mod
					fmt.Fprintf(w, "event: change\ndata: %d\n\n", mod.UnixMilli())
					flusher.Flush()
				}
			}
		}
	})
}

func fileModTime(path string) (time.Time, error) {
	info, err := os.Stat(path)
	if err != nil {
		return time.Time{}, err
	}
	return info.ModTime(), nil
}
