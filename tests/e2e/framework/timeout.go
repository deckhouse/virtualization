/*
Copyright 2025 Flant JSC

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

package framework

import (
	"os"
	"time"
)

var (
	ShortTimeout  = getTimeout("E2E_SHORT_TIMEOUT", 30*time.Second)
	MiddleTimeout = getTimeout("E2E_MIDDLE_TIMEOUT", 90*time.Second)
	LongTimeout   = getTimeout("E2E_LONG_TIMEOUT", 300*time.Second)
	MaxTimeout    = getTimeout("E2E_MAX_TIMEOUT", 600*time.Second)
)

func getTimeout(env string, defaultTimeout time.Duration) time.Duration {
	if e, ok := os.LookupEnv(env); ok {
		t, err := time.ParseDuration(e)
		if err != nil {
			return defaultTimeout
		}
		return t
	}
	return defaultTimeout
}
