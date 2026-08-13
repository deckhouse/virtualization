/*
Copyright 2026 Flant JSC

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
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

type permanentTestError struct{}

func (permanentTestError) Permanent() bool { return true }

func (permanentTestError) Error() string { return "checksum will never match" }

// testBackoff retries as many times as the default one, but with waits short
// enough for a test: sleeping through the real backoff would take ~17 minutes.
// A zero Duration is not an option, since Step() reads it as "no steps left".
var testBackoff = Backoff{Duration: time.Millisecond, Steps: 20}

// Test_ExponentialBackoff_PermanentError makes sure a permanent error ends the
// loop on the very first attempt. Retrying it re-downloads the whole image on
// every step, so the user waits out the entire backoff before the failure is
// reported.
func Test_ExponentialBackoff_PermanentError(t *testing.T) {
	permanent := permanentTestError{}

	attempts := 0
	backoff := testBackoff
	err := ExponentialBackoff(context.Background(), func(context.Context) error {
		attempts++
		return fmt.Errorf("source streaming error: %w", permanent)
	}, backoff)

	require.Equal(t, 1, attempts, "a permanent error must not be retried")
	require.ErrorAs(t, err, &permanentTestError{}, "the permanent error must reach the caller unchanged")
}

// Test_ExponentialBackoff_TransientError guards the opposite case: an ordinary
// error still gets every attempt of the backoff.
func Test_ExponentialBackoff_TransientError(t *testing.T) {
	transient := errors.New("connection reset by peer")

	attempts := 0
	backoff := testBackoff
	err := ExponentialBackoff(context.Background(), func(context.Context) error {
		attempts++
		return transient
	}, backoff)

	// Steps counts the waits between the attempts, so the loop runs one attempt
	// more than that.
	require.Equal(t, backoff.Steps+1, attempts)
	require.ErrorIs(t, err, transient)
}

// Test_ExponentialBackoff_Success stops the loop once the attempt succeeds.
func Test_ExponentialBackoff_Success(t *testing.T) {
	attempts := 0
	backoff := testBackoff
	err := ExponentialBackoff(context.Background(), func(context.Context) error {
		attempts++
		if attempts < 2 {
			return errors.New("temporary failure")
		}
		return nil
	}, backoff)

	require.NoError(t, err)
	require.Equal(t, 2, attempts)
}
