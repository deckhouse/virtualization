package retry

import (
	"context"
	"fmt"
	"time"
)

// Fn is a func to retry.
type Fn func(ctx context.Context) error

// Retry retries a given function, f, using exponential backoff.
// If the predicate is never satisfied, it will return the
// last error returned by f.
func Retry(ctx context.Context, f Fn) error {
	if f == nil {
		return fmt.Errorf("nil f passed to retry")
	}

	return ExponentialBackoff(ctx, f, defaultBackoff)
}

// Sleep for 1 then 3 seconds. This should cover networking blips.
var defaultBackoff = Backoff{
	Duration: time.Second,
	Factor:   3.0,
	Jitter:   0.1,
	Steps:    3,
}
