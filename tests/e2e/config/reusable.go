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
	"os"
)

// ReusableEnv defines an environment variable used to retain all resources created during e2e test after its completion (no cleanup).
// When a test starts, it will reuse existing virtual machines created earlier, if they exist.
// If no virtual machines were found, they will be created.
// Only the following e2e tests are supported in REUSABLE mode. All other tests will be skipped.
// - "Virtual machine configuration"
// - "Virtual machine migration"
// - "VM connectivity"
// - "Complex test"
const ReusableEnv = "REUSABLE"

const reusableValue = "yes"

func CheckReusableOption() error {
	env := os.Getenv(ReusableEnv)
	switch env {
	case reusableValue, "":
		return nil
	default:
		return fmt.Errorf("invalid value for the REUSABLE env: %q", env)
	}
}

func IsReusable() bool {
	return os.Getenv(ReusableEnv) == reusableValue
}
