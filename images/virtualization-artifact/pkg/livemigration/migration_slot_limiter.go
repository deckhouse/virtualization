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
	slotWaiting  = "waiting"
	slotAcquired = "acquired"
)

// slotAnnotations names the VMI annotations a MigrationSlotLimiter uses to persist slot
// ownership across controller restarts.
type slotAnnotations struct {
	slot string
	node string
}

// MigrationSlotLimiter limits the number of concurrent live migrations per node. The gate
// runs in the single leader instance of virtualization-controller, so an in-memory registry
// guarded by a mutex is enough to serialize concurrent reconcile workers without a cluster
// resource.
//
// The registry is not persisted: it is rebuilt on startup from VMI annotations (see Restore).
// The inbound limiter (keyed by target node) and the sync limiter (keyed by source node)
// differ only in those annotations.
type MigrationSlotLimiter struct {
	mu      sync.Mutex
	enabled bool
	limit   int
	ann     slotAnnotations
	// slots maps a node to the set of owner keys currently holding a slot.
	slots map[string]map[string]struct{}
}

func newMigrationSlotLimiter(enabled bool, limit int, ann slotAnnotations) *MigrationSlotLimiter {
	if limit < 1 {
		limit = 1
	}
	return &MigrationSlotLimiter{
		enabled: enabled,
		limit:   limit,
		ann:     ann,
		slots:   map[string]map[string]struct{}{},
	}
}

func (l *MigrationSlotLimiter) Enabled() bool {
	return l.enabled
}

// TryAcquire reserves a slot on node for the migration owning kvvmi. It is idempotent by
// owner key: a repeated call for the same migration returns the slot it already holds
// instead of taking a second one.
func (l *MigrationSlotLimiter) TryAcquire(kvvmi *virtv1.VirtualMachineInstance, node string) bool {
	if node == "" {
		return false
	}
	owner := ownerKey(kvvmi)
	if owner == "" {
		return false
	}

	l.mu.Lock()
	defer l.mu.Unlock()

	owners := l.slots[node]
	if owners == nil {
		owners = map[string]struct{}{}
		l.slots[node] = owners
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

// Release frees the slot held by the migration owning kvvmi on node. When the owning
// migration UID is no longer known (MigrationState wiped), every slot keyed by the VMI on
// that node is freed instead — otherwise the slot would leak until a controller restart.
// It is idempotent: releasing a slot that is not held is a no-op.
func (l *MigrationSlotLimiter) Release(kvvmi *virtv1.VirtualMachineInstance, node string) {
	if node == "" {
		return
	}

	l.mu.Lock()
	defer l.mu.Unlock()

	owners := l.slots[node]
	if owners == nil {
		return
	}
	if owner := ownerKey(kvvmi); owner != "" {
		delete(owners, owner)
	} else {
		prefix := kvvmi.Namespace + "/" + kvvmi.Name + "/"
		for o := range owners {
			if strings.HasPrefix(o, prefix) {
				delete(owners, o)
			}
		}
	}
	if len(owners) == 0 {
		delete(l.slots, node)
	}
}

// ReleaseByKVVMI frees any slot held by the given VMI on any node, regardless of the
// migration UID. A deleted VMI can no longer have an active migration, so dropping every
// owner keyed by this VMI is safe. This covers the case where a VMI is removed before its
// migration reaches a terminal state, which would otherwise leak the slot and starve
// subsequent migrations to that node.
func (l *MigrationSlotLimiter) ReleaseByKVVMI(namespace, name string) {
	prefix := namespace + "/" + name + "/"

	l.mu.Lock()
	defer l.mu.Unlock()

	for node, owners := range l.slots {
		for owner := range owners {
			if strings.HasPrefix(owner, prefix) {
				delete(owners, owner)
			}
		}
		if len(owners) == 0 {
			delete(l.slots, node)
		}
	}
}

// Restore rebuilds the in-memory registry after a controller restart or leader change by
// scanning VMIs annotated as acquired and still backed by a non-final migration — the same
// liveness signal the reconcile release path uses. The VMI's MigrationState is not trusted:
// it stays non-final forever after an abnormal death and would resurrect a leaked slot.
//
// Stale annotations are cleaned up by the reconcile release path, not patched here.
func (l *MigrationSlotLimiter) Restore(ctx context.Context, c client.Reader) error {
	if !l.enabled {
		return nil
	}

	var vmis virtv1.VirtualMachineInstanceList
	if err := c.List(ctx, &vmis); err != nil {
		return err
	}
	var migrations virtv1.VirtualMachineInstanceMigrationList
	if err := c.List(ctx, &migrations); err != nil {
		return err
	}
	backed := map[string]struct{}{}
	for i := range migrations.Items {
		m := &migrations.Items[i]
		if !m.IsFinal() {
			backed[m.Namespace+"/"+m.Spec.VMIName] = struct{}{}
		}
	}

	l.mu.Lock()
	defer l.mu.Unlock()

	for i := range vmis.Items {
		kvvmi := &vmis.Items[i]
		if kvvmi.Annotations[l.ann.slot] != slotAcquired {
			continue
		}
		if _, ok := backed[kvvmi.Namespace+"/"+kvvmi.Name]; !ok {
			continue
		}
		node := kvvmi.Annotations[l.ann.node]
		owner := ownerKey(kvvmi)
		if node == "" || owner == "" {
			continue
		}
		owners := l.slots[node]
		if owners == nil {
			owners = map[string]struct{}{}
			l.slots[node] = owners
		}
		owners[owner] = struct{}{}
	}

	return nil
}

// HasMigrationSlot reports whether the VMI currently holds or waits for any migration slot,
// i.e. carries an inbound or sync slot annotation.
func HasMigrationSlot(kvvmi *virtv1.VirtualMachineInstance) bool {
	if kvvmi.Annotations == nil {
		return false
	}
	if _, ok := kvvmi.Annotations[InboundMigrationSlotAnnotation]; ok {
		return true
	}
	_, ok := kvvmi.Annotations[SyncMigrationSlotAnnotation]
	return ok
}

// ownerKey identifies the migration that owns a slot. Binding to VMI plus the current
// migration UID keeps repeated reconciles of the same migration idempotent while
// distinguishing a fresh migration of the same VMI.
func ownerKey(kvvmi *virtv1.VirtualMachineInstance) string {
	if kvvmi.Status.MigrationState == nil {
		return ""
	}
	return fmt.Sprintf("%s/%s/%s", kvvmi.Namespace, kvvmi.Name, kvvmi.Status.MigrationState.MigrationUID)
}
