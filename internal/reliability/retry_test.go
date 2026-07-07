package reliability

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestRetrySucceedsAfterTransientFailures(t *testing.T) {
	attempts := 0
	opts := Options{Retries: 2, BaseDelay: time.Millisecond, Timeout: time.Second}

	result, err := Retry(context.Background(), opts, func(ctx context.Context) (string, error) {
		attempts++
		if attempts < 3 {
			return "", errors.New("transient")
		}
		return "ok", nil
	})

	if err != nil {
		t.Fatalf("expected success, got error: %v", err)
	}
	if result != "ok" {
		t.Errorf("expected result %q, got %q", "ok", result)
	}
	if attempts != 3 {
		t.Errorf("expected 3 attempts, got %d", attempts)
	}
}

func TestRetryExhaustsAndReturnsLastError(t *testing.T) {
	attempts := 0
	opts := Options{Retries: 1, BaseDelay: time.Millisecond, Timeout: time.Second}
	sentinel := errors.New("permanent failure")

	_, err := Retry(context.Background(), opts, func(ctx context.Context) (int, error) {
		attempts++
		return 0, sentinel
	})

	if attempts != 2 {
		t.Errorf("expected retries+1=2 attempts, got %d", attempts)
	}
	if !errors.Is(err, sentinel) {
		t.Errorf("expected wrapped sentinel error, got %v", err)
	}
}

func TestRetryInvokesOnRetryBeforeEachBackoff(t *testing.T) {
	var seen []int
	opts := Options{
		Retries:   2,
		BaseDelay: time.Millisecond,
		Timeout:   time.Second,
		OnRetry:   func(attempt int, err error) { seen = append(seen, attempt) },
	}

	_, _ = Retry(context.Background(), opts, func(ctx context.Context) (int, error) {
		return 0, errors.New("always fails")
	})

	if len(seen) != 2 {
		t.Fatalf("expected OnRetry called twice (before attempts 2 and 3), got %v", seen)
	}
	if seen[0] != 1 || seen[1] != 2 {
		t.Errorf("expected attempt numbers [1 2], got %v", seen)
	}
}

func TestRetryRespectsContextCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	opts := Options{Retries: 3, BaseDelay: time.Hour, Timeout: time.Second}
	_, err := Retry(ctx, opts, func(ctx context.Context) (int, error) {
		return 0, errors.New("fails")
	})

	if !errors.Is(err, context.Canceled) {
		t.Errorf("expected context.Canceled during backoff wait, got %v", err)
	}
}
