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

package livemigration

import (
	virtv1 "kubevirt.io/api/core/v1"
)

const (
	SyncMigrationSlotAnnotation       = "virtualization.deckhouse.io/sync-migration-slot"
	SyncMigrationSourceNodeAnnotation = "virtualization.deckhouse.io/sync-migration-source-node"
	SyncMigrationSlotWaiting          = slotWaiting
	SyncMigrationSlotAcquired         = slotAcquired
)

// NewSyncMigrationLimiter limits how many outbound live migrations per source node may be
// in the sync (memory transfer) phase at the same time.
func NewSyncMigrationLimiter(enabled bool, limit int) *MigrationSlotLimiter {
	return newMigrationSlotLimiter(enabled, limit, slotAnnotations{
		slot: SyncMigrationSlotAnnotation,
		node: SyncMigrationSourceNodeAnnotation,
	})
}

func MarkSyncMigrationSlotWaiting(kvvmi *virtv1.VirtualMachineInstance, sourceNode string) {
	setSyncMigrationSlot(kvvmi, SyncMigrationSlotWaiting, sourceNode)
}

func MarkSyncMigrationSlotAcquired(kvvmi *virtv1.VirtualMachineInstance, sourceNode string) {
	setSyncMigrationSlot(kvvmi, SyncMigrationSlotAcquired, sourceNode)
}

func setSyncMigrationSlot(kvvmi *virtv1.VirtualMachineInstance, state, sourceNode string) {
	if kvvmi.Annotations == nil {
		kvvmi.Annotations = map[string]string{}
	}
	kvvmi.Annotations[SyncMigrationSlotAnnotation] = state
	kvvmi.Annotations[SyncMigrationSourceNodeAnnotation] = sourceNode
}

func ClearSyncMigrationSlot(kvvmi *virtv1.VirtualMachineInstance) {
	if kvvmi.Annotations == nil {
		return
	}
	delete(kvvmi.Annotations, SyncMigrationSlotAnnotation)
	delete(kvvmi.Annotations, SyncMigrationSourceNodeAnnotation)
	if len(kvvmi.Annotations) == 0 {
		kvvmi.Annotations = nil
	}
}

func IsSyncMigrationSlotWaiting(kvvmi *virtv1.VirtualMachineInstance) bool {
	return kvvmi.Annotations[SyncMigrationSlotAnnotation] == SyncMigrationSlotWaiting
}

func SyncMigrationWaitingSourceNode(kvvmi *virtv1.VirtualMachineInstance) string {
	return kvvmi.Annotations[SyncMigrationSourceNodeAnnotation]
}
