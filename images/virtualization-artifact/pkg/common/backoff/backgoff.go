package backoff

import (
	"time"

	"k8s.io/apimachinery/pkg/util/wait"
)

func CalculateBackOff(failedCount int) time.Duration {
	if failedCount == 0 {
		return 0
	}

	evacuationBackoff := wait.Backoff{
		Duration: 2 * time.Second,
		Factor:   2.0,
		Jitter:   0,
		Cap:      5 * time.Minute,
		Steps:    failedCount,
	}

	return evacuationBackoff.Step()
}
