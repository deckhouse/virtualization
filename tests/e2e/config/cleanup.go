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

package config

import (
	"fmt"
	"log"
	"os"
)

// WithPostCleanUpEnv defines an environment variable used to explicitly request the deletion of created/used resources.
// For example, this option is useful when combined with the `REUSABLE=yes` option,
// as the reusable mode does not delete created/used resources by default.
const (
	WithPostCleanUpEnv    = "WITH_POST_CLEANUP"
	WithoutPostCleanUpEnv = "WITHOUT_POST_CLEANUP"
)

func CheckWithPostCleanUpOption() error {
	env := os.Getenv(WithPostCleanUpEnv)
	switch env {
	case "yes", "":
		return nil
	default:
		log.Printf("To run tests in %s mode, set %s=yes. If you don't intend to use this mode, leave the variable unset.\n", WithPostCleanUpEnv, WithPostCleanUpEnv)
		return fmt.Errorf("invalid value for the %s env: %q", WithPostCleanUpEnv, env)
	}
}

func WithPostCleanUp() bool {
	return os.Getenv(WithPostCleanUpEnv) == "yes"
}

func WithoutPostCleanUp() bool {
	return os.Getenv(WithoutPostCleanUpEnv) == "yes"
}

func IsCleanUpNeeded() bool {
	if IsReusable() {
		return WithPostCleanUp()
	}

	if WithoutPostCleanUp() {
		return false
	}

	return true
}
