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
			"Usual behaviour for tests is to make post cleanup (when %[1]s is not set or equal to 'yes'). Use %[1]s=no to skip post cleanup after tests. Reusable mode is opposite: post cleanup is disabled when %[1]s is not set or equal to 'no', to remove temporary resources, run with %[1]s=yes.\n",
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
