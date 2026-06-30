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
	"fmt"
	"strings"
	"sync"

	virtv1 "kubevirt.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	InboundMigrationSlotAnnotation       = "virtualization.deckhouse.io/inbound-migration-slot"
	InboundMigrationTargetNodeAnnotation = "virtualization.deckhouse.io/inbound-migration-target-node"
	InboundMigrationSlotWaiting          = "waiting"
	InboundMigrationSlotAcquired         = "acquired"

	ParallelInboundMigrationsPerNodeDefault = 1
)

// InboundMigrationLimiter limits the number of concurrent inbound live migrations
// per target node. The gate runs in the single leader instance of
// virtualization-controller, so an in-memory registry guarded by a mutex is
// enough to serialize concurrent reconcile workers without a cluster resource.
//
// The registry is not persisted: it is rebuilt on startup from VMI annotations
// (see Restore).
type InboundMigrationLimiter struct {
	mu      sync.Mutex
	enabled bool
	limit   int
	// slots maps a target node to the set of owner keys currently holding a slot.
	slots map[string]map[string]struct{}
}

func NewInboundMigrationLimiter(enabled bool, limit int) *InboundMigrationLimiter {
	if limit < 1 {
		limit = ParallelInboundMigrationsPerNodeDefault
	}
	return &InboundMigrationLimiter{
		enabled: enabled,
		limit:   limit,
		slots:   map[string]map[string]struct{}{},
	}
}

func (l *InboundMigrationLimiter) Enabled() bool {
	return l.enabled
}

// TryAcquire reserves an inbound slot on targetNode for the migration owning kvvmi.
// It is idempotent by owner key: a repeated call for the same migration returns
// the slot it already holds instead of taking a second one.
func (l *InboundMigrationLimiter) TryAcquire(kvvmi *virtv1.VirtualMachineInstance, targetNode string) bool {
	if targetNode == "" {
		return false
	}
	owner := ownerKey(kvvmi)
	if owner == "" {
		return false
	}

	l.mu.Lock()
	defer l.mu.Unlock()

	owners := l.slots[targetNode]
	if owners == nil {
		owners = map[string]struct{}{}
		l.slots[targetNode] = owners
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

// Release frees the slot held by the migration owning kvvmi on targetNode.
// It is idempotent: releasing a slot that is not held is a no-op.
func (l *InboundMigrationLimiter) Release(kvvmi *virtv1.VirtualMachineInstance, targetNode string) {
	if targetNode == "" {
		return
	}
	owner := ownerKey(kvvmi)
	if owner == "" {
		return
	}

	l.mu.Lock()
	defer l.mu.Unlock()

	owners := l.slots[targetNode]
	if owners == nil {
		return
	}
	delete(owners, owner)
	if len(owners) == 0 {
		delete(l.slots, targetNode)
	}
}

// Restore rebuilds the in-memory registry after a controller restart or leader
// change by scanning VMIs annotated with inbound-migration-slot=acquired.
//
// Stale annotations (VMI no longer in an active migration) do not occupy a slot;
// they are cleaned up by the regular reconcile when the migration reaches a
// terminal phase.
//
// ponytail: stale annotation cleanup is left to the reconcile release path
// instead of patching VMIs here — same end state, no extra writes at startup.
func (l *InboundMigrationLimiter) Restore(ctx context.Context, c client.Reader) error {
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
		if kvvmi.Annotations[InboundMigrationSlotAnnotation] != InboundMigrationSlotAcquired {
			continue
		}
		if !isInActiveMigration(kvvmi) {
			continue
		}
		targetNode := kvvmi.Annotations[InboundMigrationTargetNodeAnnotation]
		owner := ownerKey(kvvmi)
		if targetNode == "" || owner == "" {
			continue
		}
		owners := l.slots[targetNode]
		if owners == nil {
			owners = map[string]struct{}{}
			l.slots[targetNode] = owners
		}
		owners[owner] = struct{}{}
	}

	return nil
}

func isInActiveMigration(kvvmi *virtv1.VirtualMachineInstance) bool {
	state := kvvmi.Status.MigrationState
	return state != nil && !state.Completed && !state.Failed
}

// ownerKey identifies the migration that owns a slot. Binding to VMI plus the
// current migration UID keeps repeated reconciles of the same migration
// idempotent while distinguishing a fresh migration of the same VMI.
func ownerKey(kvvmi *virtv1.VirtualMachineInstance) string {
	if kvvmi.Status.MigrationState == nil {
		return ""
	}
	return fmt.Sprintf("%s/%s/%s", kvvmi.Namespace, kvvmi.Name, kvvmi.Status.MigrationState.MigrationUID)
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
