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
	"strings"

	. "github.com/onsi/ginkgo/v2"

	"github.com/deckhouse/virtualization/test/e2e/internal/framework"
)

const (
	migrationLimitsCheckEnvName = "MIGRATION_LIMITS_PRECHECK"

	// migrationLimitDisabledValue is the annotation value that turns the
	// corresponding migration limit off.
	migrationLimitDisabledValue = "disabled"
)

// migrationLimitAnnotations are the virtualization ModuleConfig annotations
// that must all be set to "disabled": the suite migrates many VMs in parallel,
// and any of these limits makes migrations queue up and time the specs out.
var migrationLimitAnnotations = []string{
	"virtualization.deckhouse.io/inbound-migration-limit",
	"virtualization.deckhouse.io/outbound-migration-limit",
	"virtualization.deckhouse.io/parallel-per-cluster-migration-limit",
	"virtualization.deckhouse.io/parallel-per-node-migration-limit",
}

// migrationLimitsPrecheck implements the Precheck interface: it requires the
// migration limits to be disabled on the virtualization ModuleConfig.
type migrationLimitsPrecheck struct{}

func (m *migrationLimitsPrecheck) Label() string {
	return PrecheckMigrationLimits
}

func (m *migrationLimitsPrecheck) Run(ctx context.Context, f *framework.Framework) error {
	if !isCheckEnabled(migrationLimitsCheckEnvName) {
		_, _ = GinkgoWriter.Write([]byte("Migration limits precheck is disabled.\n"))
		return nil
	}

	mc, err := f.GetModuleConfig(ctx, virtualizationModuleName)
	if err != nil {
		return fmt.Errorf("%s=no to disable this precheck: failed to get the %s ModuleConfig: %w",
			migrationLimitsCheckEnvName, virtualizationModuleName, err)
	}

	var wrong []string
	for _, annotation := range migrationLimitAnnotations {
		if value := mc.Annotations[annotation]; value != migrationLimitDisabledValue {
			wrong = append(wrong, fmt.Sprintf("%s=%q", annotation, value))
		}
	}
	if len(wrong) == 0 {
		return nil
	}

	return fmt.Errorf("%s=no to disable this precheck: the e2e suite migrates many VMs in parallel and requires "+
		"every migration limit to be disabled on the %s ModuleConfig; got: %s.\n"+
		"To pass this precheck, run:\n%s",
		migrationLimitsCheckEnvName, virtualizationModuleName, strings.Join(wrong, ", "), disableMigrationLimitsCommand())
}

// disableMigrationLimitsCommand renders the kubectl command that sets every
// required annotation, ready to be copy-pasted from the precheck error.
func disableMigrationLimitsCommand() string {
	b := &strings.Builder{}
	fmt.Fprintf(b, "  kubectl annotate moduleconfig %s \\\n", virtualizationModuleName)
	for _, annotation := range migrationLimitAnnotations {
		fmt.Fprintf(b, "    %s=%s \\\n", annotation, migrationLimitDisabledValue)
	}
	b.WriteString("    --overwrite")
	return b.String()
}

// Register the migration limits precheck as common (runs for all tests).
func init() {
	RegisterPrecheck(&migrationLimitsPrecheck{}, true)
}
