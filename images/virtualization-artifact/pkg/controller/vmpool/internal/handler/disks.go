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
	"k8s.io/apimachinery/pkg/util/rand"
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

	// Disks referenced by any live member — the authoritative "in use" signal for
	// reuse disks (see ADR: not the platform InUse condition, which flips on Stop).
	referenced := map[string]bool{}
	for i := range members {
		if members[i].GetDeletionTimestamp() != nil {
			continue
		}
		for _, ref := range members[i].Spec.BlockDeviceRefs {
			if ref.Kind == v1alpha2.DiskDevice {
				referenced[ref.Name] = true
			}
		}
	}
	// Guards against handing the same free disk to two members within one pass
	// (the informer cache does not yet reflect the attach we just did).
	assignedThisPass := map[string]bool{}

	var errs error
	for i := range members {
		m := &members[i]
		if m.GetDeletionTimestamp() != nil {
			continue
		}
		for j := range pool.Spec.VirtualDiskTemplates {
			dt := pool.Spec.VirtualDiskTemplates[j]
			var derr error
			if isDeletePolicy(dt) {
				derr = h.ensureDeleteDisk(ctx, pool, m, dt)
			} else {
				derr = h.ensureRetainDisk(ctx, pool, m, dt, referenced, assignedThisPass)
			}
			if derr != nil {
				errs = errors.Join(errs, derr)
			}
		}
	}
	return reconcile.Result{}, errs
}

// ensureRetainDisk makes sure the member has a reusable (Retain) disk of the
// template attached: it reuses a free pool-owned disk if one exists, otherwise
// creates a new one. The disk is owned by the pool, so it outlives the replica
// and is reused on a later scale-up.
func (h *DisksHandler) ensureRetainDisk(
	ctx context.Context,
	pool *v1alpha2.VirtualMachinePool,
	m *v1alpha2.VirtualMachine,
	dt v1alpha2.VirtualDiskTemplateSpec,
	referenced, assignedThisPass map[string]bool,
) error {
	reuseDisks, err := h.listReuseDisks(ctx, pool, dt)
	if err != nil {
		return err
	}
	reuseByName := make(map[string]*v1alpha2.VirtualDisk, len(reuseDisks))
	for i := range reuseDisks {
		reuseByName[reuseDisks[i].Name] = &reuseDisks[i]
	}

	// Already attached to a reuse disk of this template? Then nothing to do.
	for _, ref := range m.Spec.BlockDeviceRefs {
		if ref.Kind == v1alpha2.DiskDevice && reuseByName[ref.Name] != nil {
			return nil
		}
	}

	// Reuse a free disk: pool-owned, Ready and referenced by nobody live.
	for i := range reuseDisks {
		d := &reuseDisks[i]
		if d.Status.Phase != v1alpha2.DiskReady || referenced[d.Name] || assignedThisPass[d.Name] {
			continue
		}
		assignedThisPass[d.Name] = true
		return h.attachDisk(ctx, m, d.Name)
	}

	// None free — create a new pool-owned disk and attach it.
	name := fmt.Sprintf("%s-%s-%s", pool.GetName(), dt.Name, rand.String(6))
	if err := h.client.Create(ctx, h.newRetainDisk(pool, dt, name)); err != nil && !apierrors.IsAlreadyExists(err) {
		return fmt.Errorf("create reuse disk %s: %w", name, err)
	}
	assignedThisPass[name] = true
	return h.attachDisk(ctx, m, name)
}

func (h *DisksHandler) listReuseDisks(ctx context.Context, pool *v1alpha2.VirtualMachinePool, dt v1alpha2.VirtualDiskTemplateSpec) ([]v1alpha2.VirtualDisk, error) {
	var list v1alpha2.VirtualDiskList
	if err := h.client.List(ctx, &list,
		client.InNamespace(pool.GetNamespace()),
		client.MatchingLabels{poollabels.PoolUID: string(pool.GetUID()), poollabels.DiskTemplate: dt.Name},
	); err != nil {
		return nil, fmt.Errorf("list reuse disks: %w", err)
	}
	// Keep only disks owned by the pool (Retain); Delete disks are owned by a VM.
	owned := make([]v1alpha2.VirtualDisk, 0, len(list.Items))
	for i := range list.Items {
		if ref := metav1.GetControllerOf(&list.Items[i]); ref != nil && ref.UID == pool.GetUID() {
			owned = append(owned, list.Items[i])
		}
	}
	return owned, nil
}

func (h *DisksHandler) newRetainDisk(pool *v1alpha2.VirtualMachinePool, dt v1alpha2.VirtualDiskTemplateSpec, name string) *v1alpha2.VirtualDisk {
	return &v1alpha2.VirtualDisk{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: pool.GetNamespace(),
			Labels: map[string]string{
				poollabels.PoolUID:      string(pool.GetUID()),
				poollabels.Pool:         pool.GetName(),
				poollabels.DiskTemplate: dt.Name,
			},
			// Owned by the pool: the disk outlives the replica and is reused.
			OwnerReferences: []metav1.OwnerReference{
				*metav1.NewControllerRef(pool, v1alpha2.VirtualMachinePoolGVK),
			},
		},
		Spec: *dt.Spec.DeepCopy(),
	}
}

func (h *DisksHandler) attachDisk(ctx context.Context, m *v1alpha2.VirtualMachine, diskName string) error {
	if hasDiskRef(m, diskName) {
		return nil
	}
	updated := m.DeepCopy()
	updated.Spec.BlockDeviceRefs = append(updated.Spec.BlockDeviceRefs, v1alpha2.BlockDeviceSpecRef{
		Kind: v1alpha2.DiskDevice,
		Name: diskName,
	})
	if err := h.client.Update(ctx, updated); err != nil {
		return fmt.Errorf("attach disk %s to %s: %w", diskName, m.GetName(), err)
	}
	return nil
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

	return h.attachDisk(ctx, m, diskName)
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
