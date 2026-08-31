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

package importer

import (
	"encoding/json"
	"errors"
	"fmt"
	"testing"

	"golang.org/x/sys/unix"
)

func TestIsPermanentFailure(t *testing.T) {
	// The disk is declared failed only for what a restart cannot fix. A failed pull is
	// usually the registry or the network blinking, and the pod is restarted anyway.
	cases := map[string]struct {
		err  error
		want bool
	}{
		"image does not fit":   {ValidationSizeError{}, true},
		"target refused write": {NewWriteFailedError(unix.ENOSPC), true},
		"wrapped write error":  {fmt.Errorf("convert: %w", NewWriteFailedError(unix.EIO)), true},
		"pull failed":          {NewImagePullFailedError(errors.New("connection refused")), false},
		"plain error":          {errors.New("boom"), false},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			if got := IsPermanentFailure(tc.err); got != tc.want {
				t.Fatalf("IsPermanentFailure(%v) = %v, want %v", tc.err, got, tc.want)
			}
		})
	}
}

func TestFailureMessageFormat(t *testing.T) {
	// Every failing exit path of the importer reports through this, and the controller reads
	// the result: an empty or unparsable message is the same as no reason at all.
	t.Run("permanent failure is marked", func(t *testing.T) {
		msg := failureMessage(NewWriteFailedError(unix.ENOSPC))
		if msg.ErrMessage == nil || *msg.ErrMessage == "" {
			t.Fatal("no reason in the message")
		}
		if msg.Permanent == nil || !*msg.Permanent {
			t.Fatal("a refused write must be marked permanent")
		}
	})

	t.Run("transient failure is not marked", func(t *testing.T) {
		msg := failureMessage(NewImagePullFailedError(errors.New("connection refused")))
		if msg.Permanent != nil {
			t.Fatalf("permanent = %v, want unset for a failed pull", *msg.Permanent)
		}
	})

	t.Run("serializes to what the controller reads", func(t *testing.T) {
		out, err := failureMessage(ValidationSizeError{}).String()
		if err != nil {
			t.Fatalf("serialize: %v", err)
		}
		var parsed struct {
			ErrMessage string `json:"error-message"`
			Permanent  bool   `json:"permanent"`
		}
		if err := json.Unmarshal([]byte(out), &parsed); err != nil {
			t.Fatalf("the controller would fail to parse %q: %v", out, err)
		}
		if !parsed.Permanent {
			t.Fatal("permanent flag lost in serialization")
		}
	})
}
