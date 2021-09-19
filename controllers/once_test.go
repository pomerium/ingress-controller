package controllers

import (
	"context"
	"fmt"
	"math/rand"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestOnce(t *testing.T) {
	var callCount int32
	var errSeen int32
	var onError int32

	o := newOnce(func(ctx context.Context) error {
		_ = atomic.AddInt32(&callCount, 1)
		time.Sleep(time.Second)
		return fmt.Errorf("ERROR")
	}, func() {
		_ = atomic.AddInt32(&onError, 1)
	})

	ctx := context.Background()
	var wg sync.WaitGroup

	iters := 100
	wg.Add(iters)
	for i := 0; i < iters; i++ {
		go func(x int) {
			time.Sleep(time.Millisecond * time.Duration(rand.Intn(10)+10))
			if err := o.yield(ctx); err != nil {
				_ = atomic.AddInt32(&errSeen, 1)
			}
			wg.Done()
		}(i)
	}
	wg.Wait()
	assert.Equal(t, callCount, int32(1))
	assert.Equal(t, errSeen, int32(1))
	assert.Equal(t, onError, int32(1))
}
