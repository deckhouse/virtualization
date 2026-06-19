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
	"slices"

	. "github.com/onsi/ginkgo/v2"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/test/e2e/internal/framework"
)

const (
	targetMigrationPrecheckEnvName = "TARGET_MIGRATION_PRECHECK"

	targetMigrationFeatureName         = "TargetMigration"
	minReadyTargetMigrationWorkerNodes = 2
)

// targetMigrationPrecheck implements Precheck interface for VMOP target migration.
type targetMigrationPrecheck struct{}

func (t *targetMigrationPrecheck) Label() string {
	return PrecheckTargetMigration
}

func (t *targetMigrationPrecheck) Run(ctx context.Context, f *framework.Framework) error {
	if !isCheckEnabled(targetMigrationPrecheckEnvName) {
		_, _ = GinkgoWriter.Write([]byte("Target migration precheck is disabled.\n"))
		return nil
	}

	if err := checkTargetMigrationFeature(ctx, f); err != nil {
		return err
	}

	workerNodes, err := listReadyNodesByLabels(ctx, f, map[string]string{
		kvmLabelKey:       "true",
		nodeGroupLabelKey: "worker",
	})
	if err != nil {
		return fmt.Errorf("%s=no to disable this precheck: failed to list ready KVM-enabled worker nodes: %w", targetMigrationPrecheckEnvName, err)
	}
	if len(workerNodes) < minReadyTargetMigrationWorkerNodes {
		return fmt.Errorf("%s=no to disable this precheck: at least %d ready KVM-enabled worker nodes are required, got %d", targetMigrationPrecheckEnvName, minReadyTargetMigrationWorkerNodes, len(workerNodes))
	}

	return nil
}

func checkTargetMigrationFeature(ctx context.Context, f *framework.Framework) error {
	mc, err := f.GetVirtualizationModuleConfig(ctx)
	if err != nil {
		return fmt.Errorf("%s=no to disable this precheck: failed to get virtualization module config: %w", targetMigrationPrecheckEnvName, err)
	}

	enabled := slices.Contains(mc.Spec.Settings.FeatureGates, targetMigrationFeatureName)
	if !enabled {
		return fmt.Errorf("%s=no to disable this precheck: %s feature should be enabled", targetMigrationPrecheckEnvName, targetMigrationFeatureName)
	}

	vmop := &v1alpha2.VirtualMachineOperation{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "target-migration-precheck",
			Namespace: metav1.NamespaceDefault,
		},
		Spec: v1alpha2.VirtualMachineOperationSpec{
			Type:           v1alpha2.VMOPTypeMigrate,
			VirtualMachine: "target-migration-precheck",
			Migrate: &v1alpha2.VirtualMachineOperationMigrateSpec{
				NodeSelector: map[string]string{
					"kubernetes.io/hostname": "target-migration-precheck",
				},
			},
		},
	}

	err = f.GenericClient().Create(ctx, vmop, &client.CreateOptions{
		DryRun: []string{metav1.DryRunAll},
	})
	if err != nil {
		return fmt.Errorf("%s=no to disable this precheck: failed to check %s feature availability: %w", targetMigrationPrecheckEnvName, targetMigrationFeatureName, err)
	}

	return nil
}

func init() {
	RegisterPrecheck(&targetMigrationPrecheck{}, false)
}
