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

// An immediate storage class is required for test cases where a `VirtualDisk` might not be attached to a `VirtualMachine` but should be in the Ready phase to operate with it.
// If a test case does not require the immediate storage class, this check can be skipped.
const SkipImmediateStorageClassCheckEnv = "SKIP_IMMEDIATE_SC_CHECK"

func CheckStorageClassOption() error {
	env := os.Getenv(SkipImmediateStorageClassCheckEnv)
	switch env {
	case "yes", "":
		return nil
	default:
		log.Printf("To skip the immediate storage class check, set %s=yes. If you don't intend to skip this check, leave the variable unset.\n", SkipImmediateStorageClassCheckEnv)
		return fmt.Errorf("invalid value for the %s env: %q", SkipImmediateStorageClassCheckEnv, env)
	}
}

func SkipImmediateStorageClassCheck() bool {
	return os.Getenv(SkipImmediateStorageClassCheckEnv) == "yes"
}
