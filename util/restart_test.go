package util

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"golang.org/x/sync/errgroup"
)

type testConfig string

func (t testConfig) Clone() testConfig {
	return t
}

func TestFilter(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	src := make(chan testConfig)
	go func() {
		for _, txt := range []testConfig{"a", "a", "b", "c", "c", "d"} {
			src <- txt
		}
		close(src)
	}()
	dst := make(chan testConfig)
	go func() {
		filterChanges(ctx, dst, src, func(prev, next testConfig) bool { return prev == next })
		close(dst)
	}()

	var res []testConfig
	for txt := range dst {
		res = append(res, txt)
	}

	assert.Equal(t, []testConfig{"a", "b", "c", "d"}, res)
}

func TestRunTasks(t *testing.T) {
	for _, tc := range []struct {
		name       string
		jobs, want []testConfig
		check      func(t assert.TestingT, err error, msgAndArgs ...interface{}) bool
	}{
		{
			"work",
			[]testConfig{"work-1"},
			[]testConfig{"work-1"},
			assert.NoError,
		},
		{
			"work duplicates",
			[]testConfig{"work-1", "work-1"},
			[]testConfig{"work-1"},
			assert.NoError,
		},
		{
			"work repeated",
			[]testConfig{"work-1", "work-2", "work-1"},
			[]testConfig{"work-1", "work-2", "work-1"},
			assert.NoError,
		},
		{
			"work skip equal",
			[]testConfig{"work-1", "work-1", "work-1", "work-2"},
			[]testConfig{"work-1", "work-2"},
			assert.NoError,
		},
		{
			"error",
			[]testConfig{"work-1", "error-1"},
			[]testConfig{"work-1", "error-1"},
			assert.Error,
		},
		{
			"long shutdown within limits",
			[]testConfig{"work-1", "wait-1"},
			[]testConfig{"work-1", "wait-1"},
			assert.NoError,
		},
		{
			"shut down too long",
			[]testConfig{"work-1", "lock-1", "work-2"},
			[]testConfig{"work-1", "lock-1"},
			assert.Error,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := testRunTasks(tc.jobs)
			t.Log(tc.jobs, "=>", got, err)
			if tc.check(t, err) {
				assert.Equal(t, tc.want, got)
			}
		})
	}
}
func testRunTasks(jobs []testConfig) ([]testConfig, error) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second*5)
	defer cancel()

	var got []testConfig

	taskTimeout := time.Second

	eg, ctx := errgroup.WithContext(ctx)
	r := NewRestartOnChange[testConfig]()
	eg.Go(func() error {
		return r.Run(ctx,
			func(prev, next testConfig) bool { return prev == next },
			func(ctx context.Context, tc testConfig) error {
				got = append(got, tc)

				if strings.HasPrefix(string(tc), "work-") {
					<-ctx.Done()
					return fmt.Errorf("%s: %w", tc, ctx.Err())
				} else if strings.HasPrefix(string(tc), "wait-") {
					<-ctx.Done()
					time.Sleep(taskTimeout / 2)
					return fmt.Errorf("%s: %w", tc, ctx.Err())
				} else if strings.HasPrefix(string(tc), "lock-") {
					<-ctx.Done()
					time.Sleep(taskTimeout * 2)
					return fmt.Errorf("%s: %w", tc, ctx.Err())
				} else if strings.HasPrefix(string(tc), "error-") {
					return errors.New(string(tc))
				} else {
					return fmt.Errorf("unexpected %s", tc)
				}
			},
			time.Second)
	})
	eg.Go(func() error {
		for _, tc := range jobs {
			r.OnConfigUpdated(ctx, tc)
			select {
			case <-ctx.Done():
				return fmt.Errorf("waiting for task results: %w", ctx.Err())
			case <-time.After(time.Millisecond * 200):
			}
		}
		cancel()
		return nil
	})

	return got, eg.Wait()
}
