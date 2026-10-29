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

package internal

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	corev1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	virtv1 "kubevirt.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/deckhouse/virtualization-controller/pkg/common/annotations"
	"github.com/deckhouse/virtualization-controller/pkg/controller/kvbuilder"
	"github.com/deckhouse/virtualization-controller/pkg/controller/peering"
	"github.com/deckhouse/virtualization-controller/pkg/controller/vm/internal/state"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
	intvirtv1alpha1 "github.com/deckhouse/virtualization/api/internalvirtualization/v1alpha1"
)

const migrationReceiverRequeueDelay = 10 * time.Second

// isMigrationReceiver tells whether the VM was created to receive a migration from a peer cluster and has
// not received it: such a VM is not started or synchronized as usual. The mark stays after a failed or
// withdrawn receive, so a VM with blank disks never starts; the request deletes it.
func isMigrationReceiver(vm *v1alpha2.VirtualMachine) bool {
	return vm != nil && vm.GetAnnotations()[annotations.AnnVMMigrationReceiver] != ""
}

// loadMigrationReceiver returns the request the VM was created for: the InternalVirtualizationVirtualMachineReceiver with the
// name of the VM, nil when there is none or it is the request of another VM.
func loadMigrationReceiver(ctx context.Context, cl client.Client, vm *v1alpha2.VirtualMachine) (*intvirtv1alpha1.InternalVirtualizationVirtualMachineReceiver, error) {
	if !isMigrationReceiver(vm) {
		return nil, nil
	}
	var r intvirtv1alpha1.InternalVirtualizationVirtualMachineReceiver
	err := cl.Get(ctx, types.NamespacedName{Namespace: vm.Namespace, Name: vm.Name}, &r)
	if k8serrors.IsNotFound(err) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get the migration receiver: %w", err)
	}
	if string(r.UID) != vm.GetAnnotations()[annotations.AnnVMMigrationReceiver] {
		return nil, nil
	}
	return &r, nil
}

// isReceiving tells whether the request still waits for the VM to arrive.
func isReceiving(r *intvirtv1alpha1.InternalVirtualizationVirtualMachineReceiver) bool {
	if r == nil || r.DeletionTimestamp != nil {
		return false
	}
	return r.Status.Phase != intvirtv1alpha1.ReceiverRunning && r.Status.Phase != intvirtv1alpha1.ReceiverFailed
}

// receiverMigrationName is the name and the ID of the receiving migration: the UID of the request, which the
// source gives its migration too.
func receiverMigrationName(r *intvirtv1alpha1.InternalVirtualizationVirtualMachineReceiver) string {
	return string(r.UID)
}

// receivingMigration returns the migration that receives the VM. The request may already be gone: its UID
// stays in the annotation of the VM, and the migration is named after it.
func (h *SyncKvvmHandler) receivingMigration(ctx context.Context, vm *v1alpha2.VirtualMachine, r *intvirtv1alpha1.InternalVirtualizationVirtualMachineReceiver) (*virtv1.VirtualMachineInstanceMigration, error) {
	name := vm.GetAnnotations()[annotations.AnnVMMigrationReceiver]
	if r != nil {
		name = receiverMigrationName(r)
	}
	if name == "" {
		return nil, nil
	}
	var received virtv1.VirtualMachineInstanceMigration
	err := h.client.Get(ctx, types.NamespacedName{Namespace: vm.Namespace, Name: name}, &received)
	if k8serrors.IsNotFound(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &received, nil
}

// syncMigrationReceiver prepares this cluster to receive the VM and reports whether the regular
// synchronization of the KVVM may run: not while the migration lasts, as it would rebuild the receiving
// KVVM.
func (h *SyncKvvmHandler) syncMigrationReceiver(ctx context.Context, s state.VirtualMachineState, r *intvirtv1alpha1.InternalVirtualizationVirtualMachineReceiver) (bool, error) {
	vm := s.VirtualMachine().Changed()

	// The receiving migration is the only reliable sign of the end: the VMI is briefly Running with the
	// migration completed and not yet failed while the source still finishes, and taking that for a
	// success hands the KVVM back to the regular synchronization in the middle of the migration.
	received, err := h.receivingMigration(ctx, vm, r)
	if err != nil {
		return false, err
	}

	// The request may already be gone or deleting while the guest has arrived: the source rolls back on a
	// deleted operation, and deleting the KVVM then would kill the received guest. The succeeded receiving
	// migration is the fact that wins over the missing request.
	if received != nil && received.Status.Phase == virtv1.MigrationSucceeded {
		return h.completeMigrationReceiver(ctx, s, r)
	}

	if !isReceiving(r) {
		if r != nil && r.Status.Phase == intvirtv1alpha1.ReceiverRunning {
			delete(vm.Annotations, annotations.AnnVMMigrationReceiver)
			return true, nil
		}
		if received != nil && received.Status.Phase == virtv1.MigrationFailed && r != nil {
			return false, peering.SetReceiverFailed(ctx, h.client, r, peering.ReceiverReasonMigrationFailed, "the receiving migration failed")
		}
		return false, h.cancelMigrationReceiver(ctx, s)
	}

	rcv, err := peering.ParseReceiver(r)
	if err != nil {
		return false, peering.SetReceiverFailed(ctx, h.client, r, peering.ReceiverReasonInvalidRequest, err.Error())
	}

	receiving := received != nil
	if received != nil && received.Status.Phase == virtv1.MigrationFailed {
		return false, peering.SetReceiverFailed(ctx, h.client, r, peering.ReceiverReasonMigrationFailed, "the receiving migration failed")
	}

	// Nothing is received until the address is reserved here: the one of the source, or one of this cluster
	// when the request asks for it. The IPAM handlers put it in the status from a vmip and from an IPAddress
	// of the sdn module alike.
	current := vm.Status.IPAddress
	if address := peering.ReceiverIPAddress(r); address != "" && current != "" && current != address {
		return false, peering.SetReceiverFailed(ctx, h.client, r, peering.ReceiverReasonIPAddressNotAvailable,
			fmt.Sprintf("the IP address %s is not available in this cluster", address))
	}
	if current == "" {
		return false, nil
	}

	kvvm, err := s.KVVM(ctx)
	if err != nil {
		return false, err
	}
	if kvvm == nil {
		vds, err := s.VirtualDisksByName(ctx)
		if err != nil {
			return false, err
		}
		for _, src := range rcv.VirtualDisks {
			if vd := vds[src.Name]; vd == nil || vd.Status.Target.PersistentVolumeClaim == "" {
				return false, nil
			}
		}
		if msg := checkReceiverCapacity(rcv.Instance, vds); msg != "" {
			return false, peering.SetReceiverFailed(ctx, h.client, r, peering.ReceiverReasonDiskTooSmall, msg)
		}
		kvvm, err = makeReceiverKVVM(ctx, s, rcv.Instance)
		if err != nil {
			return false, peering.SetReceiverFailed(ctx, h.client, r, peering.ReceiverReasonInvalidRequest, err.Error())
		}
		if err = h.client.Create(ctx, kvvm); err != nil && !k8serrors.IsAlreadyExists(err) {
			return false, fmt.Errorf("create the receiving internal virtual machine: %w", err)
		}
	}

	// KubeVirt refuses the receiving migration until it has created the VMI of the KVVM.
	kvvmi, err := s.KVVMI(ctx)
	if err != nil || kvvmi == nil {
		return false, err
	}
	if msg := checkReceiverDisks(rcv.Instance, kvvmi); msg != "" {
		return false, peering.SetReceiverFailed(ctx, h.client, r, peering.ReceiverReasonDiskTooSmall, msg)
	}

	// KubeVirt admission refuses a second migration of the VMI before the API server could answer AlreadyExists.
	if !receiving {
		vmim := &virtv1.VirtualMachineInstanceMigration{
			ObjectMeta: metav1.ObjectMeta{
				Name:            receiverMigrationName(r),
				Namespace:       vm.Namespace,
				OwnerReferences: []metav1.OwnerReference{*metav1.NewControllerRef(vm, v1alpha2.SchemeGroupVersion.WithKind(v1alpha2.VirtualMachineKind))},
			},
			Spec: virtv1.VirtualMachineInstanceMigrationSpec{
				VMIName: vm.Name,
				Receive: &virtv1.VirtualMachineInstanceMigrationTarget{MigrationID: receiverMigrationName(r)},
			},
		}
		if err = h.client.Create(ctx, vmim); err != nil && !k8serrors.IsAlreadyExists(err) {
			return false, fmt.Errorf("create the receiving migration: %w", err)
		}
	}
	return false, peering.PatchReceiverStatus(ctx, h.client, r, func(st *intvirtv1alpha1.VirtualMachineReceiverStatus) {
		st.Phase = intvirtv1alpha1.ReceiverReady
		st.Message = receivingPodMessage(kvvm)
	})
}

// receivingPodMessage tells the source why the receiving pod does not start: otherwise the source sees only a
// migration that does not begin.
func receivingPodMessage(kvvm *virtv1.VirtualMachine) string {
	for _, c := range kvvm.Status.Conditions {
		if c.Type == virtv1.VirtualMachineConditionType(corev1.PodScheduled) && c.Status == corev1.ConditionFalse {
			return fmt.Sprintf("the receiving pod cannot be scheduled: %s: %s", c.Reason, c.Message)
		}
	}
	return ""
}

// completeMigrationReceiver turns the received VM into a regular one: KubeVirt leaves the receiving
// strategy on the KVVM, so the one the VM controller gives the run policy is set here, and the request
// confirms to the source that the VM runs here.
func (h *SyncKvvmHandler) completeMigrationReceiver(ctx context.Context, s state.VirtualMachineState, r *intvirtv1alpha1.InternalVirtualizationVirtualMachineReceiver) (bool, error) {
	vm := s.VirtualMachine().Changed()
	kvvm, err := s.KVVM(ctx)
	if err != nil {
		return false, err
	}
	if kvvm != nil {
		strategy := virtv1.RunStrategyManual
		if vm.Spec.RunPolicy == v1alpha2.AlwaysOnUnlessStoppedManually {
			strategy = virtv1.RunStrategyAlways
		}
		if kvvm.Spec.RunStrategy == nil || *kvvm.Spec.RunStrategy != strategy {
			patch, _ := json.Marshal(map[string]any{"spec": map[string]any{"runStrategy": strategy}})
			if err = h.client.Patch(ctx, kvvm, client.RawPatch(types.MergePatchType, patch)); err != nil {
				return false, fmt.Errorf("set the run strategy of the received internal virtual machine: %w", err)
			}
		}
		// The sizes serve the images created on the way in; a disk resized later must not get them again.
		if kvvm.Spec.Template != nil && kvvm.Spec.Template.ObjectMeta.Annotations[annotations.AnnSourceVolumeSizes] != "" {
			patch, _ := json.Marshal(map[string]any{"spec": map[string]any{"template": map[string]any{"metadata": map[string]any{"annotations": map[string]any{annotations.AnnSourceVolumeSizes: nil}}}}})
			if err = h.client.Patch(ctx, kvvm, client.RawPatch(types.MergePatchType, patch)); err != nil {
				return false, fmt.Errorf("drop the source sizes from the received internal virtual machine: %w", err)
			}
		}
	}
	if r != nil && r.DeletionTimestamp == nil {
		if err = peering.PatchReceiverStatus(ctx, h.client, r, func(st *intvirtv1alpha1.VirtualMachineReceiverStatus) {
			st.Phase = intvirtv1alpha1.ReceiverRunning
			st.ReceiverStarted = true
		}); err != nil {
			return false, err
		}
	}
	delete(vm.Annotations, annotations.AnnVMMigrationReceiver)
	delete(vm.Labels, annotations.LabelVMReceiver)
	for _, ref := range vm.Spec.BlockDeviceRefs {
		if ref.Kind != v1alpha2.DiskDevice {
			continue
		}
		vd := &v1alpha2.VirtualDisk{ObjectMeta: metav1.ObjectMeta{Namespace: vm.Namespace, Name: ref.Name}}
		patch, _ := json.Marshal(map[string]any{"metadata": map[string]any{"labels": map[string]any{annotations.LabelVMReceiver: nil}}})
		if err = h.client.Patch(ctx, vd, client.RawPatch(types.MergePatchType, patch)); client.IgnoreNotFound(err) != nil {
			return false, fmt.Errorf("unmark the received disk %q: %w", ref.Name, err)
		}
	}
	return true, nil
}

// checkReceiverDisks returns why the disks here cannot receive the disks of the source: a block device here
// is smaller than the one of the source, and QEMU of the target refuses it. The sizes are known once the
// receiving pod has the disks; until then nothing is checked.
func checkReceiverDisks(source, kvvmi *virtv1.VirtualMachineInstance) string {
	if kvvmi == nil {
		return ""
	}
	sizes := map[string]int64{}
	for _, v := range source.Status.VolumeStatus {
		sizes[v.Name] = v.Size
	}
	for _, v := range kvvmi.Status.VolumeStatus {
		if want := sizes[v.Name]; v.Size > 0 && v.Size < want {
			return fmt.Sprintf("the disk %q there is %d bytes, smaller than %d bytes here", v.Name, v.Size, want)
		}
	}
	return ""
}

// checkReceiverCapacity refuses the disks here before the source starts copying: a disk whose capacity is
// below the size of the disk of the source cannot hold it. The exact size of the device is known only once
// the receiving pod has the disks, and checkReceiverDisks checks it then.
func checkReceiverCapacity(source *virtv1.VirtualMachineInstance, vds map[string]*v1alpha2.VirtualDisk) string {
	for _, v := range source.Status.VolumeStatus {
		name, kind := kvbuilder.GetOriginalDiskName(v.Name)
		vd := vds[name]
		if kind != v1alpha2.DiskDevice || vd == nil || v.Size == 0 {
			continue
		}
		capacity, err := resource.ParseQuantity(vd.Status.Capacity)
		if err == nil && capacity.Value() < v.Size {
			return fmt.Sprintf("the disk %q there is %d bytes, larger than its capacity %s here", name, v.Size, vd.Status.Capacity)
		}
	}
	return ""
}

// cancelMigrationReceiver undoes an unfinished receive: the receiving KubeVirt objects go. The VM keeps
// its mark and is not started: the request deletes it with its disks.
func (h *SyncKvvmHandler) cancelMigrationReceiver(ctx context.Context, s state.VirtualMachineState) error {
	vm := s.VirtualMachine().Changed()

	// Never delete a receiving VM whose guest already runs: a succeeded migration and a missing request
	// can race, and the completed path must win.
	kvvmi, err := s.KVVMI(ctx)
	if err != nil {
		return err
	}
	if kvvmi != nil && kvvmi.Status.Phase == virtv1.Running {
		return nil
	}

	kvvm, err := s.KVVM(ctx)
	if err != nil {
		return err
	}
	if kvvm != nil && kvvm.Spec.RunStrategy != nil && *kvvm.Spec.RunStrategy == virtv1.RunStrategyWaitAsReceiver && kvvm.DeletionTimestamp == nil {
		if err = h.client.Delete(ctx, kvvm, client.PropagationPolicy(metav1.DeletePropagationForeground)); err != nil && !k8serrors.IsNotFound(err) {
			return fmt.Errorf("delete the receiving internal virtual machine: %w", err)
		}
	}

	var vmims virtv1.VirtualMachineInstanceMigrationList
	if err = h.client.List(ctx, &vmims, client.InNamespace(vm.Namespace)); err != nil {
		return err
	}
	for i := range vmims.Items {
		if vmims.Items[i].Spec.Receive != nil && vmims.Items[i].Spec.VMIName == vm.Name {
			if err = h.client.Delete(ctx, &vmims.Items[i]); err != nil && !k8serrors.IsNotFound(err) {
				return fmt.Errorf("delete the receiving migration: %w", err)
			}
		}
	}
	return nil
}

// makeReceiverKVVM builds the receiving KubeVirt VM: the KVVM this cluster builds for the VM, with its
// volumes, networks and placement, running the domain of the source. The migrating domain must match the
// source; building it here could give another one (the CPU model of a class of the same name, defaults of
// another version). Building the domain here and comparing it with the source is a later improvement.
func makeReceiverKVVM(ctx context.Context, s state.VirtualMachineState, source *virtv1.VirtualMachineInstance) (*virtv1.VirtualMachine, error) {
	kvvm, err := MakeKVVMFromVMSpec(ctx, s)
	if err != nil {
		return nil, err
	}
	kvvm.Spec.Template.Spec.Domain = *source.Spec.Domain.DeepCopy()
	if err = setSourceVolumeSizes(kvvm, source); err != nil {
		return nil, err
	}
	runStrategy := virtv1.RunStrategyWaitAsReceiver
	kvvm.Spec.RunStrategy = &runStrategy
	kvvm.Spec.Running = nil
	return kvvm, nil
}

// setSourceVolumeSizes hands KubeVirt the sizes of the source images: the receiving instance has not run,
// so it knows no sizes, and the image of a Filesystem volume it would create from the PVC size differs from
// the source one by the filesystem overhead.
func setSourceVolumeSizes(kvvm *virtv1.VirtualMachine, source *virtv1.VirtualMachineInstance) error {
	sizes := map[string]int64{}
	for _, vs := range source.Status.VolumeStatus {
		if vs.Size > 0 {
			sizes[vs.Name] = vs.Size
		}
	}
	if len(sizes) == 0 {
		return nil
	}
	raw, err := json.Marshal(sizes)
	if err != nil {
		return err
	}
	if kvvm.Spec.Template.ObjectMeta.Annotations == nil {
		kvvm.Spec.Template.ObjectMeta.Annotations = map[string]string{}
	}
	kvvm.Spec.Template.ObjectMeta.Annotations[annotations.AnnSourceVolumeSizes] = string(raw)
	return nil
}
