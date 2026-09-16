package httpops

import (
	"net/http"
)

func NewHandler(ready func() bool, metrics http.Handler) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /health", func(response http.ResponseWriter, _ *http.Request) {
		response.WriteHeader(http.StatusOK)
		_, _ = response.Write([]byte("ok\n"))
	})
	mux.HandleFunc("GET /ready", func(response http.ResponseWriter, _ *http.Request) {
		if !ready() {
			http.Error(response, "not ready", http.StatusServiceUnavailable)
			return
		}
		response.WriteHeader(http.StatusOK)
		_, _ = response.Write([]byte("ready\n"))
	})
	mux.Handle("GET /metrics", metrics)
	return mux
}
