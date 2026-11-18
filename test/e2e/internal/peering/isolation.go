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

package peering

import (
	"context"
	"fmt"
	"slices"

	"github.com/deckhouse/virtualization/test/e2e/internal/rewrite"
)

// CheckMigrationIsolation refuses a shared controller restart during another peer migration.
// An empty VM name is used before the test creates its own resources.
func CheckMigrationIsolation(ctx context.Context, c rewrite.Client, vmName string, namespaces ...string) error {
	if c == nil {
		return fmt.Errorf("a migration client is required to check recovery test isolation")
	}

	migrations := &rewrite.VirtualMachineInstanceMigrationList{}
	if err := c.List(ctx, migrations); err != nil {
		return fmt.Errorf("check recovery test isolation: %w", err)
	}

	for _, migration := range migrations.Items {
		// Local migrations do not use the synchronization controller.
		if (migration.Spec.SendTo == nil && migration.Spec.Receive == nil) || migration.IsFinal() || (vmName != "" && migration.Spec.VMIName == vmName && slices.Contains(namespaces, migration.Namespace)) {
			continue
		}

		return fmt.Errorf("migration %s/%s for virtual machine %q is %s (deletionTimestamp=%v): run peering recovery tests on a dedicated cluster without other active peer migrations; the shared synchronization controller must not be restarted",
			migration.Namespace, migration.Name, migration.Spec.VMIName, migration.Status.Phase, migration.DeletionTimestamp)
	}

	return nil
}
