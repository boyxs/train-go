package ioc

import (
	"net/http"

	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// InitWebServer worker 的最小 HTTP 面：/metrics + /health。server 生命周期归 main。
func InitWebServer() *http.ServeMux {
	mux := http.NewServeMux()
	mux.Handle("/metrics", promhttp.Handler())
	mux.HandleFunc("/health", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"ok","service":"worker"}`))
	})
	return mux
}
