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
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"strings"
	"time"

	corev1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	virtv1 "kubevirt.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/deckhouse/virtualization-controller/pkg/common/annotations"
	commonvmop "github.com/deckhouse/virtualization-controller/pkg/common/vmop"
	"github.com/deckhouse/virtualization-controller/pkg/controller/conditions"
	"github.com/deckhouse/virtualization-controller/pkg/controller/kvbuilder"
	"github.com/deckhouse/virtualization-controller/pkg/controller/peering"
	"github.com/deckhouse/virtualization-controller/pkg/controller/reconciler"
	"github.com/deckhouse/virtualization-controller/pkg/controller/vmop/migration/internal/service"
	"github.com/deckhouse/virtualization-controller/pkg/eventrecord"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/vmopcondition"
	intvirtv1alpha1 "github.com/deckhouse/virtualization/api/internalvirtualization/v1alpha1"
)

const (
	peerHandlerName = "PeerMigrationHandler"

	// labelPeerVMOPUID marks the request the source created in the peer for a migration: only it is
	// removed on a rollback, a request with the same name that is not marked belongs to another migration.
	labelPeerVMOPUID = peering.LabelReceiverVMOPUID

	peerRequeue = 5 * time.Second
	// peerSyncTimeout bounds the wait for the synchronization with the peer to start: KubeVirt retries the
	// synchronization controller of the peer with no overall timeout, so an unreachable peer would hang.
	peerSyncTimeout = 5 * time.Minute
	// peerConfirmTimeout bounds the wait for the peer to confirm that the VM runs there.
	peerConfirmTimeout = 10 * time.Minute
	// peerPrepareTimeout bounds the wait for the peer to prepare the receiver: a request it cannot serve
	// (a disk that never binds, say) would otherwise wait for good.
	peerPrepareTimeout = 5 * time.Minute
	// peerPinTimeout bounds the wait for the address and the pin of the synchronization controller of a
	// ready receiver. The receiver carries no time it became ready, so the wait counts from its creation,
	// after the time it may have taken to prepare.
	peerPinTimeout = 5 * time.Minute
	// peerDeletionTimeout bounds the cancel of an operation whose peer cannot be asked: the source releases
	// the VM and leaves the request to the peer TTL instead of locking the VM for good.
	peerDeletionTimeout = 10 * time.Minute

	// annRunStrategyBeforeMove keeps on the KubeVirt VM its run strategy from before the move, to restore
	// it on a rollback: the VM controller applies a changed run strategy only on a restart.
	annRunStrategyBeforeMove = "virtualization.deckhouse.io/run-strategy-before-move"
)

// PeerMigrationHandler moves a VM to a peer cluster: a Migrate operation with
// spec.migrate.projectPeerName. It asks the peer to receive the VM with a InternalVirtualizationVirtualMachineReceiver in the
// namespace of the same name, starts the migration to the peer once it is ready and deletes the VM here
// when the peer confirms that the VM runs there. Every step is derived from what exists in both clusters,
// so a reconcile can stop anywhere.
type PeerMigrationHandler struct {
	client    client.Client
	peers     PeerClients
	migration *service.MigrationService
	recorder  eventrecord.EventRecorderLogger
}

// PeerClients gives clients to peer clusters (peering.PeerClients).
type PeerClients interface {
	Client(ctx context.Context, peer string) (client.Client, error)
	Target(ctx context.Context, peer, namespace string) (peering.Target, error)
	Paired(ctx context.Context, peer, namespace string) error
}

func NewPeerMigrationHandler(client client.Client, peers PeerClients, migration *service.MigrationService, recorder eventrecord.EventRecorderLogger) *PeerMigrationHandler {
	return &PeerMigrationHandler{client: client, peers: peers, migration: migration, recorder: recorder}
}

func (h PeerMigrationHandler) Name() string {
	return peerHandlerName
}

// IsPeerMigration tells a migration to a peer cluster.
func IsPeerMigration(vmop *v1alpha2.VirtualMachineOperation) bool {
	return vmop != nil && vmop.Spec.Type == v1alpha2.VMOPTypeMigrate && commonvmop.PeerName(vmop) != ""
}

func (h PeerMigrationHandler) Handle(ctx context.Context, vmop *v1alpha2.VirtualMachineOperation) (reconcile.Result, error) {
	if !IsPeerMigration(vmop) {
		return reconcile.Result{}, nil
	}
	res, err := h.handle(ctx, vmop)
	if err != nil {
		if released, releaseErr := h.releaseOnDeletionTimeout(ctx, vmop); releaseErr != nil {
			return res, releaseErr
		} else if released {
			return res, reconciler.ErrStopHandlerChain
		}
		h.progress(vmop, vmopcondition.ReasonOperationFailed, fmt.Sprintf("The migration handler failed: %s", err))
		h.recorder.Event(vmop, corev1.EventTypeWarning, v1alpha2.ReasonErrVMOPFailed, err.Error())
		return res, err
	}
	return res, reconciler.ErrStopHandlerChain
}

func (h PeerMigrationHandler) handle(ctx context.Context, vmop *v1alpha2.VirtualMachineOperation) (reconcile.Result, error) {
	deleting := !vmop.DeletionTimestamp.IsZero()
	if commonvmop.IsFinished(vmop) {
		if vmop.Status.Phase == v1alpha2.VMOPPhaseFailed {
			if err := h.cleanupFailed(ctx, vmop); err != nil {
				return reconcile.Result{}, err
			}
		}
		if deleting {
			controllerutil.RemoveFinalizer(vmop, v1alpha2.FinalizerVMOPCleanup)
		}
		return reconcile.Result{}, nil
	}
	if vmop.Status.Phase == "" {
		vmop.Status.Phase = v1alpha2.VMOPPhasePending
	}

	src, err := h.migration.GetMigration(ctx, vmop)
	if err != nil {
		return reconcile.Result{}, err
	}
	// After the switchover the guest runs in the peer: there is nothing to cancel, only to finish, and a
	// peer that cannot be asked now only delays the confirmation.
	switched := src != nil && src.Status.Phase == virtv1.MigrationSucceeded

	peer := commonvmop.PeerName(vmop)
	peerClient, err := h.peers.Client(ctx, peer)
	switch {
	case err == nil:
	case switched:
		peerClient = nil
	case errors.Is(err, peering.ErrPeerNotSupported) || k8serrors.IsUnauthorized(err) || k8serrors.IsForbidden(err):
		// Without a migration here the guest cannot have switched over: a retry changes nothing, the peer
		// removes the request by its TTL.
		if !deleting && src == nil {
			if errors.Is(err, peering.ErrPeerNotSupported) {
				return reconcile.Result{}, h.fail(ctx, vmop, nil, vmopcondition.ReasonPeerNotSupported, err.Error())
			}
			return reconcile.Result{}, h.fail(ctx, vmop, nil, vmopcondition.ReasonPeerRejected, refusedMessage(peer, err))
		}
		if released, releaseErr := h.releaseOnDeletionTimeout(ctx, vmop); released || releaseErr != nil {
			return reconcile.Result{}, releaseErr
		}
		return h.waitForPeer(vmop)
	default:
		return reconcile.Result{}, err
	}

	var receiver *intvirtv1alpha1.InternalVirtualizationVirtualMachineReceiver
	if peerClient != nil {
		receiver, err = h.getReceiver(ctx, peerClient, vmop)
		switch {
		case err == nil:
		case switched:
			// Once switched, finish without a receiver: the timeout hands the VM back stopped.
			peerClient = nil
		case k8serrors.IsUnauthorized(err) || k8serrors.IsForbidden(err):
			if !deleting && src == nil {
				return reconcile.Result{}, h.fail(ctx, vmop, nil, vmopcondition.ReasonPeerRejected, refusedMessage(peer, err))
			}
			if released, releaseErr := h.releaseOnDeletionTimeout(ctx, vmop); released || releaseErr != nil {
				return reconcile.Result{}, releaseErr
			}
			return h.waitForPeer(vmop)
		case !isAPIError(err):
			h.progress(vmop, vmopcondition.ReasonWaitingForPeer, unreachableMessage(peer))
			return reconcile.Result{RequeueAfter: peerRequeue}, nil
		default:
			return reconcile.Result{}, err
		}
	}
	// A request of the same name for another migration is not this operation's to wait for or delete.
	foreign := receiver != nil && receiver.Labels[labelPeerVMOPUID] != peering.ReceiverVMOPUID(vmop)
	if foreign {
		receiver = nil
	}
	if receiver != nil && !switched && !deleting {
		if err = h.refreshSyncPins(ctx, peerClient, vmop, src, receiver); err != nil {
			return reconcile.Result{}, err
		}
	}

	if switched {
		return h.finish(ctx, peerClient, vmop, src, receiver)
	}

	if deleting {
		done, err := h.rollback(ctx, peerClient, vmop, src)
		if err != nil || !done {
			if released, releaseErr := h.releaseOnDeletionTimeout(ctx, vmop); released || releaseErr != nil {
				return reconcile.Result{}, releaseErr
			}
			return reconcile.Result{RequeueAfter: peerRequeue}, err
		}
		controllerutil.RemoveFinalizer(vmop, v1alpha2.FinalizerVMOPCleanup)
		vmop.Status.Phase = v1alpha2.VMOPPhaseTerminating
		return reconcile.Result{}, nil
	}

	if src != nil {
		return h.watchMigration(ctx, peerClient, vmop, src, receiver)
	}

	switch {
	case foreign:
		return reconcile.Result{}, h.fail(ctx, vmop, peerClient, vmopcondition.ReasonPeerRejected,
			fmt.Sprintf("Cluster %q already receives a virtual machine named %q in namespace %q.", peer, vmop.Spec.VirtualMachine, vmop.Namespace))
	case receiver == nil:
		return h.request(ctx, peerClient, vmop)
	case receiver.Status.Phase == intvirtv1alpha1.ReceiverFailed:
		return reconcile.Result{}, h.fail(ctx, vmop, peerClient, vmopcondition.ReasonPeerRejected,
			fmt.Sprintf("Cluster %q rejected the virtual machine: %s", peer, receiver.Status.Message))
	case receiver.Status.Phase == intvirtv1alpha1.ReceiverReady:
		return h.start(ctx, peerClient, vmop, receiver)
	default:
		if !receiver.CreationTimestamp.IsZero() && time.Since(receiver.CreationTimestamp.Time) > peerPrepareTimeout {
			msg := fmt.Sprintf("Cluster %q did not prepare the virtual machine in %s", peer, peerPrepareTimeout)
			if receiver.Status.Message != "" {
				msg += ": " + receiver.Status.Message
			}
			return reconcile.Result{}, h.fail(ctx, vmop, peerClient, vmopcondition.ReasonPeerRejected, msg+".")
		}
		h.progress(vmop, vmopcondition.ReasonWaitingForPeer, fmt.Sprintf("Waiting for cluster %q to prepare the virtual machine.", peer))
		return reconcile.Result{RequeueAfter: peerRequeue}, nil
	}
}

// request asks the peer to receive the VM: the InternalVirtualizationVirtualMachineReceiver is the only object created there,
// the peer creates the VM, its disks and its address from it.
func (h PeerMigrationHandler) request(ctx context.Context, peer client.Client, vmop *v1alpha2.VirtualMachineOperation) (reconcile.Result, error) {
	peerName := commonvmop.PeerName(vmop)
	key := types.NamespacedName{Namespace: vmop.Namespace, Name: vmop.Spec.VirtualMachine}
	vm := &v1alpha2.VirtualMachine{}
	if err := h.client.Get(ctx, key, vm); err != nil {
		// The migration was started and the VM is gone: the move deleted it and its status was not saved.
		if sent, _ := conditions.GetCondition(vmopcondition.TypeSignalSent, vmop.Status.Conditions); k8serrors.IsNotFound(err) && sent.Status == metav1.ConditionTrue {
			msg := fmt.Sprintf("The virtual machine has moved to cluster %q.", peerName)
			h.complete(vmop, v1alpha2.VMOPPhaseCompleted, vmopcondition.ReasonOperationCompleted, msg)
			controllerutil.RemoveFinalizer(vmop, v1alpha2.FinalizerVMOPCleanup)
			return reconcile.Result{}, nil
		}
		if k8serrors.IsNotFound(err) {
			return reconcile.Result{}, h.failRequest(ctx, vmop,
				fmt.Sprintf("The virtual machine cannot move to cluster %q: %s", peerName, err))
		}
		return reconcile.Result{}, err
	}
	kvvm := &virtv1.VirtualMachine{}
	kvvmi := &virtv1.VirtualMachineInstance{}
	for _, obj := range []client.Object{kvvm, kvvmi} {
		if err := h.client.Get(ctx, key, obj); err != nil {
			if k8serrors.IsNotFound(err) {
				return reconcile.Result{}, h.failRequest(ctx, vmop,
					fmt.Sprintf("The virtual machine cannot move to cluster %q: it is not running.", peerName))
			}
			return reconcile.Result{}, err
		}
	}

	// The move runs with the spec the VM runs with, not with changes that wait for a restart: the same
	// spec decides which disks travel and which are deleted here afterwards.
	spec, err := runningSpec(vm, kvvm)
	if err != nil {
		return reconcile.Result{}, h.failRequest(ctx, vmop,
			fmt.Sprintf("The virtual machine cannot move to cluster %q: %s", peerName, err))
	}
	mid := peering.ReceiverVMOPUID(vmop)
	vds, err := h.virtualDisks(ctx, vm, spec)
	if err != nil {
		if k8serrors.IsNotFound(err) {
			return reconcile.Result{}, h.failRequest(ctx, vmop,
				fmt.Sprintf("The virtual machine cannot move to cluster %q: %s", peerName, err))
		}
		return reconcile.Result{}, err
	}
	// The pair may have been undone after the operation was admitted.
	if err = h.peers.Paired(ctx, peerName, vmop.Namespace); err != nil {
		if errors.Is(err, peering.ErrPeerNotPaired) {
			return reconcile.Result{}, h.failRequest(ctx, vmop, err.Error())
		}
		return reconcile.Result{}, err
	}
	target, err := h.peers.Target(ctx, peerName, vmop.Namespace)
	if err != nil {
		return reconcile.Result{}, err
	}
	receiver, err := makeReceiver(vm, spec, kvvmi, vds, mid, target)
	if err != nil {
		return reconcile.Result{}, h.failRequest(ctx, vmop,
			fmt.Sprintf("The virtual machine cannot move to cluster %q: %s", peerName, err))
	}

	// The VM is locked only when the request is ready to be created: a disk that is not ready yet or a
	// spec that cannot be read must not leave the VM locked behind a retry loop.
	if err = h.markMoved(ctx, vm, kvvm, vmop); err != nil {
		return reconcile.Result{}, err
	}

	err = peer.Create(ctx, receiver)
	switch {
	case err == nil:
	case k8serrors.IsAlreadyExists(err):
		existing, err := h.getReceiver(ctx, peer, vmop)
		if err != nil {
			return reconcile.Result{}, err
		}
		if existing != nil && existing.Labels[labelPeerVMOPUID] != mid {
			return reconcile.Result{}, h.fail(ctx, vmop, peer, vmopcondition.ReasonPeerRejected,
				fmt.Sprintf("Cluster %q already receives a virtual machine named %q in namespace %q.", commonvmop.PeerName(vmop), vm.Name, target.Namespace))
		}
	case k8serrors.IsUnauthorized(err) || k8serrors.IsForbidden(err):
		return reconcile.Result{}, h.fail(ctx, vmop, nil, vmopcondition.ReasonPeerRejected, refusedMessage(peerName, err))
	case k8serrors.IsInvalid(err):
		return reconcile.Result{}, h.fail(ctx, vmop, peer, vmopcondition.ReasonPeerRejected,
			fmt.Sprintf("Cluster %q rejected the virtual machine: %s", commonvmop.PeerName(vmop), err))
	default:
		return reconcile.Result{}, err
	}

	h.progress(vmop, vmopcondition.ReasonWaitingForPeer, fmt.Sprintf("Waiting for cluster %q to prepare the virtual machine.", commonvmop.PeerName(vmop)))
	return reconcile.Result{RequeueAfter: peerRequeue}, nil
}

// start creates the migration to the peer once its receiver is ready and has published the address of its
// synchronization controller and the pin of the key it generated for this migration: the controller here
// trusts the peer by the pin, and the peer trusts the pin of this cluster from the request.
func (h PeerMigrationHandler) start(ctx context.Context, peer client.Client, vmop *v1alpha2.VirtualMachineOperation, receiver *intvirtv1alpha1.InternalVirtualizationVirtualMachineReceiver) (reconcile.Result, error) {
	if receiver.Status.SynchronizationAddress == "" || receiver.Status.SyncPin == "" {
		// A peer whose KubeVirt knows no keys per migration never publishes the pin: a bounded wait
		// tells the user instead of an endless one.
		if !receiver.CreationTimestamp.IsZero() && time.Since(receiver.CreationTimestamp.Time) > peerPrepareTimeout+peerPinTimeout {
			return reconcile.Result{}, h.fail(ctx, vmop, peer, vmopcondition.ReasonPeerRejected,
				fmt.Sprintf("Cluster %q did not get ready for the migration in %s. Check that the virtualization module there is of the same version as in this cluster.", commonvmop.PeerName(vmop), peerPrepareTimeout+peerPinTimeout))
		}
		h.progress(vmop, vmopcondition.ReasonWaitingForPeer, fmt.Sprintf("Waiting for cluster %q to publish the address and the key of its synchronization controller.", commonvmop.PeerName(vmop)))
		return reconcile.Result{RequeueAfter: peerRequeue}, nil
	}

	vmim := &virtv1.VirtualMachineInstanceMigration{
		ObjectMeta: metav1.ObjectMeta{
			Namespace:   vmop.Namespace,
			Name:        service.MigrationName(vmop),
			Annotations: map[string]string{annotations.AnnMigrationPeerSyncPin: receiver.Status.SyncPin},
			OwnerReferences: []metav1.OwnerReference{
				*metav1.NewControllerRef(vmop, v1alpha2.SchemeGroupVersion.WithKind(v1alpha2.VirtualMachineOperationKind)),
			},
		},
		Spec: virtv1.VirtualMachineInstanceMigrationSpec{
			VMIName: vmop.Spec.VirtualMachine,
			SendTo: &virtv1.VirtualMachineInstanceMigrationSource{
				MigrationID: string(receiver.UID),
				ConnectURL:  receiver.Status.SynchronizationAddress,
			},
		},
	}
	if err := h.client.Create(ctx, vmim); client.IgnoreAlreadyExists(err) != nil {
		return reconcile.Result{}, fmt.Errorf("create the migration to the peer: %w", err)
	}
	conditions.SetCondition(conditions.NewConditionBuilder(vmopcondition.TypeSignalSent).
		Generation(vmop.GetGeneration()).Reason(vmopcondition.ReasonSignalSentSuccess).Status(metav1.ConditionTrue), &vmop.Status.Conditions)
	h.progress(vmop, vmopcondition.ReasonMigrationPending, "The migration to the peer cluster is starting.")
	return reconcile.Result{RequeueAfter: peerRequeue}, nil
}

// watchMigration follows the migration to the peer until it succeeds or fails. What the peer reports about
// its receiving pod is shown to the user: a pod that cannot start there looks here like a migration that
// does not begin.
func (h PeerMigrationHandler) watchMigration(ctx context.Context, peer client.Client, vmop *v1alpha2.VirtualMachineOperation, src *virtv1.VirtualMachineInstanceMigration, receiver *intvirtv1alpha1.InternalVirtualizationVirtualMachineReceiver) (reconcile.Result, error) {
	peerName := commonvmop.PeerName(vmop)
	if src.Status.Phase == virtv1.MigrationFailed {
		msg := "The migration to the peer cluster failed."
		if src.Status.MigrationState != nil && src.Status.MigrationState.FailureReason != "" {
			msg = fmt.Sprintf("The migration to the peer cluster failed: %s", src.Status.MigrationState.FailureReason)
		}
		return reconcile.Result{}, h.fail(ctx, vmop, peer, vmopcondition.ReasonOperationFailed, msg)
	}
	peerMessage := ""
	if receiver != nil {
		peerMessage = receiver.Status.Message
	}
	started := src.Status.MigrationState != nil && src.Status.MigrationState.StartTimestamp != nil
	if !started && time.Since(src.CreationTimestamp.Time) > peerSyncTimeout {
		if peerMessage != "" {
			return reconcile.Result{}, h.fail(ctx, vmop, peer, vmopcondition.ReasonPeerRejected,
				fmt.Sprintf("The migration did not start in %s: in cluster %q %s.", peerSyncTimeout, peerName, peerMessage))
		}
		return reconcile.Result{}, h.fail(ctx, vmop, peer, vmopcondition.ReasonMigrationNetworkUnreachable,
			fmt.Sprintf("The migration did not start in %s: the nodes of this cluster may not reach cluster %q.", peerSyncTimeout, peerName))
	}
	switch {
	case started:
		h.progress(vmop, vmopcondition.ReasonMigrationRunning, "The virtual machine is being migrated to the peer cluster.")
	case peerMessage != "":
		h.progress(vmop, vmopcondition.ReasonMigrationPending, fmt.Sprintf("The migration to the peer cluster is waiting: in cluster %q %s.", peerName, peerMessage))
	default:
		h.progress(vmop, vmopcondition.ReasonMigrationPending, "The migration to the peer cluster is synchronizing with the peer.")
	}
	return reconcile.Result{RequeueAfter: peerRequeue}, nil
}

// finish waits for the peer to confirm that the VM runs there and deletes the VM here. Until then the VM
// stays stopped here: the guest runs in the peer.
func (h PeerMigrationHandler) finish(ctx context.Context, peer client.Client, vmop *v1alpha2.VirtualMachineOperation, src *virtv1.VirtualMachineInstanceMigration, receiver *intvirtv1alpha1.InternalVirtualizationVirtualMachineReceiver) (reconcile.Result, error) {
	if receiver != nil && receiver.Status.Phase == intvirtv1alpha1.ReceiverRunning {
		return reconcile.Result{}, h.moved(ctx, peer, vmop, receiver)
	}
	// The peer says its VM has not run: there is nothing to wait for, the guest is gone with the switchover.
	failedInPeer := receiver != nil && receiver.Status.Phase == intvirtv1alpha1.ReceiverFailed && !receiver.Status.ReceiverStarted
	switched := src.CreationTimestamp.Time
	if src.Status.MigrationState != nil && src.Status.MigrationState.EndTimestamp != nil {
		switched = src.Status.MigrationState.EndTimestamp.Time
	}
	if !failedInPeer && time.Since(switched) <= peerConfirmTimeout {
		h.progress(vmop, vmopcondition.ReasonWaitingForPeer, fmt.Sprintf("Waiting for cluster %q to confirm that the virtual machine runs there.", commonvmop.PeerName(vmop)))
		return reconcile.Result{RequeueAfter: peerRequeue}, nil
	}
	// The guest arrived, the receiving migration succeeded there; only the regular handling of the VM in
	// the peer lags behind. The move is complete.
	if receiver != nil && receiver.Status.ReceiverStarted {
		return reconcile.Result{}, h.moved(ctx, peer, vmop, receiver)
	}
	msg := fmt.Sprintf("Cluster %q did not confirm in %s that the virtual machine runs there; the virtual machine is kept stopped in this cluster. Check cluster %[1]q: if the virtual machine runs there, delete it in this cluster; if not, start it here.", commonvmop.PeerName(vmop), peerConfirmTimeout)
	if failedInPeer {
		why := receiver.Status.Message
		if why == "" {
			why = "the virtual machine did not start there"
		}
		msg = fmt.Sprintf("Cluster %q did not start the virtual machine after the switchover: %s. The virtual machine is kept stopped in this cluster; start it here.", commonvmop.PeerName(vmop), why)
		if err := peer.Delete(ctx, receiver); client.IgnoreNotFound(err) != nil {
			return reconcile.Result{}, err
		}
	}
	if err := h.handBack(ctx, vmop); err != nil {
		return reconcile.Result{}, err
	}
	h.complete(vmop, v1alpha2.VMOPPhaseFailed, vmopcondition.ReasonPeerNotConfirmed, msg)
	h.recorder.Event(vmop, corev1.EventTypeWarning, v1alpha2.ReasonErrVMOPFailed, msg)
	controllerutil.RemoveFinalizer(vmop, v1alpha2.FinalizerVMOPCleanup)
	return reconcile.Result{}, nil
}

// waitForPeer preserves the lock and finalizer until the receiver state is known.
func (h PeerMigrationHandler) waitForPeer(vmop *v1alpha2.VirtualMachineOperation) (reconcile.Result, error) {
	h.progress(vmop, vmopcondition.ReasonWaitingForPeer,
		fmt.Sprintf("Waiting for cluster %q to report the state of the virtual machine.", commonvmop.PeerName(vmop)))
	return reconcile.Result{RequeueAfter: peerRequeue}, nil
}

// releaseOnDeletionTimeout stops an operation that has been deleting for too long and whose peer cannot
// be asked. The VM must not stay locked by a request the source cannot remove; the peer removes the
// request itself by its TTL.
func (h PeerMigrationHandler) releaseOnDeletionTimeout(ctx context.Context, vmop *v1alpha2.VirtualMachineOperation) (bool, error) {
	if vmop.DeletionTimestamp.IsZero() || time.Since(vmop.DeletionTimestamp.Time) <= peerDeletionTimeout {
		return false, nil
	}
	// A migration that has not failed may still switch the guest over: wait for it to end.
	src, err := h.migration.GetMigration(ctx, vmop)
	if err != nil || (src != nil && src.Status.Phase != virtv1.MigrationFailed) {
		return false, err
	}
	peerName := commonvmop.PeerName(vmop)
	if peerClient, err := h.peers.Client(ctx, peerName); err == nil {
		if receiver, err := h.ownReceiver(ctx, peerClient, vmop); err == nil && receiver != nil {
			_ = peerClient.Delete(ctx, receiver)
		}
	}
	stayed, err := h.release(ctx, vmop)
	if err != nil {
		return false, err
	}
	msg := fmt.Sprintf("The request to receive in cluster %q was not removed in %s; the peer removes it later. The virtual machine is released here.", peerName, peerDeletionTimeout)
	if !stayed {
		msg = fmt.Sprintf("The request to receive in cluster %q was not removed in %s; the peer removes it later. The virtual machine is not running here and may run in cluster %q: it is left stopped with the Manual run policy, check it there before starting it here.", peerName, peerDeletionTimeout, peerName)
	}
	h.recorder.Event(vmop, corev1.EventTypeWarning, v1alpha2.ReasonErrVMOPFailed, msg)
	controllerutil.RemoveFinalizer(vmop, v1alpha2.FinalizerVMOPCleanup)
	vmop.Status.Phase = v1alpha2.VMOPPhaseTerminating
	return true, nil
}

// release hands the VM back after a move that did not happen. Only a VM still running here proves the guest
// stayed; any other is handed back stopped, as the guest may run in the peer. It reports whether the VM stayed.
func (h PeerMigrationHandler) release(ctx context.Context, vmop *v1alpha2.VirtualMachineOperation) (bool, error) {
	var kvvmi virtv1.VirtualMachineInstance
	err := h.client.Get(ctx, types.NamespacedName{Namespace: vmop.Namespace, Name: vmop.Spec.VirtualMachine}, &kvvmi)
	if client.IgnoreNotFound(err) != nil {
		return false, err
	}
	if err == nil && kvvmi.Status.Phase == virtv1.Running {
		return true, h.unmarkMoved(ctx, vmop)
	}
	return false, h.handBack(ctx, vmop)
}

func (h PeerMigrationHandler) isMarkedMoved(ctx context.Context, vmop *v1alpha2.VirtualMachineOperation) (bool, error) {
	var vm v1alpha2.VirtualMachine
	err := h.client.Get(ctx, types.NamespacedName{Namespace: vmop.Namespace, Name: vmop.Spec.VirtualMachine}, &vm)
	if err != nil {
		return false, err
	}
	return vm.Annotations[annotations.AnnVMMovedToPeer] == vmop.Name, nil
}

// handBack leaves the VM here stopped but startable by hand: its run policy becomes Manual, so that it
// does not start by itself while the guest may run in the peer, and the lock of the move goes.
func (h PeerMigrationHandler) handBack(ctx context.Context, vmop *v1alpha2.VirtualMachineOperation) error {
	var vm v1alpha2.VirtualMachine
	err := h.client.Get(ctx, types.NamespacedName{Namespace: vmop.Namespace, Name: vmop.Spec.VirtualMachine}, &vm)
	if k8serrors.IsNotFound(err) {
		return nil
	}
	if err != nil {
		return err
	}
	if vm.GetAnnotations()[annotations.AnnVMMovedToPeer] == vmop.Name {
		patch, _ := json.Marshal(map[string]any{
			"metadata": map[string]any{"annotations": map[string]any{annotations.AnnVMMovedToPeer: nil}},
			"spec":     map[string]any{"runPolicy": v1alpha2.ManualPolicy},
		})
		if err = h.client.Patch(ctx, &vm, client.RawPatch(types.MergePatchType, patch)); err != nil {
			return fmt.Errorf("hand the virtual machine back: %w", err)
		}
	}
	var kvvm virtv1.VirtualMachine
	if err = h.client.Get(ctx, types.NamespacedName{Namespace: vm.Namespace, Name: vm.Name}, &kvvm); err != nil {
		return client.IgnoreNotFound(err)
	}
	patch, _ := json.Marshal(map[string]any{
		"metadata": map[string]any{"annotations": map[string]any{annRunStrategyBeforeMove: nil}},
		"spec":     map[string]any{"runStrategy": virtv1.RunStrategyManual},
	})
	return h.client.Patch(ctx, &kvvm, client.RawPatch(types.MergePatchType, patch))
}

// moved ends the operation as a move: the guest runs in the peer, so the VM here and the request there go.
func (h PeerMigrationHandler) moved(ctx context.Context, peer client.Client, vmop *v1alpha2.VirtualMachineOperation, receiver *intvirtv1alpha1.InternalVirtualizationVirtualMachineReceiver) error {
	if err := h.deleteSource(ctx, vmop); err != nil {
		return err
	}
	// The request goes only once the peer has lifted the receive mode: deleted earlier, it makes the peer
	// cancel the receive and remove the KubeVirt VM the guest runs in. Otherwise the peer removes it itself.
	if receiver.Status.Phase == intvirtv1alpha1.ReceiverRunning {
		if err := peer.Delete(ctx, receiver); client.IgnoreNotFound(err) != nil {
			return err
		}
	}
	msg := fmt.Sprintf("The virtual machine has moved to cluster %q.", commonvmop.PeerName(vmop))
	h.complete(vmop, v1alpha2.VMOPPhaseCompleted, vmopcondition.ReasonOperationCompleted, msg)
	h.recorder.Event(vmop, corev1.EventTypeNormal, v1alpha2.ReasonVMOPSucceeded, msg)
	controllerutil.RemoveFinalizer(vmop, v1alpha2.FinalizerVMOPCleanup)
	return nil
}

// deleteSource deletes the VM and its VirtualDisks here. The addresses the VM got automatically go with it;
// the ones the user created and the snapshots of the disks stay, and the snapshots are named in an event.
func (h PeerMigrationHandler) deleteSource(ctx context.Context, vmop *v1alpha2.VirtualMachineOperation) error {
	var vm v1alpha2.VirtualMachine
	err := h.client.Get(ctx, types.NamespacedName{Namespace: vmop.Namespace, Name: vmop.Spec.VirtualMachine}, &vm)
	if k8serrors.IsNotFound(err) {
		return nil
	}
	if err != nil {
		return err
	}
	// A VM without the lock of this operation is not the one that moved: it was created again under the name.
	if vm.GetAnnotations()[annotations.AnnVMMovedToPeer] != vmop.Name {
		return nil
	}
	spec := &vm.Spec
	var kvvm virtv1.VirtualMachine
	if err = h.client.Get(ctx, types.NamespacedName{Namespace: vm.Namespace, Name: vm.Name}, &kvvm); err == nil {
		spec, err = runningSpec(&vm, &kvvm)
		if err != nil {
			return err
		}
	} else if !k8serrors.IsNotFound(err) {
		return err
	}
	disks := map[string]bool{}
	for _, ref := range spec.BlockDeviceRefs {
		if ref.Kind != v1alpha2.DiskDevice {
			continue
		}
		disks[ref.Name] = true
		vd := &v1alpha2.VirtualDisk{ObjectMeta: metav1.ObjectMeta{Namespace: vm.Namespace, Name: ref.Name}}
		if err = h.client.Delete(ctx, vd); client.IgnoreNotFound(err) != nil {
			return err
		}
	}
	if vm.DeletionTimestamp == nil {
		if err = h.client.Delete(ctx, &vm); client.IgnoreNotFound(err) != nil {
			return err
		}
		h.reportKeptSnapshots(ctx, vmop, disks)
	}
	return nil
}

// reportKeptSnapshots tells the user which snapshots of the deleted disks remain in this project.
func (h PeerMigrationHandler) reportKeptSnapshots(ctx context.Context, vmop *v1alpha2.VirtualMachineOperation, disks map[string]bool) {
	var snapshots v1alpha2.VirtualDiskSnapshotList
	if err := h.client.List(ctx, &snapshots, client.InNamespace(vmop.Namespace)); err != nil {
		return
	}
	var kept []string
	for _, s := range snapshots.Items {
		if disks[s.Spec.VirtualDiskName] {
			kept = append(kept, fmt.Sprintf("%q", s.Name))
		}
	}
	if len(kept) == 0 {
		return
	}
	h.recorder.Event(vmop, corev1.EventTypeNormal, v1alpha2.ReasonVMOPSucceeded,
		fmt.Sprintf("The virtual machine runs in cluster %q and was deleted here. Its disk snapshots %s are kept in this project.", commonvmop.PeerName(vmop), strings.Join(kept, ", ")))
}

// rollback undoes an unfinished move: the migration here goes, the request in the peer goes (the peer
// removes what it created for it), and the VM runs here as before. It reports whether nothing is left.
// A request whose VM has run in the peer is not undone: the move is finished instead.
func (h PeerMigrationHandler) rollback(ctx context.Context, peer client.Client, vmop *v1alpha2.VirtualMachineOperation, src *virtv1.VirtualMachineInstanceMigration) (bool, error) {
	if src != nil {
		if err := h.client.Delete(ctx, src); client.IgnoreNotFound(err) != nil {
			return false, err
		}
		return false, nil
	}
	receiver, err := h.ownReceiver(ctx, peer, vmop)
	if err != nil {
		return false, err
	}
	if receiver != nil {
		received, err := h.receiverMigrationSucceeded(ctx, peer, receiver)
		if err != nil {
			return false, err
		}
		if receiver.Status.ReceiverStarted || received {
			return true, h.moved(ctx, peer, vmop, receiver)
		}
		// The receiver has not run yet: it goes, and the VM is released by its state here.
		if err = peer.Delete(ctx, receiver); client.IgnoreNotFound(err) != nil {
			return false, err
		}
	}
	_, err = h.release(ctx, vmop)
	return true, err
}

// receiverMigrationSucceeded tells whether the guest has arrived in the peer: the receiving migration
// succeeded there, even if the request has not been marked or has already gone.
func (h PeerMigrationHandler) receiverMigrationSucceeded(ctx context.Context, peer client.Client, receiver *intvirtv1alpha1.InternalVirtualizationVirtualMachineReceiver) (bool, error) {
	if peer == nil || receiver == nil {
		return false, nil
	}
	received := &unstructured.Unstructured{}
	received.SetGroupVersionKind(peerMigrationGVK)
	err := peer.Get(ctx, types.NamespacedName{Namespace: receiver.Namespace, Name: string(receiver.UID)}, received)
	if k8serrors.IsNotFound(err) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	phase, _, err := unstructured.NestedString(received.Object, "status", "phase")
	return phase == string(virtv1.MigrationSucceeded), err
}

// peerMigrationGVK is the KubeVirt migration as the API server of the peer serves it: the client to the peer
// goes past the kube-api-rewriter, so kubevirt.io is not there.
var peerMigrationGVK = schema.GroupVersionKind{Group: "internal.virtualization.deckhouse.io", Version: "v1", Kind: "InternalVirtualizationVirtualMachineInstanceMigration"}

// ownReceiver is the request of this operation in the peer, if there is a peer to ask: a request of the same
// name for another migration is not this operation's.
func (h PeerMigrationHandler) ownReceiver(ctx context.Context, peer client.Client, vmop *v1alpha2.VirtualMachineOperation) (*intvirtv1alpha1.InternalVirtualizationVirtualMachineReceiver, error) {
	if peer == nil {
		return nil, nil
	}
	receiver, err := h.getReceiver(ctx, peer, vmop)
	if err != nil || receiver == nil || receiver.Labels[labelPeerVMOPUID] != peering.ReceiverVMOPUID(vmop) {
		return nil, err
	}
	return receiver, nil
}

// cleanupFailed removes what a failed operation left: a reconcile with a stale copy of the operation may
// have locked the VM and asked the peer again after the failure was saved. The copy locks the VM before it
// asks the peer, so a VM without the lock of the operation leaves nothing to remove.
func (h PeerMigrationHandler) cleanupFailed(ctx context.Context, vmop *v1alpha2.VirtualMachineOperation) error {
	marked, err := h.isMarkedMoved(ctx, vmop)
	if !marked {
		return client.IgnoreNotFound(err)
	}
	peerClient, err := h.peers.Client(ctx, commonvmop.PeerName(vmop))
	if err != nil {
		h.recorder.Event(vmop, corev1.EventTypeWarning, v1alpha2.ReasonErrVMOPFailed,
			fmt.Sprintf("The request to receive in cluster %q was not removed, the peer removes it by its TTL: %s", commonvmop.PeerName(vmop), err))
	} else {
		receiver, err := h.ownReceiver(ctx, peerClient, vmop)
		if err != nil {
			return err
		}
		if receiver != nil && !receiver.Status.ReceiverStarted {
			received, err := h.receiverMigrationSucceeded(ctx, peerClient, receiver)
			if err != nil {
				return err
			}
			if !received {
				if err = peerClient.Delete(ctx, receiver); client.IgnoreNotFound(err) != nil {
					return err
				}
			}
		}
	}
	return h.unmarkMoved(ctx, vmop)
}

// failRequest ends an operation whose request was never created: there is nothing to undo in the peer,
// and the VM here has not been locked yet.
func (h PeerMigrationHandler) failRequest(ctx context.Context, vmop *v1alpha2.VirtualMachineOperation, msg string) error {
	if err := h.unmarkMoved(ctx, vmop); err != nil {
		return err
	}
	h.complete(vmop, v1alpha2.VMOPPhaseFailed, vmopcondition.ReasonPeerRejected, msg)
	h.recorder.Event(vmop, corev1.EventTypeWarning, v1alpha2.ReasonErrVMOPFailed, msg)
	if !vmop.DeletionTimestamp.IsZero() {
		controllerutil.RemoveFinalizer(vmop, v1alpha2.FinalizerVMOPCleanup)
	}
	return nil
}

// fail ends the operation and undoes what it did here and in the peer. The VM is handed back to this
// cluster only when the peer says its VM has not run there; otherwise the guest is there and the move is
// finished instead.
func (h PeerMigrationHandler) fail(ctx context.Context, vmop *v1alpha2.VirtualMachineOperation, peer client.Client, reason vmopcondition.ReasonCompleted, msg string) error {
	receiver, err := h.ownReceiver(ctx, peer, vmop)
	if err != nil {
		return err
	}
	if receiver != nil {
		received, err := h.receiverMigrationSucceeded(ctx, peer, receiver)
		if err != nil {
			return err
		}
		if receiver.Status.ReceiverStarted || received {
			return h.moved(ctx, peer, vmop, receiver)
		}
	}
	src, err := h.migration.GetMigration(ctx, vmop)
	if err != nil {
		return err
	}
	if src != nil {
		if err = h.client.Delete(ctx, src); client.IgnoreNotFound(err) != nil {
			return err
		}
	}
	if receiver != nil {
		if err = peer.Delete(ctx, receiver); client.IgnoreNotFound(err) != nil {
			return err
		}
	}
	if _, err = h.release(ctx, vmop); err != nil {
		return err
	}
	h.complete(vmop, v1alpha2.VMOPPhaseFailed, reason, msg)
	h.recorder.Event(vmop, corev1.EventTypeWarning, v1alpha2.ReasonErrVMOPFailed, msg)
	if !vmop.DeletionTimestamp.IsZero() {
		controllerutil.RemoveFinalizer(vmop, v1alpha2.FinalizerVMOPCleanup)
	}
	return nil
}

func refusedMessage(peer string, err error) string {
	return fmt.Sprintf("Cluster %q refused the credentials of this cluster: %s. Check the pairing of the clusters and the virtualization capability on both sides.", peer, err)
}

func unreachableMessage(peer string) string {
	return fmt.Sprintf("Cluster %q is unreachable. The migration continues when the connection is restored. The virtual machine keeps running in this cluster.", peer)
}

// isAPIError tells an answer of the API server of the peer from a failure to reach it.
func isAPIError(err error) bool {
	var status k8serrors.APIStatus
	return errors.As(err, &status)
}

func (h PeerMigrationHandler) progress(vmop *v1alpha2.VirtualMachineOperation, reason vmopcondition.ReasonCompleted, msg string) {
	vmop.Status.Phase = v1alpha2.VMOPPhaseInProgress
	conditions.SetCondition(conditions.NewConditionBuilder(vmopcondition.TypeCompleted).
		Generation(vmop.GetGeneration()).Reason(reason).Status(metav1.ConditionFalse).Message(msg), &vmop.Status.Conditions)
}

func (h PeerMigrationHandler) complete(vmop *v1alpha2.VirtualMachineOperation, phase v1alpha2.VMOPPhase, reason vmopcondition.ReasonCompleted, msg string) {
	vmop.Status.Phase = phase
	status := metav1.ConditionFalse
	if phase == v1alpha2.VMOPPhaseCompleted {
		status = metav1.ConditionTrue
	}
	conditions.SetCondition(conditions.NewConditionBuilder(vmopcondition.TypeCompleted).
		Generation(vmop.GetGeneration()).Reason(reason).Status(status).Message(msg), &vmop.Status.Conditions)
}

func (h PeerMigrationHandler) getReceiver(ctx context.Context, peer client.Client, vmop *v1alpha2.VirtualMachineOperation) (*intvirtv1alpha1.InternalVirtualizationVirtualMachineReceiver, error) {
	target, err := h.peers.Target(ctx, commonvmop.PeerName(vmop), vmop.Namespace)
	if err != nil {
		return nil, err
	}
	var r intvirtv1alpha1.InternalVirtualizationVirtualMachineReceiver
	err = peer.Get(ctx, types.NamespacedName{Namespace: target.Namespace, Name: vmop.Spec.VirtualMachine}, &r)
	if k8serrors.IsNotFound(err) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get the request to receive in the peer: %w", err)
	}
	return &r, nil
}

// runningSpec returns the spec the VM runs with: the last applied one, or the current one when the
// KubeVirt VM has not been built from it yet. The whole move uses this spec, so a change that waits for a
// restart can neither travel nor be deleted here by mistake.
func runningSpec(vm *v1alpha2.VirtualMachine, kvvm *virtv1.VirtualMachine) (*v1alpha2.VirtualMachineSpec, error) {
	if kvvm == nil {
		return &vm.Spec, nil
	}
	spec, err := kvbuilder.LoadLastAppliedSpec(kvvm)
	if err != nil {
		return nil, err
	}
	if spec == nil {
		spec = &vm.Spec
	}
	return spec, nil
}

// virtualDisks returns the VirtualDisks of the running spec in the order of its block devices.
func (h PeerMigrationHandler) virtualDisks(ctx context.Context, vm *v1alpha2.VirtualMachine, spec *v1alpha2.VirtualMachineSpec) ([]*v1alpha2.VirtualDisk, error) {
	var vds []*v1alpha2.VirtualDisk
	for _, ref := range spec.BlockDeviceRefs {
		if ref.Kind != v1alpha2.DiskDevice {
			continue
		}
		var vd v1alpha2.VirtualDisk
		if err := h.client.Get(ctx, types.NamespacedName{Namespace: vm.Namespace, Name: ref.Name}, &vd); err != nil {
			return nil, err
		}
		vds = append(vds, &vd)
	}
	return vds, nil
}

// markMoved stops the regular handling of the VM here for the time of the move: the VM controller neither
// restarts the guest after it leaves nor rebuilds its KubeVirt VM, which runs with the Manual strategy.
func (h PeerMigrationHandler) markMoved(ctx context.Context, vm *v1alpha2.VirtualMachine, kvvm *virtv1.VirtualMachine, vmop *v1alpha2.VirtualMachineOperation) error {
	if vm.GetAnnotations()[annotations.AnnVMMovedToPeer] != vmop.Name {
		patch, _ := json.Marshal(map[string]any{"metadata": map[string]any{"annotations": map[string]string{annotations.AnnVMMovedToPeer: vmop.Name}}})
		if err := h.client.Patch(ctx, vm, client.RawPatch(types.MergePatchType, patch)); err != nil {
			return fmt.Errorf("mark the virtual machine moved to the peer: %w", err)
		}
	}
	if kvvm.Spec.RunStrategy == nil || *kvvm.Spec.RunStrategy != virtv1.RunStrategyManual {
		before := ""
		if kvvm.Spec.RunStrategy != nil {
			before = string(*kvvm.Spec.RunStrategy)
		}
		patch, _ := json.Marshal(map[string]any{
			"metadata": map[string]any{"annotations": map[string]string{annRunStrategyBeforeMove: before}},
			"spec":     map[string]any{"runStrategy": virtv1.RunStrategyManual, "running": nil},
		})
		if err := h.client.Patch(ctx, kvvm, client.RawPatch(types.MergePatchType, patch)); err != nil {
			return fmt.Errorf("stop restarts of the internal virtual machine: %w", err)
		}
	}
	return nil
}

// unmarkMoved hands the VM back to the regular handling: the VM controller restores its KubeVirt VM.
func (h PeerMigrationHandler) unmarkMoved(ctx context.Context, vmop *v1alpha2.VirtualMachineOperation) error {
	var vm v1alpha2.VirtualMachine
	err := h.client.Get(ctx, types.NamespacedName{Namespace: vmop.Namespace, Name: vmop.Spec.VirtualMachine}, &vm)
	if k8serrors.IsNotFound(err) {
		return nil
	}
	if err != nil {
		return err
	}
	if vm.GetAnnotations()[annotations.AnnVMMovedToPeer] != vmop.Name {
		return nil
	}

	var kvvm virtv1.VirtualMachine
	if err = h.client.Get(ctx, types.NamespacedName{Namespace: vm.Namespace, Name: vm.Name}, &kvvm); client.IgnoreNotFound(err) != nil {
		return err
	}
	if before, ok := kvvm.GetAnnotations()[annRunStrategyBeforeMove]; ok && err == nil {
		spec := map[string]any{"runStrategy": nil}
		if before != "" {
			spec["runStrategy"] = before
		}
		patch, _ := json.Marshal(map[string]any{"metadata": map[string]any{"annotations": map[string]any{annRunStrategyBeforeMove: nil}}, "spec": spec})
		if err = h.client.Patch(ctx, &kvvm, client.RawPatch(types.MergePatchType, patch)); err != nil {
			return fmt.Errorf("restore the run strategy of the internal virtual machine: %w", err)
		}
	}

	patch, _ := json.Marshal(map[string]any{"metadata": map[string]any{"annotations": map[string]any{annotations.AnnVMMovedToPeer: nil}}})
	return h.client.Patch(ctx, &vm, client.RawPatch(types.MergePatchType, patch))
}

// refreshSyncPins hands across the pins the synchronization controllers of the clusters trust each other
// by: the one of this cluster, from the state of the migration here, into the annotation of the request;
// the one of the peer, from the status of the request, into the migration here. A controller generates the
// key anew after a restart, so the pins are read again and again.
func (h PeerMigrationHandler) refreshSyncPins(ctx context.Context, peer client.Client, vmop *v1alpha2.VirtualMachineOperation, src *virtv1.VirtualMachineInstanceMigration, receiver *intvirtv1alpha1.InternalVirtualizationVirtualMachineReceiver) error {
	pin, err := peering.MigrationSyncPin(ctx, h.client, types.NamespacedName{Namespace: vmop.Namespace, Name: vmop.Spec.VirtualMachine}, "sourceState")
	if err != nil {
		return err
	}
	if pin != "" && receiver.Annotations[annotations.AnnReceiverSourceSyncPin] != pin {
		patch, _ := json.Marshal(map[string]any{"metadata": map[string]any{"annotations": map[string]string{annotations.AnnReceiverSourceSyncPin: pin}}})
		if err = peer.Patch(ctx, receiver, client.RawPatch(types.MergePatchType, patch)); err != nil {
			return fmt.Errorf("update the pin of this cluster in the request: %w", err)
		}
	}
	if src == nil {
		return nil
	}
	return peering.SetMigrationPeerSyncPin(ctx, h.client, client.ObjectKeyFromObject(src), receiver.Status.SyncPin)
}

// makeReceiver is the request to receive: the VM with the spec it runs with, its VirtualDisks with the
// StorageClass and the size they have here, the KubeVirt VMI with the running domain and the address.
func makeReceiver(vm *v1alpha2.VirtualMachine, spec *v1alpha2.VirtualMachineSpec, kvvmi *virtv1.VirtualMachineInstance, vds []*v1alpha2.VirtualDisk, mid string, target peering.Target) (*intvirtv1alpha1.InternalVirtualizationVirtualMachineReceiver, error) {
	rawVM, err := json.Marshal(map[string]any{
		"metadata": map[string]any{"name": vm.Name, "labels": userMetadata(vm.Labels), "annotations": userMetadata(vm.Annotations)},
		"spec":     spec,
	})
	if err != nil {
		return nil, err
	}

	var rawVDs []runtime.RawExtension
	for _, vd := range vds {
		size, err := resource.ParseQuantity(vd.Status.Capacity)
		if err != nil {
			return nil, fmt.Errorf("the size of the disk %q is not known yet: %w", vd.Name, err)
		}
		pvc := map[string]any{"size": size.String()}
		if vd.Status.StorageClassName != "" {
			pvc["storageClassName"] = vd.Status.StorageClassName
		}
		raw, err := json.Marshal(map[string]any{
			"metadata": map[string]any{"name": vd.Name, "labels": userMetadata(vd.Labels), "annotations": userMetadata(vd.Annotations)},
			"spec":     map[string]any{"persistentVolumeClaim": pvc},
		})
		if err != nil {
			return nil, err
		}
		rawVDs = append(rawVDs, runtime.RawExtension{Raw: raw})
	}

	rawVMI, err := json.Marshal(map[string]any{"spec": kvvmi.Spec, "status": kvvmi.Status})
	if err != nil {
		return nil, err
	}

	anns := map[string]string{}
	if target.NewAddress {
		anns[annotations.AnnReceiverNewIPAddress] = "true"
	}
	return &intvirtv1alpha1.InternalVirtualizationVirtualMachineReceiver{
		TypeMeta:   metav1.TypeMeta{APIVersion: intvirtv1alpha1.SchemeGroupVersion.String(), Kind: intvirtv1alpha1.InternalVirtualizationVirtualMachineReceiverKind},
		ObjectMeta: metav1.ObjectMeta{Namespace: target.Namespace, Name: vm.Name, Labels: map[string]string{labelPeerVMOPUID: mid}, Annotations: anns},
		Spec: intvirtv1alpha1.VirtualMachineReceiverSpec{
			VirtualMachineTemplate:                       runtime.RawExtension{Raw: rawVM},
			VirtualDiskTemplates:                         rawVDs,
			InternalVirtualizationVirtualMachineInstance: runtime.RawExtension{Raw: rawVMI},
			IPAddress: vm.Status.IPAddress,
		},
	}, nil
}

// userMetadata keeps the labels or annotations of the user: the ones of the platform describe the object in
// this cluster and are set again by the peer.
func userMetadata(m map[string]string) map[string]string {
	out := maps.Clone(m)
	for k := range out {
		if strings.Contains(k, "deckhouse.io/") || strings.HasPrefix(k, "kubectl.kubernetes.io/") {
			delete(out, k)
		}
	}
	return out
}
