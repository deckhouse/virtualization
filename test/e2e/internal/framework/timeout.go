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

// TODO: move from here to a single config file with all env variables for e2e tests.
const (
	E2EShortTimeoutEnv  = "E2E_SHORT_TIMEOUT"
	E2EMiddleTimeoutEnv = "E2E_MIDDLE_TIMEOUT"
	E2ELongTimeoutEnv   = "E2E_LONG_TIMEOUT"
	E2EMaxTimeoutEnv    = "E2E_MAX_TIMEOUT"
)

var (
	ShortTimeout    = getTimeout(E2EShortTimeoutEnv, 30*time.Second)
	MiddleTimeout   = getTimeout(E2EMiddleTimeoutEnv, 60*time.Second)
	LongTimeout     = getTimeout(E2ELongTimeoutEnv, 300*time.Second)
	MaxTimeout      = getTimeout(E2EMaxTimeoutEnv, 600*time.Second)
	PollingInterval = 1 * time.Second
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
