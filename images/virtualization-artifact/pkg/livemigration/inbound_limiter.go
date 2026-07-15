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
	"strings"

	virtv1 "kubevirt.io/api/core/v1"
)

const (
	InboundMigrationSlotAnnotation       = "virtualization.deckhouse.io/inbound-migration-slot"
	InboundMigrationTargetNodeAnnotation = "virtualization.deckhouse.io/inbound-migration-target-node"
	InboundMigrationSlotWaiting          = slotWaiting
	InboundMigrationSlotAcquired         = slotAcquired
)

// NewInboundMigrationLimiter limits concurrent inbound live migrations per target node.
func NewInboundMigrationLimiter(enabled bool, limit int) *MigrationSlotLimiter {
	return newMigrationSlotLimiter(enabled, limit, slotAnnotations{
		slot: InboundMigrationSlotAnnotation,
		node: InboundMigrationTargetNodeAnnotation,
	})
}

func MarkInboundMigrationSlotWaiting(kvvmi *virtv1.VirtualMachineInstance, targetNode string) {
	setInboundMigrationSlot(kvvmi, InboundMigrationSlotWaiting, targetNode)
}

func MarkInboundMigrationSlotAcquired(kvvmi *virtv1.VirtualMachineInstance, targetNode string) {
	setInboundMigrationSlot(kvvmi, InboundMigrationSlotAcquired, targetNode)
}

func setInboundMigrationSlot(kvvmi *virtv1.VirtualMachineInstance, state, targetNode string) {
	if kvvmi.Annotations == nil {
		kvvmi.Annotations = map[string]string{}
	}
	kvvmi.Annotations[InboundMigrationSlotAnnotation] = state
	kvvmi.Annotations[InboundMigrationTargetNodeAnnotation] = targetNode
}

func ClearInboundMigrationSlot(kvvmi *virtv1.VirtualMachineInstance) {
	if kvvmi.Annotations == nil {
		return
	}
	delete(kvvmi.Annotations, InboundMigrationSlotAnnotation)
	delete(kvvmi.Annotations, InboundMigrationTargetNodeAnnotation)
	if len(kvvmi.Annotations) == 0 {
		kvvmi.Annotations = nil
	}
}

func IsInboundMigrationSlotWaiting(kvvmi *virtv1.VirtualMachineInstance) bool {
	return kvvmi.Annotations[InboundMigrationSlotAnnotation] == InboundMigrationSlotWaiting
}

func InboundMigrationWaitingTargetNode(kvvmi *virtv1.VirtualMachineInstance) string {
	return kvvmi.Annotations[InboundMigrationTargetNodeAnnotation]
}

func DumpInboundMigrationSlot(kvvmi *virtv1.VirtualMachineInstance) string {
	state := kvvmi.Annotations[InboundMigrationSlotAnnotation]
	if state == "" {
		return "not set"
	}
	return strings.Join([]string{state, InboundMigrationWaitingTargetNode(kvvmi)}, ":")
}
