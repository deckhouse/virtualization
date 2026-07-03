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

package handler

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"time"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/rand"
	"k8s.io/client-go/util/retry"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/deckhouse/virtualization-controller/pkg/controller/vmpool/internal/poollabels"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/vmcondition"
)

const disksHandlerName = "disks"

// DisksHandler reconciles per-replica disks. It is idempotent and self-healing:
// for every live member it ensures each Delete-policy disk exists (owned by the
// VirtualMachine, so it cascades away with the replica) and is referenced by the
// member. Retain-policy (reusable) disks are handled by a later slice.
type DisksHandler struct {
	client client.Client
	// now is injectable so tests can control free-disk ageing deterministically.
	now func() time.Time
}

func NewDisksHandler(c client.Client) *DisksHandler {
	return &DisksHandler{client: c, now: time.Now}
}

func (h *DisksHandler) Name() string { return disksHandlerName }

func (h *DisksHandler) Handle(ctx context.Context, pool *v1alpha2.VirtualMachinePool) (reconcile.Result, error) {
	if pool.GetDeletionTimestamp() != nil {
		return reconcile.Result{}, nil
	}

	members, err := poollabels.ListMembers(ctx, h.client, pool)
	if err != nil {
		return reconcile.Result{}, fmt.Errorf("list pool members: %w", err)
	}

	var errs error

	// Delete disks whose disk template was removed from the spec (as opposed to a
	// disk merely freed from a scaled-down replica, which stays for reuse). Runs
	// even when no templates remain, so removing the last one still cleans up.
	if err := h.pruneRemovedTemplates(ctx, pool, members); err != nil {
		errs = errors.Join(errs, err)
	}
	if len(pool.Spec.VirtualDiskTemplates) == 0 {
		return reconcile.Result{}, errs
	}

	// Grow existing disks to the template's requested size (increase only).
	if err := h.reconcileDiskSizes(ctx, pool); err != nil {
		errs = errors.Join(errs, err)
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

	// Fallback: if a controller restart lost the in-pass guard and a reuse disk
	// ended up on two members, detach it from the stuck one so it is reassigned.
	if err := h.reassignCollisions(ctx, pool, members); err != nil {
		errs = errors.Join(errs, err)
	}

	// After (re)assignment, garbage-collect free reuse disks per Retain template.
	// Track the soonest a free disk becomes GC-eligible and requeue for it, so ttl
	// collection fires even when nothing else triggers a reconcile (idle pool).
	var requeueAfter time.Duration
	for j := range pool.Spec.VirtualDiskTemplates {
		dt := pool.Spec.VirtualDiskTemplates[j]
		if isDeletePolicy(dt) {
			continue
		}
		after, err := h.gcReuseDisks(ctx, pool, dt, referenced, assignedThisPass)
		if err != nil {
			errs = errors.Join(errs, err)
		}
		if after > 0 && (requeueAfter == 0 || after < requeueAfter) {
			requeueAfter = after
		}
	}
	return reconcile.Result{RequeueAfter: requeueAfter}, errs
}

// pruneRemovedTemplates deletes disks whose disk template is no longer present
// in the pool spec — the template was removed, as opposed to a disk merely freed
// from a scaled-down replica (that one is kept for reuse and aged out by ttl).
// An attached disk is detached first (hot-unplug); a disk that is a replica's
// first (boot) device cannot be hot-unplugged, so it is left in place until the
// replica is recreated.
func (h *DisksHandler) pruneRemovedTemplates(ctx context.Context, pool *v1alpha2.VirtualMachinePool, members []v1alpha2.VirtualMachine) error {
	current := make(map[string]bool, len(pool.Spec.VirtualDiskTemplates))
	for j := range pool.Spec.VirtualDiskTemplates {
		current[pool.Spec.VirtualDiskTemplates[j].Name] = true
	}

	var list v1alpha2.VirtualDiskList
	if err := h.client.List(ctx, &list,
		client.InNamespace(pool.GetNamespace()),
		client.MatchingLabels{poollabels.PoolUID: string(pool.GetUID())},
	); err != nil {
		return fmt.Errorf("list pool disks: %w", err)
	}

	log := logf.FromContext(ctx)
	var errs error
	for i := range list.Items {
		d := &list.Items[i]
		tmpl, managed := d.GetLabels()[poollabels.DiskTemplate]
		if !managed || current[tmpl] || d.GetDeletionTimestamp() != nil {
			continue
		}

		isBoot := false
		// attached stays true while any live member still references the disk — a
		// boot device (cannot hot-unplug) or a detach that has not succeeded yet.
		// A disk is deleted only once nothing references it, otherwise the VM would
		// hang waiting for a block device that is terminating under it.
		attached := false
		for k := range members {
			m := &members[k]
			if m.GetDeletionTimestamp() != nil {
				continue
			}
			switch diskRefIndex(m, d.Name) {
			case -1:
				// not referenced by this member
			case 0:
				isBoot = true
				attached = true
			default:
				if err := h.detachDisk(ctx, m, d.Name); err != nil {
					errs = errors.Join(errs, err)
					attached = true
				}
			}
		}
		if isBoot {
			log.Info("keeping a disk of a removed template: it is the boot device of a running replica and cannot be hot-unplugged; recreate the replica to remove it",
				"disk", d.Name, "diskTemplate", tmpl)
			continue
		}
		if attached {
			continue // detach not yet done; retry next reconcile, never delete an attached disk
		}
		if err := h.client.Delete(ctx, d); err != nil && !apierrors.IsNotFound(err) {
			errs = errors.Join(errs, fmt.Errorf("delete disk %s of removed template %s: %w", d.Name, tmpl, err))
		}
	}
	return errs
}

// reconcileDiskSizes grows every managed disk of a still-present template to the
// template's requested size. Increase only: storage cannot shrink, so a template
// size smaller than an existing disk is ignored.
func (h *DisksHandler) reconcileDiskSizes(ctx context.Context, pool *v1alpha2.VirtualMachinePool) error {
	var errs error
	for j := range pool.Spec.VirtualDiskTemplates {
		dt := pool.Spec.VirtualDiskTemplates[j]
		want := dt.Spec.PersistentVolumeClaim.Size
		if want == nil {
			continue
		}
		var list v1alpha2.VirtualDiskList
		if err := h.client.List(ctx, &list,
			client.InNamespace(pool.GetNamespace()),
			client.MatchingLabels{poollabels.PoolUID: string(pool.GetUID()), poollabels.DiskTemplate: dt.Name},
		); err != nil {
			errs = errors.Join(errs, fmt.Errorf("list disks of template %s: %w", dt.Name, err))
			continue
		}
		for i := range list.Items {
			d := &list.Items[i]
			if have := d.Spec.PersistentVolumeClaim.Size; have != nil && want.Cmp(*have) <= 0 {
				continue // already at or above the requested size
			}
			patched := d.DeepCopy()
			size := want.DeepCopy()
			patched.Spec.PersistentVolumeClaim.Size = &size
			if err := h.client.Update(ctx, patched); err != nil {
				errs = errors.Join(errs, fmt.Errorf("resize disk %s: %w", d.Name, err))
			}
		}
	}
	return errs
}

// diskRefIndex returns the position of the VirtualDisk ref in the member's block
// device list (0 = boot device), or -1 if the member does not reference it.
func diskRefIndex(m *v1alpha2.VirtualMachine, diskName string) int {
	for i, ref := range m.Spec.BlockDeviceRefs {
		if ref.Kind == v1alpha2.DiskDevice && ref.Name == diskName {
			return i
		}
	}
	return -1
}

// gcReuseDisks stamps free reuse disks with a free-since time, clears it when a
// disk is back in use, and deletes free disks that are outside the warm buffer
// (keep) and older than the ttl.
// gcReuseDisks returns how long until the next free disk of this template becomes
// eligible for collection (0 if none is pending), so the caller can requeue —
// otherwise ttl-based GC would only fire on an unrelated reconcile and free disks
// would linger on an idle pool.
func (h *DisksHandler) gcReuseDisks(
	ctx context.Context,
	pool *v1alpha2.VirtualMachinePool,
	dt v1alpha2.VirtualDiskTemplateSpec,
	referenced, assignedThisPass map[string]bool,
) (time.Duration, error) {
	disks, err := h.listReuseDisks(ctx, pool, dt)
	if err != nil {
		return 0, err
	}
	now := h.now()

	var errs error
	var free []*v1alpha2.VirtualDisk
	for i := range disks {
		d := &disks[i]
		inUse := referenced[d.Name] || assignedThisPass[d.Name]
		if inUse {
			// Back in use — drop the free-since stamp if present.
			if _, ok := d.GetAnnotations()[poollabels.FreeSince]; ok {
				patched := d.DeepCopy()
				delete(patched.Annotations, poollabels.FreeSince)
				if err := h.client.Update(ctx, patched); err != nil {
					errs = errors.Join(errs, fmt.Errorf("clear free-since on %s: %w", d.Name, err))
				}
			}
			continue
		}
		// Free — ensure it carries a free-since stamp.
		if _, ok := d.GetAnnotations()[poollabels.FreeSince]; !ok {
			patched := d.DeepCopy()
			if patched.Annotations == nil {
				patched.Annotations = map[string]string{}
			}
			patched.Annotations[poollabels.FreeSince] = now.UTC().Format(time.RFC3339)
			if err := h.client.Update(ctx, patched); err != nil {
				errs = errors.Join(errs, fmt.Errorf("stamp free-since on %s: %w", d.Name, err))
				continue
			}
			d = patched
		}
		free = append(free, d)
	}

	// No ttl configured — keep all free disks (only the warm buffer semantics
	// would apply, and without a ttl nothing ages out).
	if dt.Reclaim.TTL == nil {
		return 0, errs
	}

	// Warm buffer: keep the most-recently-freed `keep` disks immune to the ttl.
	sort.SliceStable(free, func(i, j int) bool {
		return freeSince(free[i]).After(freeSince(free[j]))
	})
	ttl := dt.Reclaim.TTL.Duration
	var requeueAfter time.Duration
	for i, d := range free {
		if i < int(dt.Reclaim.Keep) {
			continue
		}
		if age := now.Sub(freeSince(d)); age <= ttl {
			// Not yet expired — schedule a re-check for when it will be, so GC
			// fires even if nothing else triggers a reconcile.
			remaining := ttl - age
			if remaining <= 0 {
				remaining = time.Second
			}
			if requeueAfter == 0 || remaining < requeueAfter {
				requeueAfter = remaining
			}
			continue
		}
		// Conditional delete: skip if the disk changed since we read it (e.g. was
		// just handed to a new replica).
		if err := h.client.Delete(ctx, d, client.Preconditions{ResourceVersion: &d.ResourceVersion}); err != nil && !apierrors.IsNotFound(err) && !apierrors.IsConflict(err) {
			errs = errors.Join(errs, fmt.Errorf("gc free disk %s: %w", d.Name, err))
		}
	}
	return requeueAfter, errs
}

// reassignCollisions detaches a reuse disk from all but one member when several
// live members reference the same one (a cross-pass race after a restart). The
// keeper is the member that can actually use it (BlockDevicesReady=True), or,
// failing a clear winner, the lexicographically smallest name for determinism.
// The detached members get a fresh disk on the next reconcile.
func (h *DisksHandler) reassignCollisions(ctx context.Context, pool *v1alpha2.VirtualMachinePool, members []v1alpha2.VirtualMachine) error {
	reuse, err := h.listAllReuseDisks(ctx, pool)
	if err != nil {
		return err
	}
	if len(reuse) == 0 {
		return nil
	}
	reuseNames := make(map[string]bool, len(reuse))
	for i := range reuse {
		reuseNames[reuse[i].Name] = true
	}

	refBy := map[string][]*v1alpha2.VirtualMachine{}
	for i := range members {
		m := &members[i]
		if m.GetDeletionTimestamp() != nil {
			continue
		}
		for _, ref := range m.Spec.BlockDeviceRefs {
			if ref.Kind == v1alpha2.DiskDevice && reuseNames[ref.Name] {
				refBy[ref.Name] = append(refBy[ref.Name], m)
			}
		}
	}

	var errs error
	for diskName, ms := range refBy {
		if len(ms) < 2 {
			continue
		}
		keeper := pickKeeper(ms)
		for _, m := range ms {
			if m == keeper {
				continue
			}
			if err := h.detachDisk(ctx, m, diskName); err != nil {
				errs = errors.Join(errs, err)
			}
		}
	}
	return errs
}

func (h *DisksHandler) listAllReuseDisks(ctx context.Context, pool *v1alpha2.VirtualMachinePool) ([]v1alpha2.VirtualDisk, error) {
	var list v1alpha2.VirtualDiskList
	if err := h.client.List(ctx, &list,
		client.InNamespace(pool.GetNamespace()),
		client.MatchingLabels{poollabels.PoolUID: string(pool.GetUID())},
	); err != nil {
		return nil, fmt.Errorf("list reuse disks: %w", err)
	}
	owned := make([]v1alpha2.VirtualDisk, 0, len(list.Items))
	for i := range list.Items {
		_, isReuse := list.Items[i].GetLabels()[poollabels.DiskTemplate]
		if !isReuse {
			continue
		}
		if ref := metav1.GetControllerOf(&list.Items[i]); ref != nil && ref.UID == pool.GetUID() {
			owned = append(owned, list.Items[i])
		}
	}
	return owned, nil
}

func pickKeeper(ms []*v1alpha2.VirtualMachine) *v1alpha2.VirtualMachine {
	keeper := ms[0]
	for _, m := range ms {
		if blockDevicesReady(m) {
			return m
		}
		if m.GetName() < keeper.GetName() {
			keeper = m
		}
	}
	return keeper
}

func blockDevicesReady(vm *v1alpha2.VirtualMachine) bool {
	c := meta.FindStatusCondition(vm.Status.Conditions, vmcondition.TypeBlockDevicesReady.String())
	return c != nil && c.Status == metav1.ConditionTrue
}

func (h *DisksHandler) detachDisk(ctx context.Context, m *v1alpha2.VirtualMachine, diskName string) error {
	// Re-read and retry on conflict: a member is a running VM the vm-controller
	// updates often, so a blind Update from a cached copy would frequently lose the
	// race — and a failed detach must never let the caller delete a still-attached
	// disk out from under the VM.
	err := retry.RetryOnConflict(retry.DefaultRetry, func() error {
		cur := &v1alpha2.VirtualMachine{}
		if err := h.client.Get(ctx, client.ObjectKeyFromObject(m), cur); err != nil {
			return err
		}
		refs := make([]v1alpha2.BlockDeviceSpecRef, 0, len(cur.Spec.BlockDeviceRefs))
		for _, ref := range cur.Spec.BlockDeviceRefs {
			if ref.Kind == v1alpha2.DiskDevice && ref.Name == diskName {
				continue
			}
			refs = append(refs, ref)
		}
		if len(refs) == len(cur.Spec.BlockDeviceRefs) {
			cur.DeepCopyInto(m) // already detached; sync the caller's copy
			return nil
		}
		cur.Spec.BlockDeviceRefs = refs
		if err := h.client.Update(ctx, cur); err != nil {
			return err
		}
		cur.DeepCopyInto(m)
		return nil
	})
	if err != nil {
		return fmt.Errorf("detach disk %s from %s: %w", diskName, m.GetName(), err)
	}
	return nil
}

func freeSince(d *v1alpha2.VirtualDisk) time.Time {
	t, err := time.Parse(time.RFC3339, d.GetAnnotations()[poollabels.FreeSince])
	if err != nil {
		return time.Time{}
	}
	return t
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

	// Reuse a free disk (pool-owned, held by no live member). Prefer a Ready one,
	// but otherwise take a still-provisioning free disk anyway: attaching it is
	// exactly what lets a WaitForFirstConsumer disk bind, and — crucially — it
	// stops us creating a fresh disk on every reconcile while the first one is
	// still provisioning or while the attach we just did is not yet visible in
	// the informer cache (which otherwise over-creates reuse disks).
	var freeReady, freeAny *v1alpha2.VirtualDisk
	for i := range reuseDisks {
		d := &reuseDisks[i]
		if referenced[d.Name] || assignedThisPass[d.Name] || d.GetDeletionTimestamp() != nil || d.Status.Phase == v1alpha2.DiskFailed {
			continue
		}
		if freeAny == nil {
			freeAny = d
		}
		if d.Status.Phase == v1alpha2.DiskReady {
			freeReady = d
			break
		}
	}
	if pick := freeReady; pick != nil || freeAny != nil {
		if pick == nil {
			pick = freeAny
		}
		assignedThisPass[pick.Name] = true
		return h.attachDisk(ctx, m, pick.Name, dt.Name)
	}

	// No free disk at all — create a new pool-owned disk and attach it.
	name := fmt.Sprintf("%s-%s-%s", pool.GetName(), dt.Name, rand.String(6))
	if err := h.client.Create(ctx, h.newRetainDisk(pool, dt, name)); err != nil && !apierrors.IsAlreadyExists(err) {
		return fmt.Errorf("create reuse disk %s: %w", name, err)
	}
	assignedThisPass[name] = true
	return h.attachDisk(ctx, m, name, dt.Name)
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

// attachDisk makes the member reference diskName. If the member still carries an
// unresolved placeholder ref (a blockDeviceRefs entry whose name equals the disk
// template name, i.e. the user referenced the template by name), the placeholder
// is replaced in place so the disk keeps its position in the boot order;
// otherwise the ref is appended. Idempotent: a member already referencing
// diskName is left untouched.
func (h *DisksHandler) attachDisk(ctx context.Context, m *v1alpha2.VirtualMachine, diskName, placeholder string) error {
	if hasDiskRef(m, diskName) {
		return nil
	}
	updated := m.DeepCopy()
	replaced := false
	for i, ref := range updated.Spec.BlockDeviceRefs {
		if ref.Kind == v1alpha2.DiskDevice && ref.Name == placeholder {
			updated.Spec.BlockDeviceRefs[i].Name = diskName
			replaced = true
			break
		}
	}
	if !replaced {
		updated.Spec.BlockDeviceRefs = append(updated.Spec.BlockDeviceRefs, v1alpha2.BlockDeviceSpecRef{
			Kind: v1alpha2.DiskDevice,
			Name: diskName,
		})
	}
	if err := h.client.Update(ctx, updated); err != nil {
		return fmt.Errorf("attach disk %s to %s: %w", diskName, m.GetName(), err)
	}
	// Reflect the update onto the caller's copy so a subsequent disk-template
	// iteration in the same pass builds on these refs (and the fresh
	// resourceVersion) instead of clobbering them from the stale original.
	updated.DeepCopyInto(m)
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
		if err := h.client.Create(ctx, buildDeleteDisk(pool, m, dt, diskName)); err != nil && !apierrors.IsAlreadyExists(err) {
			return fmt.Errorf("create disk %s: %w", diskName, err)
		}
	case err != nil:
		return fmt.Errorf("get disk %s: %w", diskName, err)
	}

	return h.attachDisk(ctx, m, diskName, dt.Name)
}

func buildDeleteDisk(pool *v1alpha2.VirtualMachinePool, m *v1alpha2.VirtualMachine, dt v1alpha2.VirtualDiskTemplateSpec, name string) *v1alpha2.VirtualDisk {
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
