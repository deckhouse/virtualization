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
const PostCleanUpEnv = "POST_CLEANUP"

// PrecreatedCVICleanupEnv defines an environment variable to explicitly enable deletion of precreated CVIs after the suite.
//
// By default, precreated CVIs are not deleted: they are shared across runs and may be reused.
// Set PRECREATED_CVI_CLEANUP=yes to delete them after the suite; unset, empty, or "no" means no deletion.
const PrecreatedCVICleanupEnv = "PRECREATED_CVI_CLEANUP"

func CheckPrecreatedCVICleanupOption() error {
	env := os.Getenv(PrecreatedCVICleanupEnv)
	switch env {
	case "yes", "no", "":
		return nil
	default:
		return fmt.Errorf("invalid value for %s env: %q (allowed: \"\", \"yes\", \"no\")", PrecreatedCVICleanupEnv, env)
	}
}

// IsPrecreatedCVICleanupNeeded returns true only when PRECREATED_CVI_CLEANUP is explicitly set to "yes".
// Default (unset, empty, or "no"): precreated CVIs are not deleted after the suite.
func IsPrecreatedCVICleanupNeeded() bool {
	return os.Getenv(PrecreatedCVICleanupEnv) == "yes"
}

func CheckWithPostCleanUpOption() error {
	env := os.Getenv(PostCleanUpEnv)
	switch env {
	case "yes", "no", "":
		return nil
	default:
		log.Printf(
			"Usual behaviour for tests is to make post cleanup (when %[1]s is not set or equal to 'yes'). Use %[1]s=no to skip post cleanup after tests.\n",
			PostCleanUpEnv,
		)
		return fmt.Errorf("invalid value for the %s env: %q", PostCleanUpEnv, env)
	}
}

func IsCleanUpNeeded() bool {
	return os.Getenv(PostCleanUpEnv) != "no"
}
