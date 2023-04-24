package stress

import (
	"context"
	"net/http"
	"time"
)

// RunHTTPEchoServer runs a HTTP server that responds with a 200 OK to all requests
func RunHTTPEchoServer(ctx context.Context, addr string) error {
	s := &http.Server{
		Addr:              addr,
		ReadHeaderTimeout: 10 * time.Second,
		Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
		}),
	}
	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		_ = s.Shutdown(shutdownCtx)
		cancel()
	}()
	return s.ListenAndServe()
}
