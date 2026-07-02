//go:build EE
// +build EE

/*
Copyright 2026 Flant JSC
Licensed under the Deckhouse Platform Enterprise Edition (EE) license. See https://github.com/deckhouse/deckhouse/blob/main/ee/LICENSE
*/

package handler

import (
	"context"
	"errors"
	"fmt"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/deckhouse/virtualization-controller/pkg/controller/vmpool/internal/poollabels"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
)

const disksHandlerName = "disks"

// DisksHandler reconciles per-replica disks. It is idempotent and self-healing:
// for every live member it ensures each Delete-policy disk exists (owned by the
// VirtualMachine, so it cascades away with the replica) and is referenced by the
// member. Retain-policy (reusable) disks are handled by a later slice.
type DisksHandler struct {
	client client.Client
}

func NewDisksHandler(c client.Client) *DisksHandler {
	return &DisksHandler{client: c}
}

func (h *DisksHandler) Name() string { return disksHandlerName }

func (h *DisksHandler) Handle(ctx context.Context, pool *v1alpha2.VirtualMachinePool) (reconcile.Result, error) {
	if pool.GetDeletionTimestamp() != nil || len(pool.Spec.VirtualDiskTemplates) == 0 {
		return reconcile.Result{}, nil
	}

	members, err := poollabels.ListMembers(ctx, h.client, pool)
	if err != nil {
		return reconcile.Result{}, fmt.Errorf("list pool members: %w", err)
	}

	var errs error
	for i := range members {
		m := &members[i]
		if m.GetDeletionTimestamp() != nil {
			continue
		}
		for j := range pool.Spec.VirtualDiskTemplates {
			dt := pool.Spec.VirtualDiskTemplates[j]
			if !isDeletePolicy(dt) {
				continue // Retain (reusable) disks are handled by a later slice.
			}
			if err := h.ensureDeleteDisk(ctx, pool, m, dt); err != nil {
				errs = errors.Join(errs, err)
			}
		}
	}
	return reconcile.Result{}, errs
}

func isDeletePolicy(dt v1alpha2.VirtualDiskTemplateSpec) bool {
	return dt.Reclaim.OnScaleDown == "" || dt.Reclaim.OnScaleDown == v1alpha2.VirtualDiskReclaimDelete
}

func (h *DisksHandler) ensureDeleteDisk(ctx context.Context, pool *v1alpha2.VirtualMachinePool, m *v1alpha2.VirtualMachine, dt v1alpha2.VirtualDiskTemplateSpec) error {
	diskName := poollabels.DeleteDiskName(m.GetName(), dt.Name)

	var disk v1alpha2.VirtualDisk
	err := h.client.Get(ctx, types.NamespacedName{Namespace: m.GetNamespace(), Name: diskName}, &disk)
	switch {
	case apierrors.IsNotFound(err):
		if err := h.client.Create(ctx, h.newDeleteDisk(pool, m, dt, diskName)); err != nil && !apierrors.IsAlreadyExists(err) {
			return fmt.Errorf("create disk %s: %w", diskName, err)
		}
	case err != nil:
		return fmt.Errorf("get disk %s: %w", diskName, err)
	}

	if !hasDiskRef(m, diskName) {
		updated := m.DeepCopy()
		updated.Spec.BlockDeviceRefs = append(updated.Spec.BlockDeviceRefs, v1alpha2.BlockDeviceSpecRef{
			Kind: v1alpha2.DiskDevice,
			Name: diskName,
		})
		if err := h.client.Update(ctx, updated); err != nil {
			return fmt.Errorf("attach disk %s to %s: %w", diskName, m.GetName(), err)
		}
	}
	return nil
}

func (h *DisksHandler) newDeleteDisk(pool *v1alpha2.VirtualMachinePool, m *v1alpha2.VirtualMachine, dt v1alpha2.VirtualDiskTemplateSpec, name string) *v1alpha2.VirtualDisk {
	return &v1alpha2.VirtualDisk{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: pool.GetNamespace(),
			Labels: map[string]string{
				poollabels.PoolUID:      string(pool.GetUID()),
				poollabels.Pool:         pool.GetName(),
				poollabels.DiskTemplate: dt.Name,
			},
			// Owned by the VirtualMachine: the disk cascades away with the replica.
			OwnerReferences: []metav1.OwnerReference{
				*metav1.NewControllerRef(m, v1alpha2.SchemeGroupVersion.WithKind(v1alpha2.VirtualMachineKind)),
			},
		},
		Spec: *dt.Spec.DeepCopy(),
	}
}

func hasDiskRef(m *v1alpha2.VirtualMachine, diskName string) bool {
	for _, ref := range m.Spec.BlockDeviceRefs {
		if ref.Kind == v1alpha2.DiskDevice && ref.Name == diskName {
			return true
		}
	}
	return false
}
