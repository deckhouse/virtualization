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

	"github.com/deckhouse/virtualization/test/e2e/internal/framework"
)

const (
	migratablePrecheckEnvName = "MIGRATABLE_PRECHECK"

	// A virtual machine is reported as migratable only while the cluster has another node that
	// matches its placement rules, so the suite needs a second node to tell the two answers apart.
	minReadyKVMNodesForMigratable = 2
)

// migratablePrecheck implements Precheck interface for the Migratable condition test cluster requirements.
type migratablePrecheck struct{}

func (m *migratablePrecheck) Label() string {
	return PrecheckMigratable
}

func (m *migratablePrecheck) Run(ctx context.Context, f *framework.Framework) error {
	if !isCheckEnabled(migratablePrecheckEnvName) {
		return nil
	}

	nodes, err := listReadyNodesByLabels(ctx, f, map[string]string{
		kvmLabelKey: "true",
	})
	if err != nil {
		return fmt.Errorf("%s=no to disable this precheck: failed to list ready KVM-enabled nodes: %w", migratablePrecheckEnvName, err)
	}
	if len(nodes) < minReadyKVMNodesForMigratable {
		return fmt.Errorf("%s=no to disable this precheck: at least %d ready KVM-enabled nodes are required, got %d", migratablePrecheckEnvName, minReadyKVMNodesForMigratable, len(nodes))
	}

	return nil
}

func init() {
	RegisterPrecheck(&migratablePrecheck{}, false)
}
