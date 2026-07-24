package ingress

import (
	"context"
	"sync"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"sigs.k8s.io/controller-runtime/pkg/log"
	ctrlmetrics "sigs.k8s.io/controller-runtime/pkg/metrics"
)

var (
	reconcileBatchSignals = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "pomerium_ingress_reconcile_batch_signals_total",
		Help: "Number of Kubernetes change signals observed by the adaptive ingress batcher.",
	}, []string{"source"})
	reconcileBatchFlushes = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "pomerium_ingress_reconcile_batch_flushes_total",
		Help: "Number of adaptive ingress batch flushes.",
	}, []string{"result"})
	reconcileBatchSize = prometheus.NewHistogram(prometheus.HistogramOpts{
		Name:    "pomerium_ingress_reconcile_batch_size",
		Help:    "Number of change signals coalesced into an ingress batch.",
		Buckets: prometheus.ExponentialBuckets(1, 2, 12),
	})
	reconcileBatchFlushDuration = prometheus.NewHistogram(prometheus.HistogramOpts{
		Name:    "pomerium_ingress_reconcile_batch_flush_duration_seconds",
		Help:    "Time spent applying an adaptive ingress batch.",
		Buckets: prometheus.DefBuckets,
	})
	reconcileBatchRetries = prometheus.NewCounter(prometheus.CounterOpts{
		Name: "pomerium_ingress_reconcile_batch_retries_total",
		Help: "Number of ingress batch flushes attempted after a failed flush.",
	})
)

func init() {
	ctrlmetrics.Registry.MustRegister(
		reconcileBatchSignals,
		reconcileBatchFlushes,
		reconcileBatchSize,
		reconcileBatchFlushDuration,
		reconcileBatchRetries,
	)
}

type batchFlushFunc func(context.Context) error

type batchWaiter struct {
	sequence uint64
	result   chan error
}

// reconcileBatchCoordinator implements trailing-edge batching outside of
// Reconcile. Informer events can keep extending the quiet period even while a
// reconcile request waits for its configuration to be applied.
type reconcileBatchCoordinator struct {
	quietPeriod time.Duration
	maxWait     time.Duration
	flush       batchFlushFunc

	mu              sync.Mutex
	sequence        uint64
	pending         bool
	firstSignal     time.Time
	lastSignal      time.Time
	pendingSignals  int
	waiters         []batchWaiter
	lastFlushFailed bool
	wake            chan struct{}
}

func newReconcileBatchCoordinator(
	quietPeriod time.Duration,
	maxWait time.Duration,
	flush batchFlushFunc,
) *reconcileBatchCoordinator {
	if maxWait < quietPeriod {
		maxWait = quietPeriod
	}
	return &reconcileBatchCoordinator{
		quietPeriod: quietPeriod,
		maxWait:     maxWait,
		flush:       flush,
		wake:        make(chan struct{}, 1),
	}
}

// NeedLeaderElection keeps the batch writer on the same elected replica as
// the ingress controller.
func (*reconcileBatchCoordinator) NeedLeaderElection() bool { return true }

// Signal records an informer event without blocking its handler.
func (c *reconcileBatchCoordinator) Signal(source string) {
	c.mu.Lock()
	c.signalLocked(time.Now())
	c.mu.Unlock()
	reconcileBatchSignals.WithLabelValues(source).Inc()
	c.notify()
}

// Submit records a real change detected by Reconcile and waits for the batch
// containing it to finish. A failed flush is returned to controller-runtime so
// its normal rate-limited retry behavior remains intact.
func (c *reconcileBatchCoordinator) Submit(ctx context.Context) error {
	waiter := batchWaiter{result: make(chan error, 1)}
	c.mu.Lock()
	c.signalLocked(time.Now())
	waiter.sequence = c.sequence
	c.waiters = append(c.waiters, waiter)
	c.mu.Unlock()
	reconcileBatchSignals.WithLabelValues("reconcile").Inc()
	c.notify()

	select {
	case err := <-waiter.result:
		return err
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (c *reconcileBatchCoordinator) signalLocked(now time.Time) {
	c.sequence++
	if !c.pending {
		c.pending = true
		c.firstSignal = now
		c.pendingSignals = 0
	}
	c.lastSignal = now
	c.pendingSignals++
}

func (c *reconcileBatchCoordinator) notify() {
	select {
	case c.wake <- struct{}{}:
	default:
	}
}

// Start processes one full-state write at a time. Signals received during a
// write form the next batch instead of starting a concurrent Set call.
func (c *reconcileBatchCoordinator) Start(ctx context.Context) error {
	logger := log.FromContext(ctx).WithName("adaptive-ingress-batcher")
	logger.Info("starting", "quiet-period", c.quietPeriod, "max-wait", c.maxWait)
	defer c.finishWaiters(ctx.Err())

	for {
		deadline, ok := c.deadline()
		if !ok {
			select {
			case <-ctx.Done():
				return nil
			case <-c.wake:
				continue
			}
		}
		if !deadline.After(time.Now()) {
			c.flushPending(ctx)
			continue
		}

		timer := time.NewTimer(time.Until(deadline))
		select {
		case <-ctx.Done():
			stopTimer(timer)
			return nil
		case <-c.wake:
			stopTimer(timer)
			continue
		case <-timer.C:
			c.flushPending(ctx)
		}
	}
}

func (c *reconcileBatchCoordinator) deadline() (time.Time, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if !c.pending {
		return time.Time{}, false
	}
	quietDeadline := c.lastSignal.Add(c.quietPeriod)
	maxDeadline := c.firstSignal.Add(c.maxWait)
	if maxDeadline.Before(quietDeadline) {
		return maxDeadline, true
	}
	return quietDeadline, true
}

func (c *reconcileBatchCoordinator) flushPending(ctx context.Context) {
	c.mu.Lock()
	if !c.pending {
		c.mu.Unlock()
		return
	}
	sequence := c.sequence
	size := c.pendingSignals
	retry := c.lastFlushFailed
	c.pending = false
	c.pendingSignals = 0
	c.mu.Unlock()

	if retry {
		reconcileBatchRetries.Inc()
	}
	started := time.Now()
	err := c.flush(ctx)
	reconcileBatchFlushDuration.Observe(time.Since(started).Seconds())
	reconcileBatchSize.Observe(float64(size))
	result := "success"
	if err != nil {
		result = "error"
	}
	reconcileBatchFlushes.WithLabelValues(result).Inc()

	c.mu.Lock()
	c.lastFlushFailed = err != nil
	remaining := c.waiters[:0]
	for _, waiter := range c.waiters {
		if waiter.sequence <= sequence {
			waiter.result <- err
			close(waiter.result)
		} else {
			remaining = append(remaining, waiter)
		}
	}
	c.waiters = remaining
	c.mu.Unlock()
}

func (c *reconcileBatchCoordinator) finishWaiters(err error) {
	if err == nil {
		err = context.Canceled
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	for _, waiter := range c.waiters {
		waiter.result <- err
		close(waiter.result)
	}
	c.waiters = nil
}

func stopTimer(timer *time.Timer) {
	if !timer.Stop() {
		select {
		case <-timer.C:
		default:
		}
	}
}
