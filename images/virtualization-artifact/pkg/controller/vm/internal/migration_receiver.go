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
	"slices"
	"time"

	corev1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	virtv1 "kubevirt.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/deckhouse/virtualization-controller/pkg/common"
	"github.com/deckhouse/virtualization-controller/pkg/common/annotations"
	commonvm "github.com/deckhouse/virtualization-controller/pkg/common/vm"
	commonvmop "github.com/deckhouse/virtualization-controller/pkg/common/vmop"
	"github.com/deckhouse/virtualization-controller/pkg/controller/conditions"
	"github.com/deckhouse/virtualization-controller/pkg/controller/indexer"
	"github.com/deckhouse/virtualization-controller/pkg/controller/kvbuilder"
	"github.com/deckhouse/virtualization-controller/pkg/controller/peering"
	"github.com/deckhouse/virtualization-controller/pkg/controller/vm/internal/state"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/vmcondition"
	intvirtv1alpha1 "github.com/deckhouse/virtualization/api/internalvirtualization/v1alpha1"
)

const (
	migrationReceiverRequeueDelay = 10 * time.Second
	// migrationReceiverUndecidedDelay paces a VM not yet known to be the request's or not: the cache of the
	// request catches up within seconds.
	migrationReceiverUndecidedDelay = 2 * time.Second
)

// loadMigrationReceiver returns the receiver and whether its association with the VM is decided.
// Wait on unresolved owner references to avoid starting a regular VM with blank disks.
func loadMigrationReceiver(ctx context.Context, cl client.Client, vm *v1alpha2.VirtualMachine) (*intvirtv1alpha1.InternalVirtualizationVirtualMachineReceiver, bool, error) {
	if vm == nil {
		return nil, true, nil
	}

	referred := slices.ContainsFunc(vm.GetOwnerReferences(), func(ref metav1.OwnerReference) bool {
		return ref.Kind == intvirtv1alpha1.InternalVirtualizationVirtualMachineReceiverKind
	})

	var r intvirtv1alpha1.InternalVirtualizationVirtualMachineReceiver

	err := cl.Get(ctx, types.NamespacedName{Namespace: vm.Namespace, Name: vm.Name}, &r)
	if k8serrors.IsNotFound(err) {
		return nil, !referred, nil
	}

	if err != nil {
		return nil, false, fmt.Errorf("get the migration receiver: %w", err)
	}

	if peering.CreatedVirtualMachine(&r, vm) {
		return &r, true, nil
	}

	return nil, !referred, nil
}

// isReceivingKVVM also protects an orphaned WaitAsReceiver VM from regular synchronization.
// Normally the receiver stays under its finalizer until the KVVM is ready for regular handling.
func isReceivingKVVM(kvvm *virtv1.VirtualMachine) bool {
	return kvvm != nil && kvvm.Spec.RunStrategy != nil && *kvvm.Spec.RunStrategy == virtv1.RunStrategyWaitAsReceiver
}

// mayBeReceivingVM tells whether the VM receives a migration from a peer cluster, or may: such a VM is not
// started or synchronized as usual.
func mayBeReceivingVM(ctx context.Context, cl client.Client, vm *v1alpha2.VirtualMachine, kvvm *virtv1.VirtualMachine) (bool, error) {
	r, decided, err := loadMigrationReceiver(ctx, cl, vm)
	if err != nil {
		return false, err
	}

	return (r != nil && r.Status.Phase != intvirtv1alpha1.ReceiverRunning) || !decided || isReceivingKVVM(kvvm), nil
}

// syncReceivingFromPeer shows in the status of the VM that it is being received and from which cluster. The
// migration state is rebuilt from the KVVMI before, so prev keeps the peer while the request is not
// yet known to be the VM's, or an orphaned KVVM still waits to receive.
func syncReceivingFromPeer(ctx context.Context, cl client.Client, vm *v1alpha2.VirtualMachine, kvvm *virtv1.VirtualMachine, prev *v1alpha2.VirtualMachineMigrationState) error {
	r, decided, err := loadMigrationReceiver(ctx, cl, vm)
	if err != nil {
		return err
	}

	if decided && !isReceiving(r) && !isReceivingKVVM(kvvm) {
		conditions.RemoveCondition(vmcondition.TypeReceivingFromPeer, &vm.Status.Conditions)
		return nil
	}

	peer := ""
	if prev != nil {
		peer = prev.Source.ProjectPeerName
	}

	if r != nil && r.Spec.SourcePeer != "" {
		peer = r.Spec.SourcePeer
	}

	if decided {
		msg := "The virtual machine is being received from a peer cluster."
		if peer != "" {
			msg = fmt.Sprintf("The virtual machine is being received from peer cluster %q.", peer)
		}

		conditions.SetCondition(conditions.NewConditionBuilder(vmcondition.TypeReceivingFromPeer).
			Generation(vm.GetGeneration()).
			Status(metav1.ConditionTrue).
			Reason(vmcondition.ReasonReceivingFromPeerInProgress).
			Message(msg),
			&vm.Status.Conditions)
	}

	if peer == "" {
		return nil
	}

	if vm.Status.MigrationState == nil {
		vm.Status.MigrationState = &v1alpha2.VirtualMachineMigrationState{}
	}

	vm.Status.MigrationState.Source.ProjectPeerName = peer

	return nil
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

func (h *SyncKvvmHandler) receivingMigration(ctx context.Context, vm *v1alpha2.VirtualMachine, r *intvirtv1alpha1.InternalVirtualizationVirtualMachineReceiver) (*virtv1.VirtualMachineInstanceMigration, error) {
	if r == nil {
		return nil, nil
	}

	name := receiverMigrationName(r)

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

	kvvm, err := s.KVVM(ctx)
	if err != nil {
		return false, err
	}

	// Running is published only after finalization. Retaining the receiver for history or
	// TTL must not reapply the receive strategy or constrain a later KVVM replacement.
	if r != nil && r.Status.Phase == intvirtv1alpha1.ReceiverRunning {
		return true, nil
	}

	// Record the identity before touching an existing KVVM or completing the receive. A same-name
	// replacement must never inherit an earlier receive. Owner UID recovers a Create/status-patch gap.
	if kvvm != nil && r != nil {
		if r.Status.InternalVirtualMachineUID != "" && r.Status.InternalVirtualMachineUID != kvvm.UID {
			return false, fmt.Errorf("receiving internal virtual machine UID changed: expected %s, got %s", r.Status.InternalVirtualMachineUID, kvvm.UID)
		}

		if r.Status.InternalVirtualMachineUID == "" {
			if !metav1.IsControlledBy(kvvm, vm) {
				return false, fmt.Errorf("receiving internal virtual machine is not owned by the received virtual machine")
			}

			if err = peering.PatchReceiverStatus(ctx, h.client, r, func(st *intvirtv1alpha1.VirtualMachineReceiverStatus) { st.InternalVirtualMachineUID = kvvm.UID }); err != nil {
				return false, err
			}
		}
	}

	// The receiving migration is the only reliable sign of the end: the VMI is briefly Running with the
	// migration completed and not yet failed while the source still finishes, and taking that for a
	// success hands the KVVM back to the regular synchronization in the middle of the migration.
	received, err := h.receivingMigration(ctx, vm, r)
	if err != nil {
		return false, err
	}

	// The request may be deleting while the guest has arrived: the source rolls back on a
	// deleted operation, and deleting the KVVM then would kill the received guest. Success wins over cancellation.
	if received != nil && received.Status.Phase == virtv1.MigrationSucceeded {
		return h.finalizeReceivingVM(ctx, s, r)
	}

	if !isReceiving(r) {
		if received != nil && received.Status.Phase == virtv1.MigrationFailed && r != nil {
			return false, h.setReceivingMigrationFailed(ctx, s, r, received)
		}

		return false, h.cancelVMReception(ctx, s, r)
	}

	rcv, err := peering.ParseReceiver(r)
	if err != nil {
		return false, peering.SetReceiverFailed(ctx, h.client, r, peering.ReceiverReasonInvalidRequest, err.Error())
	}

	receiving := received != nil
	if received != nil && received.Status.Phase == virtv1.MigrationFailed {
		return false, h.setReceivingMigrationFailed(ctx, s, r, received)
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

	if kvvm == nil {
		// The cache may not have observed a just-created KVVM. Never recreate a recorded receive:
		// a replacement would have a different UID and could restart with blank disks.
		if r.Status.InternalVirtualMachineUID != "" {
			return false, nil
		}

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

		if err = h.client.Create(ctx, kvvm); err != nil {
			if k8serrors.IsAlreadyExists(err) {
				return false, nil
			}

			return false, fmt.Errorf("create the receiving internal virtual machine: %w", err)
		}

		if err = peering.PatchReceiverStatus(ctx, h.client, r, func(st *intvirtv1alpha1.VirtualMachineReceiverStatus) { st.InternalVirtualMachineUID = kvvm.UID }); err != nil {
			return false, err
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

	events, err := h.receivingEvents(ctx, kvvmi)
	if err != nil {
		return false, err
	}

	if msg := receivingQuotaExceeded(kvvmi, events); msg != "" {
		return false, peering.SetReceiverFailed(ctx, h.client, r, peering.ReceiverReasonQuotaExceeded, msg)
	}

	problems := append([]string{receivingPodMessage(kvvm)}, receivingTargetFailures(kvvmi, events)...)

	return false, peering.PatchReceiverStatus(ctx, h.client, r, func(st *intvirtv1alpha1.VirtualMachineReceiverStatus) {
		st.Phase = intvirtv1alpha1.ReceiverReady
		st.Message = receivingProblemsMessage(st.Message, problems)
	})
}

// receivingProblemsMessage adds the problems seen now to those seen before. With none seen now, the receive
// is no longer held up, and what held it up before is no longer the story of a failure that may follow.
func receivingProblemsMessage(history string, problems []string) string {
	if !slices.ContainsFunc(problems, func(p string) bool { return p != "" }) {
		return ""
	}

	return peering.AppendReceiverProblems(history, problems...)
}

// receivingEvents returns the events of the receiving VMI: why its pod cannot be created, and why the node
// of the pod cannot prepare the VM (the pod runs, but virt-handler fails to set up the target, for example
// to mount a volume whose mode differs from the one of the source). Both show only there.
func (h *SyncKvvmHandler) receivingEvents(ctx context.Context, kvvmi *virtv1.VirtualMachineInstance) ([]corev1.Event, error) {
	var events corev1.EventList

	err := h.client.List(ctx, &events, client.InNamespace(kvvmi.Namespace),
		client.MatchingFields{indexer.IndexFieldEventByInvolvedObjectName: kvvmi.Name})
	if err != nil {
		return nil, fmt.Errorf("list the events of the receiving internal virtual machine instance: %w", err)
	}

	return events.Items, nil
}

// receivingTargetFailures returns virt-handler synchronization failures
// for the receiving VMI in the order they first occurred.
func receivingTargetFailures(kvvmi *virtv1.VirtualMachineInstance, events []corev1.Event) []string {
	var failures []*corev1.Event

	for i := range events {
		e := &events[i]
		if e.InvolvedObject.UID == kvvmi.UID && e.Type == corev1.EventTypeWarning && e.Reason == string(virtv1.SyncFailed) {
			failures = append(failures, e)
		}
	}

	slices.SortStableFunc(failures, func(a, b *corev1.Event) int { return a.FirstTimestamp.Compare(b.FirstTimestamp.Time) })

	problems := make([]string, 0, len(failures))
	for _, e := range failures {
		problems = append(problems, fmt.Sprintf("the receiving node cannot prepare the virtual machine: %s: %s", e.Reason, e.Message))
	}

	return problems
}

// receivingQuotaExceeded tells the source that a ResourceQuota here refuses the receiving pod. KubeVirt retries
// the pod with no end, and the source would only see a migration that does not begin.
func receivingQuotaExceeded(kvvmi *virtv1.VirtualMachineInstance, events []corev1.Event) string {
	for _, e := range events {
		if e.InvolvedObject.UID != kvvmi.UID || e.Type != corev1.EventTypeWarning || e.Reason != failedCreatePodReason {
			continue
		}

		if quota := common.ParseQuotaExceeded(e.Message); quota != nil {
			return fmt.Sprintf("the ResourceQuota %q of namespace %q does not allow the virtual machine (requested: %s, used: %s, limited: %s): free resources there or raise the quota",
				quota.Name, kvvmi.Namespace, quota.Requested, quota.Used, quota.Limited)
		}
	}

	return ""
}

// setReceivingMigrationFailed ends the request when the receiving migration fails. The source reads the
// request after the failure, and a bare "failed" tells it nothing: the cause KubeVirt found ends the chain of
// problems seen before.
func (h *SyncKvvmHandler) setReceivingMigrationFailed(ctx context.Context, s state.VirtualMachineState, r *intvirtv1alpha1.InternalVirtualizationVirtualMachineReceiver, received *virtv1.VirtualMachineInstanceMigration) error {
	kvvmi, err := s.KVVMI(ctx)
	if err != nil {
		return err
	}

	return peering.SetReceiverFailed(ctx, h.client, r, peering.ReceiverReasonMigrationFailed, receivingMigrationFailedMessage(r, received, kvvmi))
}

func receivingMigrationFailedMessage(r *intvirtv1alpha1.InternalVirtualizationVirtualMachineReceiver, received *virtv1.VirtualMachineInstanceMigration, kvvmi *virtv1.VirtualMachineInstance) string {
	if r.Status.Phase == intvirtv1alpha1.ReceiverFailed && r.Status.Message != "" {
		return r.Status.Message
	}

	failed := "the receiving migration failed"
	if reason := receivingMigrationFailureReason(received, kvvmi); reason != "" {
		failed += ": " + reason
	}

	if receivingVMShutDown(received) {
		failed = "the receiving virtual machine was shut down before the migration ended"
	}

	return peering.AppendReceiverProblems(r.Status.Message, failed)
}

// receivingVMShutDown tells a receiving migration that failed because its VMI or pod here went away: the
// words of KubeVirt for it ("vmi shutdown during migration") mean nothing to the user of the source.
func receivingVMShutDown(received *virtv1.VirtualMachineInstanceMigration) bool {
	for _, c := range received.Status.Conditions {
		if c.Type != virtv1.VirtualMachineInstanceMigrationFailed || c.Status != corev1.ConditionTrue {
			continue
		}

		switch c.Reason {
		case virtv1.VirtualMachineInstanceMigrationFailedReasonVMIIsShutdown,
			virtv1.VirtualMachineInstanceMigrationFailedReasonTargetPodShutdownDuringMigration,
			virtv1.VirtualMachineInstanceMigrationFailedReasonTargetPodDisappearedDuringMigration:
			return true
		}
	}

	return false
}

// The Failed condition carries the cause; failureReason is a fallback:
// KubeVirt 1.6 doesn't set it when the target pod disappears before the migration starts.
func receivingMigrationFailureReason(received *virtv1.VirtualMachineInstanceMigration, kvvmi *virtv1.VirtualMachineInstance) string {
	for _, c := range received.Status.Conditions {
		if c.Type == virtv1.VirtualMachineInstanceMigrationFailed && c.Status == corev1.ConditionTrue && c.Message != "" {
			return c.Message
		}
	}

	if st := received.Status.MigrationState; st != nil && st.FailureReason != "" {
		return st.FailureReason
	}

	if kvvmi != nil {
		if st := kvvmi.Status.MigrationState; st != nil && st.MigrationUID == received.UID && st.FailureReason != "" {
			return st.FailureReason
		}
	}

	return ""
}

// receivingPodMessage tells the source why the receiving pod does not start: otherwise the source sees only a
// migration that does not begin and blames the network. A pod that cannot be created, or is scheduled but
// cannot pull its image, shows in the Synchronized condition of the KVVM. Its message is passed on as is: the
// cause (an authorization error, a missing image, a denied admission) is at its end.
func receivingPodMessage(kvvm *virtv1.VirtualMachine) string {
	for _, c := range kvvm.Status.Conditions {
		if c.Type == virtv1.VirtualMachineConditionType(corev1.PodScheduled) && c.Status == corev1.ConditionFalse {
			return fmt.Sprintf("the receiving pod cannot be scheduled: %s: %s", c.Reason, c.Message)
		}
	}

	for _, c := range kvvm.Status.Conditions {
		if c.Type == conditions.VirtualMachineSynchronized && c.Status == corev1.ConditionFalse {
			return fmt.Sprintf("the receiving pod cannot start: %s: %s", c.Reason, c.Message)
		}
	}

	return ""
}

// finalizeReceivingVM turns the received VM into a regular one: KubeVirt leaves the receiving
// strategy on the KVVM, so the one the VM controller gives the run policy is set here, and the request
// confirms to the source that the VM runs here.
func (h *SyncKvvmHandler) finalizeReceivingVM(ctx context.Context, s state.VirtualMachineState, r *intvirtv1alpha1.InternalVirtualizationVirtualMachineReceiver) (bool, error) {
	vm := s.VirtualMachine().Changed()

	kvvm, err := s.KVVM(ctx)
	if err != nil {
		return false, err
	}

	if kvvm != nil {
		// Match kvbuilder.SetRunPolicy; explicitly restore Always for
		// AlwaysOnUnlessStoppedManually when leaving WaitAsReceiver.
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
	// The VM and its disks stop referring to the request before it confirms the arrival: the source deletes
	// the request then, and a reference left would hand the received VM to the garbage collector.
	vds, err := s.VirtualDisksByName(ctx)
	if err != nil {
		return false, err
	}

	for _, vd := range vds {
		if err = peering.ReleaseFromReceiver(ctx, h.client, vd); err != nil {
			return false, err
		}
	}

	if err = peering.ReleaseFromReceiver(ctx, h.client, s.VirtualMachine().Current()); err != nil {
		return false, err
	}

	if r != nil {
		if err = peering.PatchReceiverStatus(ctx, h.client, r, func(st *intvirtv1alpha1.VirtualMachineReceiverStatus) {
			st.Phase = intvirtv1alpha1.ReceiverRunning
			st.ReceiverStarted = true
		}); err != nil {
			return false, err
		}
	}
	// Running is recorded last; receiver cleanup keeps its finalizer until this point.
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
			return fmt.Sprintf("the disk %q in the receiving cluster is %d bytes, smaller than the disk in the source cluster (%d bytes)", v.Name, v.Size, want)
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
			return fmt.Sprintf("the disk %q in the receiving cluster has a capacity of %s, smaller than the disk in the source cluster (%d bytes)", name, vd.Status.Capacity, v.Size)
		}
	}

	return ""
}

// cancelVMReception undoes an unfinished receive: the receiving KubeVirt objects go. The VM is not
// started: the request that records it stays until it has deleted the VM with its disks.
func (h *SyncKvvmHandler) cancelVMReception(ctx context.Context, s state.VirtualMachineState, r *intvirtv1alpha1.InternalVirtualizationVirtualMachineReceiver) error {
	// With a forcibly removed receiver the migration identity is unknown. Keep the guest safe;
	// ordinary cancellation retains the receiver under its cleanup finalizer.
	if r == nil {
		return nil
	}

	vm := s.VirtualMachine().Changed()

	// Never delete a receiving VM whose guest already runs: a succeeded migration and a missing request
	// can race, and the completed path must win. The migration is checked as well as the instance: the
	// source removes the request as soon as its receiving migration succeeded, a moment before the instance
	// of this cluster reports Running, and the guest must not go with it.
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

	received, err := h.receivingMigration(ctx, vm, r)
	if err != nil {
		return err
	}

	if received != nil && received.Status.Phase == virtv1.MigrationSucceeded {
		return nil
	}

	if kvvm != nil && kvvm.Spec.RunStrategy != nil && *kvvm.Spec.RunStrategy == virtv1.RunStrategyWaitAsReceiver && kvvm.DeletionTimestamp == nil {
		if err = h.client.Delete(ctx, kvvm, client.PropagationPolicy(metav1.DeletePropagationForeground)); err != nil && !k8serrors.IsNotFound(err) {
			return fmt.Errorf("delete the receiving internal virtual machine: %w", err)
		}
	}

	if received != nil && received.Spec.Receive != nil && received.Spec.Receive.MigrationID == string(r.UID) && received.Spec.VMIName == vm.Name {
		if err = h.client.Delete(ctx, received, client.Preconditions{UID: &received.UID, ResourceVersion: &received.ResourceVersion}); err != nil && !k8serrors.IsNotFound(err) {
			return fmt.Errorf("delete the receiving migration: %w", err)
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

// peerMigrationInProgress identifies the outgoing operation from the KVVM annotation.
// The MovedToPeer condition guards against a second guest while the KVVM is not visible.
func peerMigrationInProgress(vm *v1alpha2.VirtualMachine, kvvm *virtv1.VirtualMachine) (string, bool) {
	if op := commonvm.PeerMigrationVMOP(kvvm); op != "" {
		return op, true
	}

	return "", kvvm == nil && vm != nil && meta.IsStatusConditionTrue(vm.Status.Conditions, vmcondition.TypeMovedToPeer.String())
}

// syncMovedToPeer shows in the status of the VM that it moves and to which cluster. The migration state is
// rebuilt from the KVVMI before, so prev keeps the peer while the operation is not seen.
func syncMovedToPeer(ctx context.Context, cl client.Client, vm *v1alpha2.VirtualMachine, kvvm *virtv1.VirtualMachine, prev *v1alpha2.VirtualMachineMigrationState) error {
	op, moved := peerMigrationInProgress(vm, kvvm)
	if !moved {
		conditions.RemoveCondition(vmcondition.TypeMovedToPeer, &vm.Status.Conditions)
		return nil
	}

	peer := ""
	if prev != nil {
		peer = prev.Target.ProjectPeerName
	}

	if op != "" {
		var vmop v1alpha2.VirtualMachineOperation

		err := cl.Get(ctx, types.NamespacedName{Namespace: vm.Namespace, Name: op}, &vmop)
		if client.IgnoreNotFound(err) != nil {
			return fmt.Errorf("get the operation that moves the virtual machine: %w", err)
		}

		if err == nil && commonvmop.PeerName(&vmop) != "" {
			peer = commonvmop.PeerName(&vmop)
		}

		msg := fmt.Sprintf("The virtual machine is being moved to a peer cluster by VirtualMachineOperation %q.", op)
		if peer != "" {
			msg = fmt.Sprintf("The virtual machine is being moved to peer cluster %q by VirtualMachineOperation %q.", peer, op)
		}

		conditions.SetCondition(conditions.NewConditionBuilder(vmcondition.TypeMovedToPeer).
			Generation(vm.GetGeneration()).
			Status(metav1.ConditionTrue).
			Reason(vmcondition.ReasonMovedToPeerInProgress).
			Message(msg),
			&vm.Status.Conditions)
	}

	if peer == "" {
		return nil
	}

	if vm.Status.MigrationState == nil {
		vm.Status.MigrationState = &v1alpha2.VirtualMachineMigrationState{}
	}

	vm.Status.MigrationState.Target.ProjectPeerName = peer

	return nil
}
