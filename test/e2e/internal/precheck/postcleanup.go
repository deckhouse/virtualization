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

package precheck

import (
	"context"
	"fmt"
	"os"

	. "github.com/onsi/ginkgo/v2"

	"github.com/deckhouse/virtualization/test/e2e/internal/framework"
)

const (
	postCleanupPrecheckEnvName = "POST_CLEANUP"
)

// postcleanupPrecheck implements Precheck interface for postcleanup option.
// This is a common precheck that runs for all tests.
type postcleanupPrecheck struct{}

func (c *postcleanupPrecheck) Label() string {
	return PrecheckPostCleanup
}

func (c *postcleanupPrecheck) Run(ctx context.Context, f *framework.Framework) error {
	if !isCheckEnabled(postCleanupPrecheckEnvName) {
		_, _ = GinkgoWriter.Write([]byte("PostCleanup precheck is disabled.\n"))
		return nil
	}

	// Validate POST_CLEANUP env var
	env := os.Getenv(postCleanupPrecheckEnvName)
	switch env {
	case "yes", "no", "":
		// valid values
	default:
		return fmt.Errorf("invalid value for the %s env: %q (allowed: \"\", \"yes\", \"no\")", postCleanupPrecheckEnvName, env)
	}

	return nil
}

// Register postcleanup precheck as common (runs for all tests).
func init() {
	RegisterPrecheck(&postcleanupPrecheck{}, true)
}
