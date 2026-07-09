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
	"context"
	"strings"
	"sync"

	virtv1 "kubevirt.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	SyncMigrationSlotAnnotation       = "virtualization.deckhouse.io/sync-migration-slot"
	SyncMigrationSourceNodeAnnotation = "virtualization.deckhouse.io/sync-migration-source-node"
	SyncMigrationSlotWaiting          = "waiting"
	SyncMigrationSlotAcquired         = "acquired"

	ParallelSyncMigrationsPerNodeDefault = 1
)

// SyncMigrationLimiter limits how many outbound live migrations per source node
// may be in the sync phase at the same time
type SyncMigrationLimiter struct {
	mu      sync.Mutex
	enabled bool
	limit   int
	// slots maps a source node to the set of owner keys currently holding a slot.
	slots map[string]map[string]struct{}
}

func NewSyncMigrationLimiter(enabled bool, limit int) *SyncMigrationLimiter {
	if limit < 1 {
		limit = ParallelSyncMigrationsPerNodeDefault
	}
	return &SyncMigrationLimiter{
		enabled: enabled,
		limit:   limit,
		slots:   map[string]map[string]struct{}{},
	}
}

func (l *SyncMigrationLimiter) Enabled() bool {
	return l.enabled
}

// TryAcquire reserves a sync slot on sourceNode for the migration owning kvvmi
func (l *SyncMigrationLimiter) TryAcquire(kvvmi *virtv1.VirtualMachineInstance, sourceNode string) bool {
	if sourceNode == "" {
		return false
	}
	owner := ownerKey(kvvmi)
	if owner == "" {
		return false
	}

	l.mu.Lock()
	defer l.mu.Unlock()

	owners := l.slots[sourceNode]
	if owners == nil {
		owners = map[string]struct{}{}
		l.slots[sourceNode] = owners
	}

	if _, ok := owners[owner]; ok {
		return true
	}
	if len(owners) >= l.limit {
		return false
	}

	owners[owner] = struct{}{}
	return true
}

// Release frees the slot held by the migration owning kvvmi on sourceNode
func (l *SyncMigrationLimiter) Release(kvvmi *virtv1.VirtualMachineInstance, sourceNode string) {
	if sourceNode == "" {
		return
	}
	owner := ownerKey(kvvmi)
	if owner == "" {
		return
	}

	l.mu.Lock()
	defer l.mu.Unlock()

	owners := l.slots[sourceNode]
	if owners == nil {
		return
	}
	delete(owners, owner)
	if len(owners) == 0 {
		delete(l.slots, sourceNode)
	}
}

// ReleaseByKVVMI frees any slot held by the given VMI on any node, regardless of the migration UID
func (l *SyncMigrationLimiter) ReleaseByKVVMI(namespace, name string) {
	prefix := namespace + "/" + name + "/"

	l.mu.Lock()
	defer l.mu.Unlock()

	for sourceNode, owners := range l.slots {
		for owner := range owners {
			if strings.HasPrefix(owner, prefix) {
				delete(owners, owner)
			}
		}
		if len(owners) == 0 {
			delete(l.slots, sourceNode)
		}
	}
}

// Restore rebuilds the in-memory registry after a controller restart or leader
// change by scanning VMIs annotated with sync-migration-slot=acquired
func (l *SyncMigrationLimiter) Restore(ctx context.Context, c client.Reader) error {
	if !l.enabled {
		return nil
	}

	var vmis virtv1.VirtualMachineInstanceList
	if err := c.List(ctx, &vmis); err != nil {
		return err
	}

	l.mu.Lock()
	defer l.mu.Unlock()

	for i := range vmis.Items {
		kvvmi := &vmis.Items[i]
		if kvvmi.Annotations[SyncMigrationSlotAnnotation] != SyncMigrationSlotAcquired {
			continue
		}
		if !isInActiveMigration(kvvmi) {
			continue
		}
		sourceNode := kvvmi.Annotations[SyncMigrationSourceNodeAnnotation]
		owner := ownerKey(kvvmi)
		if sourceNode == "" || owner == "" {
			continue
		}
		owners := l.slots[sourceNode]
		if owners == nil {
			owners = map[string]struct{}{}
			l.slots[sourceNode] = owners
		}
		owners[owner] = struct{}{}
	}

	return nil
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
