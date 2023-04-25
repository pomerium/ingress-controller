package stress

import (
	"context"
	"net/http"
	"time"

	"github.com/rs/zerolog"
)

// RunHTTPEchoServer runs a HTTP server that responds with a 200 OK to all requests
func RunHTTPEchoServer(ctx context.Context, addr string) error {
	log := zerolog.Ctx(ctx).With().Str("addr", addr).Logger()
	log.Info().Msg("starting echo server...")
	s := &http.Server{
		Addr:              addr,
		ReadHeaderTimeout: 10 * time.Second,
		Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("OK\n"))
		}),
	}
	go func() {
		<-ctx.Done()
		log.Info().Msg("stopping echo server...")
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		_ = s.Shutdown(shutdownCtx)
		cancel()
	}()
	err := s.ListenAndServe()
	if err != nil && err != http.ErrServerClosed {
		log.Err(err).Msg("echo server terminated with error")
		return err
	}
	log.Info().Msg("echo server stopped")
	return nil
}
