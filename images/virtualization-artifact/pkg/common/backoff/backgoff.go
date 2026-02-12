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
