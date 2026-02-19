/*
Copyright 2024 Flant JSC

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

     http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package retry

import (
	"context"
	"fmt"
	"math"
	"math/rand"
	"strings"
	"time"

	"k8s.io/klog/v2"
)

// Jitter returns a time.Duration between duration and duration + maxFactor *
// duration.
//
// This allows clients to avoid converging on periodic behavior. If maxFactor
// is 0.0, a suggested default value will be chosen.
func Jitter(duration time.Duration, maxFactor float64) time.Duration {
	if maxFactor <= 0.0 {
		maxFactor = 1.0
	}
	wait := duration + time.Duration(rand.Float64()*maxFactor*float64(duration))
	return wait
}

// Backoff holds parameters applied to a Backoff function.
type Backoff struct {
	// The initial duration.
	Duration time.Duration
	// Duration is multiplied by factor each iteration, if factor is not zero
	// and the limits imposed by Steps and Cap have not been reached.
	// Should not be negative.
	// The jitter does not contribute to the updates to the duration parameter.
	Factor float64
	// The sleep at each iteration is the duration plus an additional
	// amount chosen uniformly at random from the interval between
	// zero and `jitter*duration`.
	Jitter float64
	// The total number of attempts in which the duration
	// parameter may change. If not positive, the duration is not
	// changed. Used for exponential backoff in combination with
	// Factor and Cap.
	Steps int
	// A limit on revised values of the duration parameter. If a
	// multiplication by the factor parameter would make the duration
	// exceed the cap then the duration is set to the cap.
	Cap time.Duration

	currentStep int
}

// Step (1) returns an amount of time to sleep determined by the
// original Duration and Jitter and (2) mutates the provided Backoff
// to update its Steps and Duration.
func (b *Backoff) Step() (int, time.Duration) {
	// Return 0 duration to indicate there are no more steps.
	if b.currentStep >= b.Steps {
		return b.Steps, 0
	}
	// Note: initial step 0 become step 1, so range is from step 1 to step "b.Steps".
	b.currentStep++

	duration := b.Duration

	// Calculate increased duration for the step.
	// No increase for the first step, increase once for the second step,
	// increase twice for the third and so on. Use factor ^ (step - 1) to calculate
	// duration for the step.
	if b.Factor != 0 && b.currentStep > 1 {
		duration = time.Duration(float64(b.Duration) * math.Pow(b.Factor, float64(b.currentStep-1)))
	}

	// Limit calculated duration to the Cap value.
	if b.Cap > 0 && duration > b.Cap {
		duration = b.Cap
	}

	// Add jitter.
	if b.Jitter > 0 {
		duration = Jitter(duration, b.Jitter)
	}
	return b.currentStep, duration
}

// ExponentialBackoff repeats a condition check with exponential backoff.
//
// It repeatedly checks the condition and then sleeps, using `backoff.Step()`
// to determine the delay.
// Stops and returns as soon as:
// 1. The condition check returns no or well known error.
// 2. `backoff.Step()` signaling there are no more attempt with zero duration.
// 3. ctx has been cancelled.
// In case (1) the returned error is what the condition function returned.
// Returns error if check was not success after all backoff steps.
func ExponentialBackoff(ctx context.Context, f Fn, backoff Backoff) error {
	const (
		dvcrNoSpaceError             = "no space left on device"
		dvcrInternalErrorPattern     = "UNKNOWN: unknown error;"
		dvcrNoSpaceErrMessage        = "DVCR is overloaded"
		internalDvcrErrMessage       = "Internal DVCR error (could it be overloaded?)"
		datasourceCreatingErrMessage = "error creating data source"
	)

	var err error
	start := time.Now()

	for {
		err = f(ctx)

		switch {
		case err == nil:
			return nil
		case strings.Contains(err.Error(), dvcrNoSpaceError):
			return fmt.Errorf("%s: %w", dvcrNoSpaceErrMessage, err)
		case strings.Contains(err.Error(), dvcrInternalErrorPattern):
			return fmt.Errorf("%s: %w", internalDvcrErrMessage, err)
		case strings.Contains(err.Error(), datasourceCreatingErrMessage):
			return err
		}

		attempt, wait := backoff.Step()
		if wait == 0 {
			break
		}

		klog.Infof("Failed to execute attempt %d of %d: %s: retry in %s...", attempt, backoff.Steps, err, wait.Truncate(100*time.Millisecond))

		timer := time.NewTimer(wait)

		select {
		case <-timer.C:
		case <-ctx.Done():
			timer.Stop()
			return fmt.Errorf("ctx cancelled: %w", err)
		}
	}

	totalDuration := time.Now().Sub(start).Truncate(time.Second)
	return fmt.Errorf("no success after %d attempts within %s, last attempt error: %w", backoff.Steps, totalDuration, err)
}
