package stress

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/cenkalti/backoff/v4"
	"github.com/rs/zerolog"
	"golang.org/x/sync/errgroup"
)

// AwaitReadyMulti concurrently waits for multiple HTTP servers to respond with a given status code to a given URL
func AwaitReadyMulti(ctx context.Context, client *http.Client, urls []string, expectHeaders map[string]string) error {
	eg, ctx := errgroup.WithContext(ctx)

	for _, url := range urls {
		url := url
		eg.Go(func() error {
			return AwaitReady(ctx, client, url, expectHeaders)
		})
	}
	return eg.Wait()
}

// AwaitReady waits for a HTTP server to respond with a given status code to a given URL
func AwaitReady(ctx context.Context, client *http.Client, url string, expectHeaders map[string]string) error {
	bo := backoff.NewExponentialBackOff()
	bo.MaxElapsedTime = 0

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(bo.NextBackOff()):
		}

		err := tryRequest(ctx, client, url, expectHeaders)
		if err == nil {
			return nil
		}
		zerolog.Ctx(ctx).Error().Err(err).Str("url", url).Msg("waiting for status")
	}
}

func tryRequest(ctx context.Context, client *http.Client, url string, expectHeaders map[string]string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return fmt.Errorf("new request: %w", err)
	}

	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	_ = resp.Body.Close()
	if resp.StatusCode/100 != 2 {
		return fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	for k, v := range expectHeaders {
		if resp.Header.Get(k) != v {
			return fmt.Errorf("unexpected header value for %s: want %s, got %s", k, v, resp.Header.Get(k))
		}
	}

	return nil
}
