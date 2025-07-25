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

// PostCleanUpEnv defines an environment variable used to explicitly request the deletion of created/used resources.
// For example, this option is useful when combined with the `REUSABLE=yes` option,
// as the reusable mode does not delete created/used resources by default.
const PostCleanUpEnv = "POST_CLEANUP"

func CheckWithPostCleanUpOption() error {
	env := os.Getenv(PostCleanUpEnv)
	switch env {
	case "yes", "no", "":
		return nil
	default:
		log.Printf(
			"To run tests without post-cleanup, set %s=no. If you want post-cleanup, either leave the variable unset or set %s=yes. By default, when in reusable mode, tests run without post-cleanup if %s is unset.\n",
			PostCleanUpEnv,
			PostCleanUpEnv,
			PostCleanUpEnv,
		)
		return fmt.Errorf("invalid value for the %s env: %q", PostCleanUpEnv, env)
	}
}

func IsCleanUpNeeded() bool {
	if IsReusable() && os.Getenv(PostCleanUpEnv) == "" {
		return false
	}

	if IsReusable() && os.Getenv(PostCleanUpEnv) == "yes" {
		return true
	}

	if os.Getenv(PostCleanUpEnv) == "no" {
		return false
	}

	return os.Getenv(PostCleanUpEnv) == "" || os.Getenv(PostCleanUpEnv) == "yes"
}
