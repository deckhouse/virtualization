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

package peering

import (
	"context"
	"encoding/json"
	"fmt"
	"maps"
	"time"

	corev1 "k8s.io/api/core/v1"
	schedulingv1 "k8s.io/api/scheduling/v1"
	storagev1 "k8s.io/api/storage/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/ptr"
	virtv1 "kubevirt.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/deckhouse/virtualization-controller/pkg/common/annotations"
	"github.com/deckhouse/virtualization-controller/pkg/controller/kubevirt"
	"github.com/deckhouse/virtualization-controller/pkg/controller/kvbuilder"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
	intvirtv1alpha1 "github.com/deckhouse/virtualization/api/internalvirtualization/v1alpha1"
)

const (
	receiverControllerName = "virtualization-receiver-controller"
	// FinalizerReceiverCleanup keeps a request until the objects created for it are gone, when the VM has
	// not run here.
	FinalizerReceiverCleanup = "internal.virtualization.deckhouse.io/vmr-cleanup"
)

// Reasons of a failed receive, for the source cluster.
const (
	ReceiverReasonInvalidRequest        = "InvalidRequest"
	ReceiverReasonVirtualMachineExists  = "VirtualMachineExists"
	ReceiverReasonVirtualDiskExists     = "VirtualDiskExists"
	ReceiverReasonNotFound              = "NotFound"
	ReceiverReasonIPAddressNotAvailable = "IPAddressNotAvailable"
	ReceiverReasonDiskTooSmall          = "DiskTooSmall"
	ReceiverReasonMigrationFailed       = "MigrationFailed"
)

// ReceivedObjects is what a request asks this cluster to create.
type ReceivedObjects struct {
	VirtualMachine *v1alpha2.VirtualMachine
	VirtualDisks   []*v1alpha2.VirtualDisk
	// Instance is the KubeVirt VMI of the source: its domain runs here.
	Instance *virtv1.VirtualMachineInstance
}

// ParseReceiver reads the objects of a request.
func ParseReceiver(r *intvirtv1alpha1.InternalVirtualizationVirtualMachineReceiver) (*ReceivedObjects, error) {
	rcv := &ReceivedObjects{VirtualMachine: &v1alpha2.VirtualMachine{}, Instance: &virtv1.VirtualMachineInstance{}}
	if err := json.Unmarshal(r.Spec.VirtualMachineTemplate.Raw, rcv.VirtualMachine); err != nil {
		return nil, fmt.Errorf("spec.virtualMachineTemplate: %w", err)
	}
	for i, raw := range r.Spec.VirtualDiskTemplates {
		vd := &v1alpha2.VirtualDisk{}
		if err := json.Unmarshal(raw.Raw, vd); err != nil {
			return nil, fmt.Errorf("spec.virtualDiskTemplates[%d]: %w", i, err)
		}
		if vd.Name == "" {
			return nil, fmt.Errorf("spec.virtualDiskTemplates[%d] has no name", i)
		}
		rcv.VirtualDisks = append(rcv.VirtualDisks, vd)
	}
	if err := json.Unmarshal(r.Spec.InternalVirtualizationVirtualMachineInstance.Raw, rcv.Instance); err != nil {
		return nil, fmt.Errorf("spec.internalVirtualizationVirtualMachineInstance: %w", err)
	}
	return rcv, nil
}

// ReceiverReconciler creates in this cluster the VM a peer asks to receive: its VirtualDisks, blank and of
// the same size, and the VM itself, which the VM controller then turns into the receiving KubeVirt VM.
// The objects are marked with the request and not owned by it: deleting the request after the switchover
// must not delete the running VM. While the VM has not run here, the request removes them on deletion.
type ReceiverReconciler struct {
	client    client.Client
	namespace string
}

// LabelReceiverVMOPUID marks the request with the operation it serves: peering-<uid of the vmop in the source>.
const LabelReceiverVMOPUID = "internal.virtualization.deckhouse.io/vmop-uid"

// receiverVMOPUIDPrefix is a wire format: requests live in the peer cluster, so it must not change with
// the name of this module.
const receiverVMOPUIDPrefix = "peering-"

// ReceiverVMOPUID is the value of LabelReceiverVMOPUID for an operation.
func ReceiverVMOPUID(vmop *v1alpha2.VirtualMachineOperation) string {
	return receiverVMOPUIDPrefix + string(vmop.GetUID())
}

func (r *ReceiverReconciler) Reconcile(ctx context.Context, req reconcile.Request) (reconcile.Result, error) {
	var rcvr intvirtv1alpha1.InternalVirtualizationVirtualMachineReceiver
	if err := r.client.Get(ctx, req.NamespacedName, &rcvr); err != nil {
		return reconcile.Result{}, client.IgnoreNotFound(err)
	}
	if !rcvr.DeletionTimestamp.IsZero() {
		return r.cleanup(ctx, &rcvr)
	}
	if controllerutil.AddFinalizer(&rcvr, FinalizerReceiverCleanup) {
		if err := r.client.Update(ctx, &rcvr); err != nil {
			return reconcile.Result{}, err
		}
	}
	// The source deletes the request when the move ends. One it left behind, because it lost the peer or
	// confirmed nothing, goes here: a failed one once the source had time to read why, a running one after
	// a day; the VM that ran here stays either way. A request whose move never started goes too: the
	// source gives up within minutes, so a longer wait means it is gone, and the request would otherwise
	// keep a VM with blank disks here for good, and only the module itself may delete it.
	// A request failed here may still receive the guest: a migration already copying is not stopped by
	// the failure, and the source must learn of the arrival.
	if rcvr.Status.Phase != intvirtv1alpha1.ReceiverRunning {
		if err := r.confirmArrival(ctx, &rcvr); err != nil {
			return reconcile.Result{}, err
		}
	}
	if rcvr.Status.Phase != intvirtv1alpha1.ReceiverRunning && rcvr.Status.Phase != intvirtv1alpha1.ReceiverFailed {
		if err := r.publishExchange(ctx, &rcvr); err != nil {
			return reconcile.Result{}, err
		}
		if !rcvr.CreationTimestamp.IsZero() && time.Since(rcvr.CreationTimestamp.Time) > receiverIdleTTL && !rcvr.Status.ReceiverStarted {
			state, err := r.cleanupState(ctx, &rcvr)
			if err != nil {
				return reconcile.Result{}, err
			}
			if state == receiverCleanupUnstarted {
				return reconcile.Result{}, client.IgnoreNotFound(r.client.Delete(ctx, &rcvr))
			}
		}
	}
	if rcvr.Status.Phase == intvirtv1alpha1.ReceiverRunning || rcvr.Status.Phase == intvirtv1alpha1.ReceiverFailed {
		ttl := receiverRunningTTL
		if rcvr.Status.Phase == intvirtv1alpha1.ReceiverFailed {
			ttl = receiverFailedTTL
		}
		if left := ttl - time.Since(receiverPhaseSince(&rcvr)); left > 0 {
			return reconcile.Result{RequeueAfter: left}, nil
		}
		return reconcile.Result{}, client.IgnoreNotFound(r.client.Delete(ctx, &rcvr))
	}

	rcv, err := ParseReceiver(&rcvr)
	if err != nil {
		return reconcile.Result{}, SetReceiverFailed(ctx, r.client, &rcvr, ReceiverReasonInvalidRequest, err.Error())
	}
	uid := string(rcvr.UID)

	var vm v1alpha2.VirtualMachine
	err = r.client.Get(ctx, types.NamespacedName{Namespace: rcvr.Namespace, Name: rcvr.Name}, &vm)
	switch {
	case err == nil && vm.Labels[annotations.LabelVMReceiver] != uid && vm.DeletionTimestamp != nil:
		// Most likely the copy of this VM that has just moved away: wait for it to go.
		return reconcile.Result{RequeueAfter: receiverCleanupRequeue}, r.setPending(ctx, &rcvr)
	case err == nil && vm.Labels[annotations.LabelVMReceiver] != uid:
		return reconcile.Result{}, SetReceiverFailed(ctx, r.client, &rcvr, ReceiverReasonVirtualMachineExists,
			fmt.Sprintf("another virtual machine named %q exists in namespace %q. Rename or delete it there and retry the move", rcvr.Name, rcvr.Namespace))
	case err == nil:
		return reconcile.Result{RequeueAfter: receiverExchangeResync}, r.setPending(ctx, &rcvr)
	case !k8serrors.IsNotFound(err):
		return reconcile.Result{}, err
	}

	if reason, msg, err := r.check(ctx, rcv, rcvr.Namespace); err != nil || msg != "" {
		if err != nil {
			return reconcile.Result{}, err
		}
		return reconcile.Result{}, SetReceiverFailed(ctx, r.client, &rcvr, reason, msg)
	}

	for _, src := range rcv.VirtualDisks {
		vd := receivedDisk(src, rcv.Instance, rcvr.Namespace, uid)
		if err = r.client.Create(ctx, vd); k8serrors.IsAlreadyExists(err) {
			var existing v1alpha2.VirtualDisk
			if err = r.client.Get(ctx, client.ObjectKeyFromObject(vd), &existing); err != nil {
				return reconcile.Result{}, err
			}
			if existing.Labels[annotations.LabelVMReceiver] != uid && existing.DeletionTimestamp != nil {
				return reconcile.Result{RequeueAfter: receiverCleanupRequeue}, r.setPending(ctx, &rcvr)
			}
			if existing.Labels[annotations.LabelVMReceiver] != uid {
				return reconcile.Result{}, SetReceiverFailed(ctx, r.client, &rcvr, ReceiverReasonVirtualDiskExists,
					fmt.Sprintf("another virtual disk named %q exists in namespace %q. Rename or delete it there and retry the move", vd.Name, rcvr.Namespace))
			}
		} else if err != nil {
			return reconcile.Result{}, SetReceiverFailed(ctx, r.client, &rcvr, ReceiverReasonInvalidRequest,
				fmt.Sprintf("the virtual disk %q cannot be created: %s", vd.Name, err))
		}
	}

	ipName, err := r.freeIPAddress(ctx, rcvr.Namespace, ReceiverIPAddress(&rcvr))
	if err != nil {
		return reconcile.Result{}, err
	}
	newVM := receivedVM(rcv.VirtualMachine, rcvr.Namespace, rcvr.Name, uid, ipName)
	if err = r.client.Create(ctx, newVM); err != nil && !k8serrors.IsAlreadyExists(err) {
		return reconcile.Result{}, SetReceiverFailed(ctx, r.client, &rcvr, ReceiverReasonInvalidRequest,
			fmt.Sprintf("the virtual machine cannot be created: %s", err))
	}
	return reconcile.Result{}, r.setPending(ctx, &rcvr)
}

// publishExchange hands across what the synchronization controllers of the clusters need to trust each
// other: the address of the controller here and the pin of the key it generated for this migration go to
// the source in the status; the pin of the source comes in an annotation of the request and goes into the
// receiving migration. A controller generates the key anew after a restart, so the pins are read again and
// again.
func (r *ReceiverReconciler) publishExchange(ctx context.Context, rcvr *intvirtv1alpha1.InternalVirtualizationVirtualMachineReceiver) error {
	address, err := LocalSynchronizationAddress(ctx, r.client, r.namespace)
	if err != nil {
		return err
	}
	pin, err := MigrationSyncPin(ctx, r.client, types.NamespacedName{Namespace: rcvr.Namespace, Name: rcvr.Name}, "targetState")
	if err != nil {
		return err
	}
	migration := types.NamespacedName{Namespace: rcvr.Namespace, Name: string(rcvr.UID)}
	if err = SetMigrationPeerSyncPin(ctx, r.client, migration, rcvr.Annotations[annotations.AnnReceiverSourceSyncPin]); err != nil {
		return err
	}
	return PatchReceiverStatus(ctx, r.client, rcvr, func(st *intvirtv1alpha1.VirtualMachineReceiverStatus) {
		st.SynchronizationAddress, st.SyncPin = address, pin
	})
}

type receiverCleanupState int

const (
	receiverCleanupUnstarted receiverCleanupState = iota
	receiverCleanupDeferred
	receiverCleanupArrived
)

func (r *ReceiverReconciler) cleanupState(ctx context.Context, rcvr *intvirtv1alpha1.InternalVirtualizationVirtualMachineReceiver) (receiverCleanupState, error) {
	if rcvr.Status.ReceiverStarted {
		return receiverCleanupArrived, nil
	}
	key := types.NamespacedName{Namespace: rcvr.Namespace, Name: string(rcvr.UID)}
	var vmim virtv1.VirtualMachineInstanceMigration
	migrationErr := r.client.Get(ctx, key, &vmim)
	if migrationErr != nil && !k8serrors.IsNotFound(migrationErr) {
		return receiverCleanupDeferred, migrationErr
	}
	if migrationErr == nil && (vmim.Spec.Receive == nil || vmim.Spec.Receive.MigrationID != string(rcvr.UID) || vmim.Spec.VMIName != rcvr.Name) {
		return receiverCleanupDeferred, nil
	}
	// The VMI of a VM this request did not create, which a request refused for the name meets, is not its
	// business.
	var vm v1alpha2.VirtualMachine
	vmErr := r.client.Get(ctx, types.NamespacedName{Namespace: rcvr.Namespace, Name: rcvr.Name}, &vm)
	if vmErr != nil && !k8serrors.IsNotFound(vmErr) {
		return receiverCleanupDeferred, vmErr
	}
	if migrationErr != nil && vmErr == nil && vm.Labels[annotations.LabelVMReceiver] != string(rcvr.UID) {
		return receiverCleanupUnstarted, nil
	}
	var vmi virtv1.VirtualMachineInstance
	vmiErr := r.client.Get(ctx, types.NamespacedName{Namespace: rcvr.Namespace, Name: rcvr.Name}, &vmi)
	if vmiErr != nil && !k8serrors.IsNotFound(vmiErr) {
		return receiverCleanupDeferred, vmiErr
	}
	if vmiErr == nil {
		if vmi.Status.Phase == virtv1.Running {
			return receiverCleanupDeferred, nil
		}
		// A receiving migration that has waited for the source all along never started: the source gave up.
		if migrationErr == nil && vmim.Status.Phase != virtv1.MigrationSucceeded && vmim.Status.Phase != virtv1.MigrationFailed &&
			!migrationStarted(vmim.Status.MigrationState) && !migrationStarted(vmi.Status.MigrationState) {
			return receiverCleanupUnstarted, nil
		}
		// Not (failed and completed): anything but a known failure while the migration has not ended. The
		// receiving VMI is never completed on a failure: the source passes on only the failure.
		ended := migrationErr == nil && (vmim.Status.Phase == virtv1.MigrationSucceeded || vmim.Status.Phase == virtv1.MigrationFailed)
		if state := vmi.Status.MigrationState; state != nil && (!state.Failed || !state.Completed) && !ended {
			return receiverCleanupDeferred, nil
		}
	}
	if migrationErr == nil {
		if vmim.Status.Phase == virtv1.MigrationSucceeded {
			return receiverCleanupArrived, nil
		}
		if vmim.Status.Phase != virtv1.MigrationFailed {
			return receiverCleanupDeferred, nil
		}
	}
	return receiverCleanupUnstarted, nil
}

// migrationStarted tells a migration state that shows any progress past the wait for the source.
func migrationStarted(state *virtv1.VirtualMachineInstanceMigrationState) bool {
	return state != nil && (state.StartTimestamp != nil || state.EndTimestamp != nil || state.Completed || state.Failed)
}

// confirmArrival tells the source that the guest is here as soon as the receiving migration succeeded. The
// controller of the VM lifts the receive mode and reports Running after that, and a delay there must not
// look to the source like a guest that never arrived.
func (r *ReceiverReconciler) confirmArrival(ctx context.Context, rcvr *intvirtv1alpha1.InternalVirtualizationVirtualMachineReceiver) error {
	if rcvr.Status.ReceiverStarted {
		return nil
	}
	var vmim virtv1.VirtualMachineInstanceMigration
	if err := r.client.Get(ctx, types.NamespacedName{Namespace: rcvr.Namespace, Name: string(rcvr.UID)}, &vmim); err != nil {
		return client.IgnoreNotFound(err)
	}
	if vmim.Status.Phase != virtv1.MigrationSucceeded || vmim.Spec.Receive == nil || vmim.Spec.Receive.MigrationID != string(rcvr.UID) || vmim.Spec.VMIName != rcvr.Name {
		return nil
	}
	return PatchReceiverStatus(ctx, r.client, rcvr, func(st *intvirtv1alpha1.VirtualMachineReceiverStatus) {
		st.ReceiverStarted = true
	})
}

// LocalSynchronizationAddress is where the KubeVirt synchronization controller of this cluster listens, as
// virt-operator publishes it; empty while there is no leader.
func LocalSynchronizationAddress(ctx context.Context, c client.Client, namespace string) (string, error) {
	kv, err := kubevirt.GetKubeVirt(ctx, c, namespace)
	if err != nil || len(kv.Status.SynchronizationAddresses) == 0 {
		return "", err
	}
	return kv.Status.SynchronizationAddresses[0], nil
}

// MigrationSyncPin is the pin of the key the synchronization controller of this cluster generated for the
// migration of the VMI, from the state of the given side, sourceState or targetState; empty until the
// controller handles the migration. The field is newer than the KubeVirt API in use, so it is read untyped.
func MigrationSyncPin(ctx context.Context, reader client.Reader, vmi types.NamespacedName, side string) (string, error) {
	obj := &unstructured.Unstructured{}
	obj.SetGroupVersionKind(virtv1.VirtualMachineInstanceGroupVersionKind)
	if err := reader.Get(ctx, vmi, obj); err != nil {
		return "", client.IgnoreNotFound(err)
	}
	pin, _, err := unstructured.NestedString(obj.Object, "status", "migrationState", side, "syncPin")
	return pin, err
}

// SetMigrationPeerSyncPin gives the KubeVirt migration the pin of the synchronization controller of the
// peer, which the controller here checks the peer against. Nothing happens while the pin or the migration
// is not there yet.
func SetMigrationPeerSyncPin(ctx context.Context, cl client.Client, migration types.NamespacedName, pin string) error {
	if pin == "" {
		return nil
	}
	var vmim virtv1.VirtualMachineInstanceMigration
	if err := cl.Get(ctx, migration, &vmim); err != nil {
		return client.IgnoreNotFound(err)
	}
	if vmim.Annotations[annotations.AnnMigrationPeerSyncPin] == pin {
		return nil
	}
	patch, _ := json.Marshal(map[string]any{"metadata": map[string]any{"annotations": map[string]string{annotations.AnnMigrationPeerSyncPin: pin}}})
	if err := cl.Patch(ctx, &vmim, client.RawPatch(types.MergePatchType, patch)); err != nil {
		return fmt.Errorf("give the receiving migration the pin of the peer: %w", err)
	}
	return nil
}

func (r *ReceiverReconciler) setPending(ctx context.Context, rcvr *intvirtv1alpha1.InternalVirtualizationVirtualMachineReceiver) error {
	if rcvr.Status.Phase != "" {
		return nil
	}
	return PatchReceiverStatus(ctx, r.client, rcvr, func(st *intvirtv1alpha1.VirtualMachineReceiverStatus) {
		st.Phase = intvirtv1alpha1.ReceiverPending
	})
}

// check tells what this cluster lacks to receive the VM: the clusters are assumed to have the same classes,
// StorageClasses and PriorityClasses, and a missing one fails the move before anything is created. The
// provisioning secret does not travel with the VM, so the project here must have it.
func (r *ReceiverReconciler) check(ctx context.Context, rcv *ReceivedObjects, namespace string) (string, string, error) {
	spec := rcv.VirtualMachine.Spec
	if err := r.client.Get(ctx, types.NamespacedName{Name: spec.VirtualMachineClassName}, &v1alpha2.VirtualMachineClass{}); err != nil {
		if k8serrors.IsNotFound(err) {
			return ReceiverReasonNotFound, fmt.Sprintf("the VirtualMachineClass %q does not exist", spec.VirtualMachineClassName), nil
		}
		return "", "", err
	}
	if spec.PriorityClassName != "" {
		if err := r.client.Get(ctx, types.NamespacedName{Name: spec.PriorityClassName}, &schedulingv1.PriorityClass{}); err != nil {
			if k8serrors.IsNotFound(err) {
				return ReceiverReasonNotFound, fmt.Sprintf("the PriorityClass %q does not exist", spec.PriorityClassName), nil
			}
			return "", "", err
		}
	}
	for _, vd := range rcv.VirtualDisks {
		sc := ptr.Deref(vd.Spec.PersistentVolumeClaim.StorageClass, "")
		if sc == "" {
			continue
		}
		if err := r.client.Get(ctx, types.NamespacedName{Name: sc}, &storagev1.StorageClass{}); err != nil {
			if k8serrors.IsNotFound(err) {
				return ReceiverReasonNotFound, fmt.Sprintf("the StorageClass %q of the disk %q does not exist", sc, vd.Name), nil
			}
			return "", "", err
		}
	}
	if p := spec.Provisioning; p != nil {
		secret := ""
		switch {
		case p.Type == v1alpha2.ProvisioningTypeUserDataRef && p.UserDataRef != nil:
			secret = p.UserDataRef.Name
		case p.Type == v1alpha2.ProvisioningTypeSysprepRef && p.SysprepRef != nil:
			secret = p.SysprepRef.Name
		}
		if secret != "" {
			if err := r.client.Get(ctx, types.NamespacedName{Namespace: namespace, Name: secret}, &corev1.Secret{}); err != nil {
				if k8serrors.IsNotFound(err) {
					return ReceiverReasonNotFound, fmt.Sprintf("the provisioning Secret %q does not exist in namespace %q: create it there before the move", secret, namespace), nil
				}
				return "", "", err
			}
		}
	}
	return "", "", nil
}

// freeIPAddress returns a VirtualMachineIPAddress with the address that no VM uses, left for example by an
// earlier move: the VM takes it instead of failing on a second address object.
func (r *ReceiverReconciler) freeIPAddress(ctx context.Context, namespace, address string) (string, error) {
	if address == "" {
		return "", nil
	}
	var vmips v1alpha2.VirtualMachineIPAddressList
	if err := r.client.List(ctx, &vmips, client.InNamespace(namespace)); err != nil {
		return "", err
	}
	for _, vmip := range vmips.Items {
		if vmip.Status.Address == address && vmip.Status.VirtualMachine == "" && vmip.DeletionTimestamp.IsZero() {
			return vmip.Name, nil
		}
	}
	return "", nil
}

// receivedDisk is the blank VirtualDisk that receives a disk of the source: the same StorageClass and
// size, and the serial of the device of the source.
func receivedDisk(src *v1alpha2.VirtualDisk, instance *virtv1.VirtualMachineInstance, namespace, uid string) *v1alpha2.VirtualDisk {
	vd := &v1alpha2.VirtualDisk{
		TypeMeta: metav1.TypeMeta{APIVersion: v1alpha2.SchemeGroupVersion.String(), Kind: v1alpha2.VirtualDiskKind},
		ObjectMeta: metav1.ObjectMeta{
			Name:        src.Name,
			Namespace:   namespace,
			Labels:      withValue(src.Labels, annotations.LabelVMReceiver, uid),
			Annotations: maps.Clone(src.Annotations),
		},
		Spec: v1alpha2.VirtualDiskSpec{PersistentVolumeClaim: v1alpha2.VirtualDiskPersistentVolumeClaim{
			StorageClass: src.Spec.PersistentVolumeClaim.StorageClass,
			Size:         src.Spec.PersistentVolumeClaim.Size,
		}},
	}
	diskName := kvbuilder.GenerateVDDiskName(src.Name)
	for _, d := range instance.Spec.Domain.Devices.Disks {
		if d.Name == diskName && d.Serial != "" {
			vd.Annotations = withValue(vd.Annotations, annotations.AnnVDSerial, d.Serial)
		}
	}
	return vd
}

// receivedVM is the VM of this cluster: the spec of the source as it runs there, with the address object of
// this cluster. The MAC addresses of the source are in its domain, which the VM runs with.
func receivedVM(src *v1alpha2.VirtualMachine, namespace, name, uid, ipName string) *v1alpha2.VirtualMachine {
	spec := *src.Spec.DeepCopy()
	spec.VirtualMachineIPAddress = ipName
	for i := range spec.Networks {
		spec.Networks[i].VirtualMachineMACAddressName = ""
		// The IPAddress of the source is not here: the VM controller reserves the address of the source.
		if spec.Networks[i].Type == v1alpha2.NetworksTypeMain {
			spec.Networks[i].IPAddressName = ""
		}
	}
	return &v1alpha2.VirtualMachine{
		TypeMeta: metav1.TypeMeta{APIVersion: v1alpha2.SchemeGroupVersion.String(), Kind: v1alpha2.VirtualMachineKind},
		ObjectMeta: metav1.ObjectMeta{
			Name:        name,
			Namespace:   namespace,
			Labels:      withValue(src.Labels, annotations.LabelVMReceiver, uid),
			Annotations: withValue(src.Annotations, annotations.AnnVMMigrationReceiver, uid),
		},
		Spec: spec,
	}
}

// cleanup removes the objects created for the request when the VM has not run here: they hold blank disks.
// After the VM has run, they are a regular VM of this cluster and stay.
func (r *ReceiverReconciler) cleanup(ctx context.Context, rcvr *intvirtv1alpha1.InternalVirtualizationVirtualMachineReceiver) (reconcile.Result, error) {
	if !controllerutil.ContainsFinalizer(rcvr, FinalizerReceiverCleanup) {
		return reconcile.Result{}, nil
	}
	state, err := r.cleanupState(ctx, rcvr)
	if err != nil {
		return reconcile.Result{}, err
	}
	if state == receiverCleanupDeferred {
		return reconcile.Result{RequeueAfter: receiverCleanupRequeue}, nil
	}
	if state == receiverCleanupUnstarted {
		marked := client.MatchingLabels{annotations.LabelVMReceiver: string(rcvr.UID)}
		var vms v1alpha2.VirtualMachineList
		if err := r.client.List(ctx, &vms, client.InNamespace(rcvr.Namespace), marked); err != nil {
			return reconcile.Result{}, err
		}
		var vds v1alpha2.VirtualDiskList
		if err := r.client.List(ctx, &vds, client.InNamespace(rcvr.Namespace), marked); err != nil {
			return reconcile.Result{}, err
		}
		for i := range vms.Items {
			if err := r.client.Delete(ctx, &vms.Items[i]); client.IgnoreNotFound(err) != nil {
				return reconcile.Result{}, err
			}
		}
		for i := range vds.Items {
			if err := r.client.Delete(ctx, &vds.Items[i]); client.IgnoreNotFound(err) != nil {
				return reconcile.Result{}, err
			}
		}
		if len(vms.Items)+len(vds.Items) > 0 {
			return reconcile.Result{RequeueAfter: receiverCleanupRequeue}, nil
		}
	}
	controllerutil.RemoveFinalizer(rcvr, FinalizerReceiverCleanup)
	return reconcile.Result{}, r.client.Update(ctx, rcvr)
}

const (
	receiverCleanupRequeue = 2 * time.Second
	receiverExchangeResync = 30 * time.Second
	receiverFailedTTL      = time.Hour
	receiverRunningTTL     = 24 * time.Hour
	// receiverIdleTTL bounds how long a request waits for its move to start: the source gives up after its
	// own timeouts of 5 minutes each for the preparation and the synchronization.
	receiverIdleTTL = 30 * time.Minute
)

// ReceiverIPAddress is the address the VM keeps here, empty when it gets an address of this cluster.
func ReceiverIPAddress(r *intvirtv1alpha1.InternalVirtualizationVirtualMachineReceiver) string {
	if r == nil || r.Annotations[annotations.AnnReceiverNewIPAddress] == "true" {
		return ""
	}
	return r.Spec.IPAddress
}

// SetReceiverFailed ends the receive with a reason the source shows to its user.
func SetReceiverFailed(ctx context.Context, cl client.Client, r *intvirtv1alpha1.InternalVirtualizationVirtualMachineReceiver, reason, message string) error {
	return PatchReceiverStatus(ctx, cl, r, func(st *intvirtv1alpha1.VirtualMachineReceiverStatus) {
		st.Phase = intvirtv1alpha1.ReceiverFailed
		st.Reason = reason
		st.Message = message
	})
}

func PatchReceiverStatus(ctx context.Context, cl client.Client, r *intvirtv1alpha1.InternalVirtualizationVirtualMachineReceiver, mutate func(*intvirtv1alpha1.VirtualMachineReceiverStatus)) error {
	before := r.DeepCopy()
	mutate(&r.Status)
	r.Status.ObservedGeneration = r.Generation
	if before.Status != r.Status {
		if err := cl.Status().Patch(ctx, r, client.MergeFrom(before)); err != nil {
			if k8serrors.IsNotFound(err) {
				return nil
			}
			return err
		}
	}
	// The TTL of a failed or running request counts from the phase it is in, not from the creation of
	// the request. The status subresource cannot carry the timestamp, so it goes into an annotation.
	if r.Status.Phase != intvirtv1alpha1.ReceiverRunning && r.Status.Phase != intvirtv1alpha1.ReceiverFailed {
		return nil
	}
	if before.Status.Phase == r.Status.Phase && r.Annotations[annotations.AnnReceiverPhaseSince] != "" {
		return nil
	}
	since := time.Now().UTC().Format(time.RFC3339)
	patch, _ := json.Marshal(map[string]any{"metadata": map[string]any{"annotations": map[string]string{annotations.AnnReceiverPhaseSince: since}}})
	if err := cl.Patch(ctx, r, client.RawPatch(types.MergePatchType, patch)); client.IgnoreNotFound(err) != nil {
		return err
	}
	if r.Annotations == nil {
		r.Annotations = map[string]string{}
	}
	r.Annotations[annotations.AnnReceiverPhaseSince] = since
	return nil
}

// receiverPhaseSince is when the request entered its current phase: the TTL of a failed or running
// request must not count the time it spent preparing before the phase began.
func receiverPhaseSince(r *intvirtv1alpha1.InternalVirtualizationVirtualMachineReceiver) time.Time {
	if value := r.Annotations[annotations.AnnReceiverPhaseSince]; value != "" {
		if since, err := time.Parse(time.RFC3339, value); err == nil {
			return since
		}
	}
	return r.CreationTimestamp.Time
}

func withValue(m map[string]string, key, value string) map[string]string {
	out := maps.Clone(m)
	if out == nil {
		out = map[string]string{}
	}
	out[key] = value
	return out
}

// SetupReceiverController starts the creation of the received VMs. The VMI of a receiving VM, named like
// the request, carries the pin of the synchronization controller to publish; the receiving migration tells
// when the guest arrived.
func SetupReceiverController(mgr manager.Manager, namespace string) error {
	receivingVMI := predicate.NewPredicateFuncs(func(o client.Object) bool {
		return o.GetAnnotations()[virtv1.CreateMigrationTarget] == "true"
	})
	sameName := handler.EnqueueRequestsFromMapFunc(func(_ context.Context, o client.Object) []reconcile.Request {
		return []reconcile.Request{{NamespacedName: client.ObjectKeyFromObject(o)}}
	})
	receivingMigration := handler.EnqueueRequestsFromMapFunc(func(_ context.Context, o client.Object) []reconcile.Request {
		vmim, ok := o.(*virtv1.VirtualMachineInstanceMigration)
		if !ok || vmim.Spec.Receive == nil {
			return nil
		}
		return []reconcile.Request{{NamespacedName: types.NamespacedName{Namespace: vmim.Namespace, Name: vmim.Spec.VMIName}}}
	})
	return builder.ControllerManagedBy(mgr).
		Named(receiverControllerName).
		For(&intvirtv1alpha1.InternalVirtualizationVirtualMachineReceiver{}).
		Watches(&virtv1.VirtualMachineInstance{}, sameName, builder.WithPredicates(receivingVMI)).
		Watches(&virtv1.VirtualMachineInstanceMigration{}, receivingMigration).
		Complete(&ReceiverReconciler{client: mgr.GetClient(), namespace: namespace})
}
