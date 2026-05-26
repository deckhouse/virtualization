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
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strconv"
	"strings"
	"time"

	coordinationv1 "k8s.io/api/coordination/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	virtv1 "kubevirt.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	InboundMigrationSlotAnnotation       = "virtualization.deckhouse.io/inbound-migration-slot"
	InboundMigrationTargetNodeAnnotation = "virtualization.deckhouse.io/inbound-migration-target-node"
	InboundMigrationSlotWaiting          = "waiting"

	InboundMigrationLimiterComponentLabel = "virtualization.deckhouse.io/component"
	InboundMigrationTargetNodeHashLabel   = "virtualization.deckhouse.io/target-node-hash"
	InboundMigrationSlotIndexLabel        = "virtualization.deckhouse.io/slot-index"

	InboundMigrationLimiterComponent = "inbound-migration-limiter"

	InboundMigrationLeaseTargetNodeAnnotation       = "virtualization.deckhouse.io/target-node"
	InboundMigrationVMINamespaceAnnotation          = "virtualization.deckhouse.io/vmi-namespace"
	InboundMigrationVMINameAnnotation               = "virtualization.deckhouse.io/vmi-name"
	InboundMigrationMigrationUIDAnnotation          = "virtualization.deckhouse.io/migration-uid"
	InboundMigrationLeaseNamespace                  = "d8-virtualization"
	ParallelInboundMigrationsPerNodeDefault         = 1
	InboundMigrationLeaseDurationSeconds      int32 = 300
)

type InboundMigrationLimiter struct {
	client client.Client
}

func NewInboundMigrationLimiter(client client.Client) *InboundMigrationLimiter {
	return &InboundMigrationLimiter{client: client}
}

func (l *InboundMigrationLimiter) TryAcquire(ctx context.Context, kvvmi *virtv1.VirtualMachineInstance, targetNode string, limit int) (bool, error) {
	if limit < 1 {
		limit = 1
	}

	slots := slotNames(targetNode, limit)
	for _, slot := range slots {
		lease, err := l.getLease(ctx, slot)
		if apierrors.IsNotFound(err) {
			continue
		}
		if err != nil {
			return false, err
		}
		if leaseHeldByVMI(lease, kvvmi) {
			return true, l.renewLease(ctx, lease)
		}
	}

	for slotIndex, slot := range slots {
		acquired, err := l.tryAcquireSlot(ctx, kvvmi, targetNode, slot, slotIndex)
		if apierrors.IsConflict(err) || apierrors.IsAlreadyExists(err) {
			continue
		}
		if err != nil {
			return false, err
		}
		if acquired {
			return true, nil
		}
	}

	return false, nil
}

func (l *InboundMigrationLimiter) Release(ctx context.Context, kvvmi *virtv1.VirtualMachineInstance, targetNode string, limit int) error {
	if targetNode == "" {
		return nil
	}
	if limit < 1 {
		limit = 1
	}

	for _, slot := range slotNames(targetNode, limit) {
		lease, err := l.getLease(ctx, slot)
		if apierrors.IsNotFound(err) {
			continue
		}
		if err != nil {
			return err
		}
		if leaseHeldByVMI(lease, kvvmi) {
			return client.IgnoreNotFound(l.client.Delete(ctx, lease))
		}
	}

	var leases coordinationv1.LeaseList
	err := l.client.List(ctx, &leases,
		client.InNamespace(InboundMigrationLeaseNamespace),
		client.MatchingLabels{
			InboundMigrationLimiterComponentLabel: InboundMigrationLimiterComponent,
			InboundMigrationTargetNodeHashLabel:   targetNodeHash(targetNode),
		},
	)
	if err != nil {
		return err
	}

	for i := range leases.Items {
		lease := &leases.Items[i]
		if leaseHeldByVMI(lease, kvvmi) {
			return client.IgnoreNotFound(l.client.Delete(ctx, lease))
		}
	}

	return nil
}

func (l *InboundMigrationLimiter) tryAcquireSlot(ctx context.Context, kvvmi *virtv1.VirtualMachineInstance, targetNode, slot string, slotIndex int) (bool, error) {
	lease, err := l.getLease(ctx, slot)
	if apierrors.IsNotFound(err) {
		return true, l.client.Create(ctx, newInboundMigrationLease(kvvmi, targetNode, slot, slotIndex))
	}
	if err != nil {
		return false, err
	}
	if leaseHeldByVMI(lease, kvvmi) {
		return true, l.renewLease(ctx, lease)
	}

	active, err := l.leaseHolderIsActive(ctx, lease)
	if err != nil {
		return false, err
	}
	if active {
		return false, nil
	}

	updateLeaseHolder(lease, kvvmi, targetNode, slotIndex)
	return true, l.client.Update(ctx, lease)
}

func (l *InboundMigrationLimiter) getLease(ctx context.Context, name string) (*coordinationv1.Lease, error) {
	lease := &coordinationv1.Lease{}
	err := l.client.Get(ctx, types.NamespacedName{Namespace: InboundMigrationLeaseNamespace, Name: name}, lease)
	return lease, err
}

func (l *InboundMigrationLimiter) renewLease(ctx context.Context, lease *coordinationv1.Lease) error {
	now := metav1.NewMicroTime(time.Now())
	lease.Spec.RenewTime = &now
	return l.client.Update(ctx, lease)
}

func (l *InboundMigrationLimiter) leaseHolderIsActive(ctx context.Context, lease *coordinationv1.Lease) (bool, error) {
	vmiNamespace := lease.Annotations[InboundMigrationVMINamespaceAnnotation]
	vmiName := lease.Annotations[InboundMigrationVMINameAnnotation]
	migrationUID := lease.Annotations[InboundMigrationMigrationUIDAnnotation]
	if vmiNamespace == "" || vmiName == "" || migrationUID == "" {
		return false, nil
	}

	var kvvmi virtv1.VirtualMachineInstance
	err := l.client.Get(ctx, types.NamespacedName{Namespace: vmiNamespace, Name: vmiName}, &kvvmi)
	if apierrors.IsNotFound(err) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if kvvmi.Status.MigrationState == nil || kvvmi.Status.MigrationState.Completed || kvvmi.Status.MigrationState.Failed {
		return false, nil
	}
	if string(kvvmi.Status.MigrationState.MigrationUID) != migrationUID {
		return false, nil
	}

	return true, nil
}

func newInboundMigrationLease(kvvmi *virtv1.VirtualMachineInstance, targetNode, name string, slotIndex int) *coordinationv1.Lease {
	now := metav1.NewMicroTime(time.Now())
	lease := &coordinationv1.Lease{
		ObjectMeta: metav1.ObjectMeta{
			Namespace:   InboundMigrationLeaseNamespace,
			Name:        name,
			Labels:      map[string]string{},
			Annotations: map[string]string{},
		},
		Spec: coordinationv1.LeaseSpec{
			LeaseDurationSeconds: ptrInt32(InboundMigrationLeaseDurationSeconds),
			AcquireTime:          &now,
			RenewTime:            &now,
		},
	}
	updateLeaseHolder(lease, kvvmi, targetNode, slotIndex)
	return lease
}

func updateLeaseHolder(lease *coordinationv1.Lease, kvvmi *virtv1.VirtualMachineInstance, targetNode string, slotIndex int) {
	if lease.Labels == nil {
		lease.Labels = map[string]string{}
	}
	if lease.Annotations == nil {
		lease.Annotations = map[string]string{}
	}

	migrationUID := string(kvvmi.Status.MigrationState.MigrationUID)
	lease.Labels[InboundMigrationLimiterComponentLabel] = InboundMigrationLimiterComponent
	lease.Labels[InboundMigrationTargetNodeHashLabel] = targetNodeHash(targetNode)
	lease.Labels[InboundMigrationSlotIndexLabel] = strconv.Itoa(slotIndex)
	lease.Annotations[InboundMigrationLeaseTargetNodeAnnotation] = targetNode
	lease.Annotations[InboundMigrationVMINamespaceAnnotation] = kvvmi.Namespace
	lease.Annotations[InboundMigrationVMINameAnnotation] = kvvmi.Name
	lease.Annotations[InboundMigrationMigrationUIDAnnotation] = migrationUID
	lease.Spec.HolderIdentity = ptrString(fmt.Sprintf("%s/%s/%s", kvvmi.Namespace, kvvmi.Name, migrationUID))
	now := metav1.NewMicroTime(time.Now())
	lease.Spec.RenewTime = &now
	if lease.Spec.AcquireTime == nil {
		lease.Spec.AcquireTime = &now
	}
}

func leaseHeldByVMI(lease *coordinationv1.Lease, kvvmi *virtv1.VirtualMachineInstance) bool {
	if lease.Annotations == nil || kvvmi.Status.MigrationState == nil {
		return false
	}

	return lease.Annotations[InboundMigrationVMINamespaceAnnotation] == kvvmi.Namespace &&
		lease.Annotations[InboundMigrationVMINameAnnotation] == kvvmi.Name &&
		lease.Annotations[InboundMigrationMigrationUIDAnnotation] == string(kvvmi.Status.MigrationState.MigrationUID)
}

func slotNames(targetNode string, limit int) []string {
	result := make([]string, 0, limit)
	hash := targetNodeHash(targetNode)
	for i := range limit {
		result = append(result, fmt.Sprintf("inbound-migration-%s-%d", hash, i))
	}
	return result
}

func targetNodeHash(targetNode string) string {
	sum := sha256.Sum256([]byte(targetNode))
	return hex.EncodeToString(sum[:])[:16]
}

func ptrString(v string) *string {
	return &v
}

func ptrInt32(v int32) *int32 {
	return &v
}

func MarkInboundMigrationSlotWaiting(kvvmi *virtv1.VirtualMachineInstance, targetNode string) {
	if kvvmi.Annotations == nil {
		kvvmi.Annotations = map[string]string{}
	}
	kvvmi.Annotations[InboundMigrationSlotAnnotation] = InboundMigrationSlotWaiting
	kvvmi.Annotations[InboundMigrationTargetNodeAnnotation] = targetNode
}

func ClearInboundMigrationSlotWaiting(kvvmi *virtv1.VirtualMachineInstance) {
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
	if !IsInboundMigrationSlotWaiting(kvvmi) {
		return "not waiting"
	}
	return strings.Join([]string{InboundMigrationSlotWaiting, InboundMigrationWaitingTargetNode(kvvmi)}, ":")
}
