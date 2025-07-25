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

// ReusableEnv defines an environment variable used to reuse resources created previously.
// By default, it retains all resources created during the e2e test after its completion (no cleanup by default in this mode).
// Use the `WITH_POST_CLEANUP=yes` environment variable to clean up resources created or used during the test.
//
// When a test starts, it will reuse existing virtual machines created earlier, if they exist.
// If no virtual machines were found, they will be created.
// Only the following e2e tests are supported in REUSABLE mode. All other tests will be skipped.
// - "VirtualMachineConfiguration"
// - "VirtualMachineMigration"
// - "VirtualMachineConnectivity"
// - "ComplexTest"
// - "ImageHotplug"
// - "VirtualMachineRestoreForce"
const ReusableEnv = "REUSABLE"

func CheckReusableOption() error {
	env := os.Getenv(ReusableEnv)
	switch env {
	case "yes", "":
		return nil
	default:
		log.Printf("To run tests in %s mode, set %s=yes. If you don't intend to use this mode, leave the variable unset.\n", ReusableEnv, ReusableEnv)
		return fmt.Errorf("invalid value for the %s env: %q", ReusableEnv, env)
	}
}

func IsReusable() bool {
	return os.Getenv(ReusableEnv) == "yes"
}
