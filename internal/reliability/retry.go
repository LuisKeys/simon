// Package reliability provides a generic exponential-backoff/timeout retry
// helper, mirroring Python's simon/reliability.py with_retry.
package reliability

import (
	"context"
	"fmt"
	"log/slog"
	"math"
	"time"
)

// Options configures Retry. Zero-value Options mirrors with_retry's own
// defaults (retries=2, base_delay=0.5, timeout=60s).
type Options struct {
	// Retries is the number of *extra* attempts after the first, so total
	// attempts = Retries + 1. Defaults to 2 when left at zero via
	// DefaultOptions.
	Retries int
	// BaseDelay is the base of the exponential backoff: BaseDelay * 2^n
	// between attempt n and n+1.
	BaseDelay time.Duration
	// Timeout bounds each individual attempt. Zero means no per-attempt
	// timeout.
	Timeout time.Duration
	// OnRetry, if set, is called with (attemptNumber, err) just before the
	// backoff sleep preceding each retry.
	OnRetry func(attempt int, err error)
}

// DefaultOptions mirrors with_retry's defaults: 2 retries (3 attempts total),
// 500ms base delay, 60s per-attempt timeout.
func DefaultOptions() Options {
	return Options{Retries: 2, BaseDelay: 500 * time.Millisecond, Timeout: 60 * time.Second}
}

// Retry runs fn with per-attempt timeout and exponential backoff. Unlike
// Python's with_retry (which takes a coroutine *factory* because a coroutine
// can only be awaited once), fn is an ordinary Go function and can simply be
// called again on each attempt.
func Retry[T any](ctx context.Context, opts Options, fn func(ctx context.Context) (T, error)) (T, error) {
	attempts := opts.Retries + 1
	var zero T
	var lastErr error

	for attempt := 0; attempt < attempts; attempt++ {
		attemptCtx := ctx
		var cancel context.CancelFunc
		if opts.Timeout > 0 {
			attemptCtx, cancel = context.WithTimeout(ctx, opts.Timeout)
		}
		result, err := fn(attemptCtx)
		if cancel != nil {
			cancel()
		}
		if err == nil {
			return result, nil
		}
		lastErr = err

		if attempt+1 >= attempts {
			break
		}

		delay := time.Duration(float64(opts.BaseDelay) * math.Pow(2, float64(attempt)))
		slog.Warn("attempt failed; retrying",
			"attempt", attempt+1, "of", attempts, "err", err, "delay", delay)
		if opts.OnRetry != nil {
			opts.OnRetry(attempt+1, err)
		}

		select {
		case <-time.After(delay):
		case <-ctx.Done():
			return zero, ctx.Err()
		}
	}

	return zero, fmt.Errorf("retry: all %d attempts failed: %w", attempts, lastErr)
}
