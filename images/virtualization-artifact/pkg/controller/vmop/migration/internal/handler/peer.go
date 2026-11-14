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
	"strings"
	"time"

	corev1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
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
	commonvm "github.com/deckhouse/virtualization-controller/pkg/common/vm"
	commonvmop "github.com/deckhouse/virtualization-controller/pkg/common/vmop"
	"github.com/deckhouse/virtualization-controller/pkg/controller/conditions"
	"github.com/deckhouse/virtualization-controller/pkg/controller/peering"
	"github.com/deckhouse/virtualization-controller/pkg/controller/reconciler"
	migrationprogress "github.com/deckhouse/virtualization-controller/pkg/controller/vmop/migration/internal/progress"
	"github.com/deckhouse/virtualization-controller/pkg/controller/vmop/migration/internal/service"
	"github.com/deckhouse/virtualization-controller/pkg/eventrecord"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/vmcondition"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/vmopcondition"
	intvirtv1alpha1 "github.com/deckhouse/virtualization/api/internalvirtualization/v1alpha1"
)

const (
	peerHandlerName = "PeerMigrationHandler"

	// labelPeerVMOPUID marks the request the source created in the peer for a migration: only it is
	// removed on a rollback, a request with the same name that is not marked belongs to another migration.
	labelPeerVMOPUID = peering.LabelReceiverVMOPUID

	peerRequeue = 5 * time.Second
	// movedConditionRequeue paces the wait for the VM controller to show the move in the condition of the VM.
	movedConditionRequeue = time.Second
	// peerSourcePreparationTimeout bounds that wait: the VM controller shows the move within a second, and a restart of
	// it takes under a minute. A VM that never shows it would keep the operation waiting and the VM locked.
	peerSourcePreparationTimeout = 5 * time.Minute
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
	// Wait for receiver cleanup before releasing the source VM to avoid running both copies.
	// Bound the wait so an unresponsive peer cannot keep the VM locked indefinitely.
	peerRequestTimeout = 10 * time.Minute
	// Wait for the peer's failure cause: local KubeVirt may report only a closed socket.
	// Allow time for the peer to publish the cause and this controller to observe it.
	peerCauseTimeout = 30 * time.Second
)

// PeerMigrationHandler moves a VM to a peer cluster: a Migrate operation with
// spec.migrate.projectPeerName. It asks the peer to receive the VM with a InternalVirtualizationVirtualMachineReceiver in the
// namespace of the same name, starts the migration to the peer once it is ready and deletes the VM here
// when the peer confirms that the VM runs there. Every step is derived from what exists in both clusters,
// so a reconcile can stop anywhere.
type PeerMigrationHandler struct {
	client client.Client
	// reader decides whether the guest stayed here after a move that did not happen: the cache may still show
	// running an instance that has failed with the handover, and the VM would start here by its old run
	// strategy while the guest runs in the peer.
	reader    client.Reader
	peers     PeerClients
	migration *service.MigrationService
	recorder  eventrecord.EventRecorderLogger
	// progressStrategy smooths the transfer counters of the outgoing migration, as the local migration does.
	progressStrategy migrationprogress.Strategy
}

// PeerClients gives clients to peer clusters (peering.PeerClients).
type PeerClients interface {
	Client(ctx context.Context, peer string) (client.Client, error)
	Target(ctx context.Context, peer, namespace string) (peering.Target, error)
	Paired(ctx context.Context, peer, namespace string) error
}

func NewPeerMigrationHandler(client client.Client, reader client.Reader, peers PeerClients, migration *service.MigrationService, recorder eventrecord.EventRecorderLogger) *PeerMigrationHandler {
	return &PeerMigrationHandler{client: client, reader: reader, peers: peers, migration: migration, recorder: recorder, progressStrategy: migrationprogress.NewProgress()}
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

	if err := h.labelWithVM(ctx, vmop); err != nil {
		return reconcile.Result{}, err
	}

	res, err := h.handle(ctx, vmop)
	if err != nil {
		if released, releaseErr := h.releaseOnDeletionTimeout(ctx, vmop); releaseErr != nil {
			return res, releaseErr
		} else if released {
			return res, reconciler.ErrStopHandlerChain
		}

		if !commonvmop.IsFinished(vmop) {
			h.progress(vmop, vmopcondition.ReasonOperationFailed, fmt.Sprintf("The migration handler failed: %s", err))
		}

		h.recorder.Event(vmop, corev1.EventTypeWarning, v1alpha2.ReasonErrVMOPFailed, err.Error())

		return res, err
	}
	// Nothing in the peer wakes the operation here: a request it removes or marks failed is seen only on the
	// next reconcile, so a move that is not over keeps polling instead of waiting for good.
	if res.IsZero() && !commonvmop.IsFinished(vmop) && vmop.Status.Phase != v1alpha2.VMOPPhaseTerminating {
		res.RequeueAfter = peerRequeue
	}

	return res, reconciler.ErrStopHandlerChain
}

// labelWithVM sets the VM UID label normally set by the skipped lifecycle handler.
// Preserve existing labels and do not associate the operation with a recreated VM.
func (h PeerMigrationHandler) labelWithVM(ctx context.Context, vmop *v1alpha2.VirtualMachineOperation) error {
	if _, ok := vmop.GetLabels()[annotations.LabelVirtualMachineUID]; ok {
		return nil
	}

	var vm v1alpha2.VirtualMachine

	err := h.client.Get(ctx, types.NamespacedName{Namespace: vmop.Namespace, Name: vmop.Spec.VirtualMachine}, &vm)
	if err != nil {
		return client.IgnoreNotFound(err)
	}

	if vm.CreationTimestamp.After(vmop.CreationTimestamp.Time) {
		return nil
	}
	// Equal timestamps have only second precision and cannot establish ownership of a historical operation.
	if commonvmop.IsFinished(vmop) && vm.CreationTimestamp.Equal(&vmop.CreationTimestamp) {
		return nil
	}

	annotations.AddLabel(vmop, annotations.LabelVirtualMachineUID, string(vm.UID))

	return nil
}

func (h PeerMigrationHandler) handle(ctx context.Context, vmop *v1alpha2.VirtualMachineOperation) (reconcile.Result, error) {
	deleting := !vmop.DeletionTimestamp.IsZero()
	if commonvmop.IsFinished(vmop) {
		if vmop.Status.Phase == v1alpha2.VMOPPhaseFailed {
			cleaned, err := h.cleanupFailed(ctx, vmop)
			if err != nil {
				return reconcile.Result{}, err
			}

			if !cleaned {
				return reconcile.Result{RequeueAfter: peerRequeue}, nil
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
		// Revoked access cannot recover through a retry. Cancel the source migration before releasing its VM.
		if !deleting {
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
			if !deleting {
				return reconcile.Result{}, h.fail(ctx, vmop, nil, vmopcondition.ReasonPeerRejected, refusedMessage(peer, err))
			}

			if released, releaseErr := h.releaseOnDeletionTimeout(ctx, vmop); released || releaseErr != nil {
				return reconcile.Result{}, releaseErr
			}

			return h.waitForPeer(vmop)
		case !isAPIError(err):
			// A cancel must end even when the network to the peer stays down, as when the peer refuses.
			if released, releaseErr := h.releaseOnDeletionTimeout(ctx, vmop); released || releaseErr != nil {
				return reconcile.Result{}, releaseErr
			}

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

	// A VM that goes away takes the move with it: nothing starts for it any more, and the operation ends as
	// what it was instead of failing on the instance that left with the VM.
	if ended, err := h.endIfVMDeleting(ctx, vmop, peerClient, receiver); err != nil || ended {
		return reconcile.Result{}, err
	}

	// The migration here is started only for a ready request, and the peer removes a request only once its
	// receiving migration is over without the guest: with the request gone, nothing can arrive there.
	if src != nil && receiver == nil {
		return reconcile.Result{}, h.failWithoutReceiver(ctx, vmop, peerClient, src)
	}

	if src != nil {
		return h.watchMigration(ctx, peerClient, vmop, src, receiver)
	}

	switch {
	case foreign:
		return reconcile.Result{}, h.fail(ctx, vmop, peerClient, vmopcondition.ReasonPeerRejected,
			receiverConflictMessage(peer, vmop.Spec.VirtualMachine, vmop.Namespace))
	case receiver == nil:
		// A migration to the peer aborts a snapshot in progress as a local one does. The wait comes before the
		// request, so the peer timeouts do not run while the operation waits.
		wait, err := waitForSnapshots(ctx, h.reader, vmop)
		if err != nil || wait {
			return reconcile.Result{}, err
		}

		return h.createReceiver(ctx, peerClient, vmop)
	case !receiver.DeletionTimestamp.IsZero():
		// The request goes, so the move has been undone: this is a copy of the operation older than its
		// failure, and a migration started now would be left without a receiver, locking the VM.
		if time.Since(receiver.DeletionTimestamp.Time) > peerRequestTimeout {
			return reconcile.Result{}, h.fail(ctx, vmop, peerClient, vmopcondition.ReasonPeerNotConfirmed,
				fmt.Sprintf("Peer cluster %q did not remove the request to receive the virtual machine in %s.", peer, peerRequestTimeout))
		}

		return reconcile.Result{RequeueAfter: peerRequeue}, nil
	case receiver.Status.Phase == intvirtv1alpha1.ReceiverFailed:
		return reconcile.Result{}, h.fail(ctx, vmop, peerClient, vmopcondition.ReasonPeerRejected,
			fmt.Sprintf("Peer cluster %q rejected the virtual machine: %s", peer, receiver.Status.Message))
	case receiver.Status.Phase == intvirtv1alpha1.ReceiverReady:
		return h.start(ctx, peerClient, vmop, receiver)
	default:
		if !receiver.CreationTimestamp.IsZero() && time.Since(receiver.CreationTimestamp.Time) > peerPrepareTimeout {
			msg := fmt.Sprintf("Peer cluster %q did not prepare the virtual machine in %s", peer, peerPrepareTimeout)
			if receiver.Status.Message != "" {
				msg += ": " + receiver.Status.Message
			}

			return reconcile.Result{}, h.fail(ctx, vmop, peerClient, vmopcondition.ReasonPeerRejected, msg+".")
		}

		h.progress(vmop, vmopcondition.ReasonWaitingForPeer, fmt.Sprintf("Waiting for peer cluster %q to prepare the virtual machine.", peer))

		return reconcile.Result{RequeueAfter: peerRequeue}, nil
	}
}

// endIfVMDeleting checks the peer and saved operation status when the source VM is deleting.
// It reports whether the operation has ended.
func (h PeerMigrationHandler) endIfVMDeleting(ctx context.Context, vmop *v1alpha2.VirtualMachineOperation, peer client.Client, receiver *intvirtv1alpha1.InternalVirtualizationVirtualMachineReceiver) (bool, error) {
	peerName := commonvmop.PeerName(vmop)

	var vm v1alpha2.VirtualMachine

	err := h.client.Get(ctx, types.NamespacedName{Namespace: vmop.Namespace, Name: vmop.Spec.VirtualMachine}, &vm)
	if k8serrors.IsNotFound(err) {
		return false, nil
	}

	if err != nil {
		return false, err
	}

	if vm.DeletionTimestamp.IsZero() {
		return false, nil
	}

	if marked, err := h.hasPeerMigrationVMOP(ctx, vmop); err != nil || !marked {
		return false, err
	}

	arrived := receiver != nil && receiver.Status.ReceiverStarted
	if !arrived && receiver != nil {
		if arrived, err = h.receiverMigrationSucceeded(ctx, peer, receiver); err != nil {
			return false, err
		}
	}

	if arrived && receiver != nil {
		return true, h.moved(ctx, peer, vmop, receiver)
	}
	// Without a request to ask, the migration sent is the last proof of the move.
	sent, _ := conditions.GetCondition(vmopcondition.TypeSignalSent, vmop.Status.Conditions)
	if arrived || (receiver == nil && sent.Status == metav1.ConditionTrue) {
		msg := fmt.Sprintf("The virtual machine has moved to peer cluster %q.", peerName)
		h.complete(vmop, v1alpha2.VMOPPhaseCompleted, vmopcondition.ReasonOperationCompleted, msg)
		h.recorder.Event(vmop, corev1.EventTypeNormal, v1alpha2.ReasonVMOPSucceeded, msg)
		controllerutil.RemoveFinalizer(vmop, v1alpha2.FinalizerVMOPCleanup)

		return true, nil
	}

	return true, h.failBeforeReceiverCreation(ctx, vmop,
		fmt.Sprintf("The virtual machine cannot move to peer cluster %q: it is being deleted in this cluster.", peerName))
}

// createReceiver asks the peer to receive the VM: the InternalVirtualizationVirtualMachineReceiver is the only object created there,
// the peer creates the VM, its disks and its address from it.
func (h PeerMigrationHandler) createReceiver(ctx context.Context, peer client.Client, vmop *v1alpha2.VirtualMachineOperation) (reconcile.Result, error) {
	peerName := commonvmop.PeerName(vmop)
	key := types.NamespacedName{Namespace: vmop.Namespace, Name: vmop.Spec.VirtualMachine}

	vm := &v1alpha2.VirtualMachine{}
	if err := h.reader.Get(ctx, key, vm); err != nil {
		// The migration was started and the VM is gone: the move deleted it and its status was not saved.
		if sent, _ := conditions.GetCondition(vmopcondition.TypeSignalSent, vmop.Status.Conditions); k8serrors.IsNotFound(err) && sent.Status == metav1.ConditionTrue {
			msg := fmt.Sprintf("The virtual machine has moved to peer cluster %q.", peerName)
			h.complete(vmop, v1alpha2.VMOPPhaseCompleted, vmopcondition.ReasonOperationCompleted, msg)
			controllerutil.RemoveFinalizer(vmop, v1alpha2.FinalizerVMOPCleanup)

			return reconcile.Result{}, nil
		}

		if k8serrors.IsNotFound(err) {
			return reconcile.Result{}, h.failBeforeReceiverCreation(ctx, vmop,
				fmt.Sprintf("The virtual machine cannot move to peer cluster %q: %s", peerName, err))
		}

		return reconcile.Result{}, err
	}

	kvvm := &virtv1.VirtualMachine{}

	kvvmi := &virtv1.VirtualMachineInstance{}
	for _, obj := range []client.Object{kvvm, kvvmi} {
		if err := h.client.Get(ctx, key, obj); err != nil {
			if k8serrors.IsNotFound(err) {
				return reconcile.Result{}, h.failBeforeReceiverCreation(ctx, vmop,
					fmt.Sprintf("The virtual machine cannot move to peer cluster %q: it is not running.", peerName))
			}

			return reconcile.Result{}, err
		}
	}

	// The move runs with the spec the VM runs with, not with changes that wait for a restart: the same
	// spec decides which disks travel and which are deleted here afterwards.
	spec, err := runningSpec(vm, kvvm)
	if err != nil {
		return reconcile.Result{}, h.failBeforeReceiverCreation(ctx, vmop,
			fmt.Sprintf("The virtual machine cannot move to peer cluster %q: %s", peerName, err))
	}

	mid := peering.ReceiverVMOPUID(vmop)

	vds, err := h.virtualDisks(ctx, vm, spec)
	if err != nil {
		if k8serrors.IsNotFound(err) {
			return reconcile.Result{}, h.failBeforeReceiverCreation(ctx, vmop,
				fmt.Sprintf("The virtual machine cannot move to peer cluster %q: %s", peerName, err))
		}

		return reconcile.Result{}, err
	}
	// The pair may have been undone after the operation was admitted.
	if err = h.peers.Paired(ctx, peerName, vmop.Namespace); err != nil {
		if errors.Is(err, peering.ErrPeerNotPaired) {
			return reconcile.Result{}, h.failBeforeReceiverCreation(ctx, vmop, err.Error())
		}

		return reconcile.Result{}, err
	}

	target, err := h.peers.Target(ctx, peerName, vmop.Namespace)
	if err != nil {
		return reconcile.Result{}, err
	}

	receiver, err := makeReceiver(vm, spec, kvvmi, vds, mid, target)
	if err != nil {
		return reconcile.Result{}, h.failBeforeReceiverCreation(ctx, vmop,
			fmt.Sprintf("The virtual machine cannot move to peer cluster %q: %s", peerName, err))
	}

	// The VM is locked only when the request is ready to be created: a disk that is not ready yet or a
	// spec that cannot be read must not leave the VM locked behind a retry loop.
	if err = h.setPeerMigrationVMOP(ctx, kvvm, vmop); err != nil {
		return reconcile.Result{}, err
	}
	// The peer is asked only once the VM here shows the move: from then on it cannot be changed, and the
	// request carries the spec it runs with.
	if !meta.IsStatusConditionTrue(vm.Status.Conditions, vmcondition.TypeMovedToPeer.String()) {
		// The peer has not been asked yet: the guest is here, and giving up only lifts the lock.
		if !vmop.CreationTimestamp.IsZero() && time.Since(vmop.CreationTimestamp.Time) > peerSourcePreparationTimeout {
			if err = h.clearPeerMigrationVMOP(ctx, vmop); err != nil {
				return reconcile.Result{}, err
			}

			msg := fmt.Sprintf("The migration to peer cluster %q did not start: the VM controller did not confirm source preparation within %s of operation creation.", peerName, peerSourcePreparationTimeout) + keepsRunningSuffix
			h.complete(vmop, v1alpha2.VMOPPhaseFailed, vmopcondition.ReasonOperationFailed, msg)
			h.recorder.Event(vmop, corev1.EventTypeWarning, v1alpha2.ReasonErrVMOPFailed, msg)

			return reconcile.Result{}, nil
		}

		h.progress(vmop, vmopcondition.ReasonWaitingForPeer, "Waiting for the VM controller to confirm source preparation.")

		return reconcile.Result{RequeueAfter: movedConditionRequeue}, nil
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
				receiverConflictMessage(commonvmop.PeerName(vmop), vm.Name, target.Namespace))
		}
	case k8serrors.IsUnauthorized(err) || k8serrors.IsForbidden(err):
		return reconcile.Result{}, h.fail(ctx, vmop, nil, vmopcondition.ReasonPeerRejected, refusedMessage(peerName, err))
	case k8serrors.IsInvalid(err):
		return reconcile.Result{}, h.fail(ctx, vmop, peer, vmopcondition.ReasonPeerRejected,
			fmt.Sprintf("Peer cluster %q rejected the virtual machine: %s", commonvmop.PeerName(vmop), err))
	default:
		return reconcile.Result{}, err
	}

	h.progress(vmop, vmopcondition.ReasonWaitingForPeer, fmt.Sprintf("Waiting for peer cluster %q to prepare the virtual machine.", commonvmop.PeerName(vmop)))

	return reconcile.Result{RequeueAfter: peerRequeue}, nil
}

// start creates the migration to the peer once its receiver is ready and has published the address of its
// synchronization controller and the pin of the key it generated for this migration: the controller here
// trusts the peer by the pin, and the peer trusts the pin of this cluster from the request.
func (h PeerMigrationHandler) start(ctx context.Context, peer client.Client, vmop *v1alpha2.VirtualMachineOperation, receiver *intvirtv1alpha1.InternalVirtualizationVirtualMachineReceiver) (reconcile.Result, error) {
	vm := &v1alpha2.VirtualMachine{}
	if err := h.reader.Get(ctx, types.NamespacedName{Namespace: vmop.Namespace, Name: vmop.Spec.VirtualMachine}, vm); err != nil {
		return reconcile.Result{}, err
	}

	if receiver.Status.Synchronization.SyncAddress == "" || receiver.Status.Synchronization.SyncPin == "" {
		// A peer whose KubeVirt knows no keys per migration never publishes the pin: a bounded wait
		// tells the user instead of an endless one.
		if !receiver.CreationTimestamp.IsZero() && time.Since(receiver.CreationTimestamp.Time) > peerPrepareTimeout+peerPinTimeout {
			return reconcile.Result{}, h.fail(ctx, vmop, peer, vmopcondition.ReasonPeerRejected,
				fmt.Sprintf("Peer cluster %q did not get ready for the migration in %s. Check that the virtualization module there is of the same version as in this cluster.", commonvmop.PeerName(vmop), peerPrepareTimeout+peerPinTimeout))
		}

		h.progress(vmop, vmopcondition.ReasonWaitingForPeer, fmt.Sprintf("Waiting for peer cluster %q to publish the address and the key of its synchronization controller.", commonvmop.PeerName(vmop)))

		return reconcile.Result{RequeueAfter: peerRequeue}, nil
	}

	vmim := &virtv1.VirtualMachineInstanceMigration{
		ObjectMeta: metav1.ObjectMeta{
			Namespace:   vmop.Namespace,
			Name:        service.MigrationName(vmop),
			Annotations: peering.MigrationSynchronizationAnnotations(receiver.Status.Synchronization),
			OwnerReferences: []metav1.OwnerReference{
				*metav1.NewControllerRef(vmop, v1alpha2.SchemeGroupVersion.WithKind(v1alpha2.VirtualMachineOperationKind)),
			},
		},
		Spec: virtv1.VirtualMachineInstanceMigrationSpec{
			VMIName: vmop.Spec.VirtualMachine,
			SendTo: &virtv1.VirtualMachineInstanceMigrationSource{
				MigrationID: string(receiver.UID),
				ConnectURL:  receiver.Status.Synchronization.SyncAddress,
			},
		},
	}
	if err := h.client.Create(ctx, vmim); client.IgnoreAlreadyExists(err) != nil {
		// KubeVirt refuses a migration it cannot run (a guest without an invariant TSC, another migration of
		// the VM in progress) with 400 or 422: a retry changes nothing, and the operation would stay in
		// progress with the VM locked for good.
		if k8serrors.IsBadRequest(err) || k8serrors.IsInvalid(err) || k8serrors.IsForbidden(err) {
			return reconcile.Result{}, h.fail(ctx, vmop, peer, vmopcondition.ReasonOperationFailed,
				fmt.Sprintf("The migration to the peer cluster was refused: %s.", strings.TrimSuffix(apiErrorMessage(err), ".")))
		}

		return reconcile.Result{}, fmt.Errorf("create the migration to the peer: %w", err)
	}

	conditions.SetCondition(conditions.NewConditionBuilder(vmopcondition.TypeSignalSent).
		Generation(vmop.GetGeneration()).Reason(vmopcondition.ReasonSignalSentSuccess).Status(metav1.ConditionTrue), &vmop.Status.Conditions)
	h.setProgress(vmop, progressWaitingForSyncSlot)
	h.progress(vmop, vmopcondition.ReasonMigrationPending, "The migration to the peer cluster is starting.")

	return reconcile.Result{RequeueAfter: peerRequeue}, nil
}

// peerFailureMessage tells why the migration to the peer failed. KubeVirt here knows only that the receiving
// side went away (libvirt reports a closed socket), so the cause the peer found, in the message of its
// request, replaces it.
func peerFailureMessage(src *virtv1.VirtualMachineInstanceMigration, peerName, peerMessage string) string {
	if peerMessage != "" {
		return fmt.Sprintf("The migration to the peer cluster failed: in peer cluster %q %s.", peerName, strings.TrimSuffix(peerMessage, "."))
	}

	if src.Status.MigrationState != nil && src.Status.MigrationState.FailureReason != "" {
		return fmt.Sprintf("The migration to the peer cluster failed: %s.", src.Status.MigrationState.FailureReason)
	}

	return "The migration to the peer cluster failed."
}

// waitsForPeerCause tells whether the failure of the migration here waits for the peer to report its cause
// (see peerCauseTimeout). There is nothing to wait for when the peer has already reported it, when the
// guest has arrived there (the move is finished instead) or when the request is gone.
func waitsForPeerCause(src *virtv1.VirtualMachineInstanceMigration, receiver *intvirtv1alpha1.InternalVirtualizationVirtualMachineReceiver) bool {
	if receiver == nil || receiver.Status.Phase == intvirtv1alpha1.ReceiverFailed || receiver.Status.ReceiverStarted {
		return false
	}

	failed := migrationFailedAt(src)

	return failed != nil && time.Since(failed.Time) < peerCauseTimeout
}

// migrationFailedAt returns when the migration failed: the time of the transition to Failed, or the end of
// the migration state. Without either the failure is not waited on.
func migrationFailedAt(src *virtv1.VirtualMachineInstanceMigration) *metav1.Time {
	for _, t := range src.Status.PhaseTransitionTimestamps {
		if t.Phase == virtv1.MigrationFailed {
			return &t.PhaseTransitionTimestamp
		}
	}

	if src.Status.MigrationState != nil {
		return src.Status.MigrationState.EndTimestamp
	}

	return nil
}

// watchMigration follows the migration to the peer until it succeeds or fails. What the peer reports about
// its receiving pod is shown to the user: a pod that cannot start there looks here like a migration that
// does not begin.
func (h PeerMigrationHandler) watchMigration(ctx context.Context, peer client.Client, vmop *v1alpha2.VirtualMachineOperation, src *virtv1.VirtualMachineInstanceMigration, receiver *intvirtv1alpha1.InternalVirtualizationVirtualMachineReceiver) (reconcile.Result, error) {
	peerName := commonvmop.PeerName(vmop)
	// ReceiverStarted confirms arrival even if the source migration never learns it
	// because synchronization breaks after switchover.
	if receiver != nil && receiver.Status.ReceiverStarted {
		return reconcile.Result{}, h.moved(ctx, peer, vmop, receiver)
	}

	peerMessage := ""
	if receiver != nil {
		peerMessage = receiver.Status.Message
	}

	if src.Status.Phase == virtv1.MigrationFailed {
		if waitsForPeerCause(src, receiver) {
			h.progress(vmop, vmopcondition.ReasonWaitingForPeer, fmt.Sprintf("The migration to the peer cluster failed, waiting for peer cluster %q to report why.", peerName))
			return reconcile.Result{RequeueAfter: peerRequeue}, nil
		}

		return reconcile.Result{}, h.fail(ctx, vmop, peer, vmopcondition.ReasonOperationFailed, peerFailureMessage(src, peerName, peerMessage))
	}

	started := src.Status.MigrationState != nil && src.Status.MigrationState.StartTimestamp != nil
	// Fail promptly if the peer stops receiving; fail() rechecks whether the guest arrived meanwhile.
	if receiver != nil && receiver.Status.Phase == intvirtv1alpha1.ReceiverFailed {
		return reconcile.Result{}, h.fail(ctx, vmop, peer, vmopcondition.ReasonPeerRejected, peerFailureMessage(src, peerName, peerMessage))
	}

	if !started && time.Since(src.CreationTimestamp.Time) > peerSyncTimeout {
		if peerMessage != "" {
			return reconcile.Result{}, h.fail(ctx, vmop, peer, vmopcondition.ReasonPeerRejected,
				fmt.Sprintf("The migration did not start in %s: in peer cluster %q %s.", peerSyncTimeout, peerName, peerMessage))
		}

		return reconcile.Result{}, h.fail(ctx, vmop, peer, vmopcondition.ReasonMigrationNetworkUnreachable,
			fmt.Sprintf("The migration did not start in %s: the nodes of this cluster may not reach peer cluster %q.", peerSyncTimeout, peerName))
	}

	switch {
	case started:
		h.syncProgress(vmop, src)
		h.progress(vmop, vmopcondition.ReasonMigrationRunning, "The virtual machine is being migrated to the peer cluster.")
	case peerMessage != "":
		// The request keeps the whole chain of problems for a failure; the wait is on the latest.
		h.progress(vmop, vmopcondition.ReasonMigrationPending, fmt.Sprintf("The migration to the peer cluster is waiting: in peer cluster %q %s.", peerName, peering.LatestReceiverProblem(peerMessage)))
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
		if !commonvmop.IsFinished(vmop) {
			h.progress(vmop, vmopcondition.ReasonWaitingForPeer, fmt.Sprintf("Waiting for peer cluster %q to confirm that the virtual machine runs there.", commonvmop.PeerName(vmop)))
		}

		return reconcile.Result{RequeueAfter: peerRequeue}, nil
	}
	// The guest arrived, the receiving migration succeeded there; only the regular handling of the VM in
	// the peer lags behind. The move is complete.
	if receiver != nil && receiver.Status.ReceiverStarted {
		return reconcile.Result{}, h.moved(ctx, peer, vmop, receiver)
	}

	msg := fmt.Sprintf("Peer cluster %q did not confirm in %s that the virtual machine runs there; the virtual machine is kept stopped in this cluster. Check peer cluster %[1]q: if the virtual machine runs there, delete it in this cluster; if not, start it here.", commonvmop.PeerName(vmop), peerConfirmTimeout)

	if failedInPeer {
		why := receiver.Status.Message
		if why == "" {
			why = "the virtual machine did not start there"
		}

		msg = fmt.Sprintf("Peer cluster %q did not start the virtual machine after the switchover: %s. The virtual machine is kept stopped in this cluster; start it here.", commonvmop.PeerName(vmop), why)

		if err := peer.Delete(ctx, receiver); client.IgnoreNotFound(err) != nil {
			return reconcile.Result{}, err
		}
	}

	if err := h.releaseVMWithManualPolicy(ctx, vmop); err != nil {
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
		fmt.Sprintf("Waiting for peer cluster %q to report the state of the virtual machine.", commonvmop.PeerName(vmop)))

	return reconcile.Result{RequeueAfter: peerRequeue}, nil
}

// releaseOnDeletionTimeout stops an operation that has been deleting for too long and whose peer cannot
// be asked. The VM must not stay locked by a request the source cannot remove; the peer removes the
// request itself by its TTL.
func (h PeerMigrationHandler) releaseOnDeletionTimeout(ctx context.Context, vmop *v1alpha2.VirtualMachineOperation) (bool, error) {
	if vmop.DeletionTimestamp.IsZero() {
		return false, nil
	}

	src, err := h.migration.GetMigration(ctx, vmop)
	if err != nil {
		return false, err
	}

	if src != nil {
		current, err := h.abortSourceMigration(ctx, src)
		if err != nil || (current != nil && current.Status.Phase != virtv1.MigrationFailed) {
			return false, err
		}
	}

	if time.Since(vmop.DeletionTimestamp.Time) <= peerDeletionTimeout {
		return false, nil
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

	msg := fmt.Sprintf("The request to receive in peer cluster %q was not removed in %s; the peer removes it later. The virtual machine is released here.", peerName, peerDeletionTimeout)
	if !stayed {
		msg = fmt.Sprintf("The request to receive in peer cluster %q was not removed in %s; the peer removes it later.", peerName, peerDeletionTimeout) + stoppedSuffix(peerName)
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

	err := h.reader.Get(ctx, types.NamespacedName{Namespace: vmop.Namespace, Name: vmop.Spec.VirtualMachine}, &kvvmi)
	if client.IgnoreNotFound(err) != nil {
		return false, err
	}

	if err == nil && kvvmi.Status.Phase == virtv1.Running {
		return true, h.clearPeerMigrationVMOP(ctx, vmop)
	}

	return false, h.releaseVMWithManualPolicy(ctx, vmop)
}

func (h PeerMigrationHandler) hasPeerMigrationVMOP(ctx context.Context, vmop *v1alpha2.VirtualMachineOperation) (bool, error) {
	var kvvm virtv1.VirtualMachine

	err := h.client.Get(ctx, types.NamespacedName{Namespace: vmop.Namespace, Name: vmop.Spec.VirtualMachine}, &kvvm)
	if k8serrors.IsNotFound(err) {
		return false, nil
	}

	if err != nil {
		return false, err
	}

	return commonvm.PeerMigrationVMOP(&kvvm) == vmop.Name, nil
}

// releaseVMWithManualPolicy leaves the VM here stopped but startable by hand: its run policy becomes Manual, so that it
// does not start by itself while the guest may run in the peer, and the lock of the move goes.
func (h PeerMigrationHandler) releaseVMWithManualPolicy(ctx context.Context, vmop *v1alpha2.VirtualMachineOperation) error {
	var vm v1alpha2.VirtualMachine

	err := h.client.Get(ctx, types.NamespacedName{Namespace: vmop.Namespace, Name: vmop.Spec.VirtualMachine}, &vm)
	if k8serrors.IsNotFound(err) {
		return nil
	}

	if err != nil {
		return err
	}

	var kvvm virtv1.VirtualMachine
	if err = h.client.Get(ctx, types.NamespacedName{Namespace: vm.Namespace, Name: vm.Name}, &kvvm); err != nil {
		return client.IgnoreNotFound(err)
	}

	if commonvm.PeerMigrationVMOP(&kvvm) != vmop.Name {
		return nil
	}
	// The run policy changes before the mark goes: without the mark, the VM controller would start the VM by
	// its old policy while the guest may run in the peer.
	if vm.Spec.RunPolicy != v1alpha2.ManualPolicy {
		patch, _ := json.Marshal(map[string]any{"spec": map[string]any{"runPolicy": v1alpha2.ManualPolicy}})
		if err = h.client.Patch(ctx, &vm, client.RawPatch(types.MergePatchType, patch)); err != nil {
			return fmt.Errorf("hand the virtual machine back: %w", err)
		}
	}

	patch, _ := json.Marshal(map[string]any{
		"metadata": map[string]any{"annotations": map[string]any{annotations.AnnPeerMigrationVMOP: nil}},
	})

	return h.client.Patch(ctx, &kvvm, client.RawPatch(types.MergePatchType, patch))
}

// moved ends the operation as a move: the guest runs in the peer, so the VM here and the request there go.
func (h PeerMigrationHandler) moved(ctx context.Context, peer client.Client, vmop *v1alpha2.VirtualMachineOperation, receiver *intvirtv1alpha1.InternalVirtualizationVirtualMachineReceiver) error {
	if err := h.deleteSource(ctx, vmop); err != nil {
		return err
	}
	// Remove the request on arrival: it keeps the VM name busy in the peer.
	// ReceiverStarted means the guest arrived even before the peer lifts the receive mode.
	if receiver != nil && (receiver.Status.ReceiverStarted || receiver.Status.Phase == intvirtv1alpha1.ReceiverRunning) {
		if err := peer.Delete(ctx, receiver); client.IgnoreNotFound(err) != nil {
			return err
		}
	}

	msg := fmt.Sprintf("The virtual machine has moved to peer cluster %q.", commonvmop.PeerName(vmop))
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
	if marked, err := h.hasPeerMigrationVMOP(ctx, vmop); err != nil || !marked {
		return err
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
		fmt.Sprintf("The virtual machine runs in peer cluster %q and was deleted here. Its disk snapshots %s are kept in this project.", commonvmop.PeerName(vmop), strings.Join(kept, ", ")))
}

// rollback undoes an unfinished move: the migration here goes, the request in the peer goes (the peer
// removes what it created for it), and the VM runs here as before. It reports whether nothing is left.
// A request whose VM has run in the peer is not undone: the move is finished instead.
func (h PeerMigrationHandler) rollback(ctx context.Context, peer client.Client, vmop *v1alpha2.VirtualMachineOperation, src *virtv1.VirtualMachineInstanceMigration) (bool, error) {
	receiver, err := h.ownReceiver(ctx, peer, vmop)
	if err != nil {
		return false, err
	}
	// The guest may have arrived while the migration here still runs (it may never learn the end when the
	// synchronization breaks): there is nothing to undo then, the move is finished.
	if receiver != nil {
		received, err := h.receiverMigrationSucceeded(ctx, peer, receiver)
		if err != nil {
			return false, err
		}

		if receiver.Status.ReceiverStarted || received {
			return true, h.moved(ctx, peer, vmop, receiver)
		}
	}

	if src != nil {
		if err := h.client.Delete(ctx, src); client.IgnoreNotFound(err) != nil {
			return false, err
		}

		return false, nil
	}

	if receiver != nil {
		// Wait for receiver cleanup before releasing the source to prevent both copies from running.
		// releaseOnDeletionTimeout handles an unresponsive peer.
		if receiver.DeletionTimestamp.IsZero() {
			if err = peer.Delete(ctx, receiver); client.IgnoreNotFound(err) != nil {
				return false, err
			}
		}

		return false, nil
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

// abortSourceMigration requests cancellation and reads its outcome past the cache before releasing the VM.
func (h PeerMigrationHandler) abortSourceMigration(ctx context.Context, src *virtv1.VirtualMachineInstanceMigration) (*virtv1.VirtualMachineInstanceMigration, error) {
	if src == nil {
		return nil, nil
	}

	if src.Status.Phase != virtv1.MigrationSucceeded && src.DeletionTimestamp.IsZero() {
		if err := h.client.Delete(ctx, src); client.IgnoreNotFound(err) != nil {
			return nil, err
		}
	}

	current := &virtv1.VirtualMachineInstanceMigration{}
	if err := h.reader.Get(ctx, client.ObjectKeyFromObject(src), current); err != nil {
		return nil, client.IgnoreNotFound(err)
	}

	return current, nil
}

// cleanupFailed undoes work recreated by a stale reconcile after failure.
// It reports completion and keeps the VM locked until cancellation and peer cleanup finish.
func (h PeerMigrationHandler) cleanupFailed(ctx context.Context, vmop *v1alpha2.VirtualMachineOperation) (bool, error) {
	src, err := h.migration.GetMigration(ctx, vmop)
	if err != nil {
		return false, err
	}

	src, err = h.abortSourceMigration(ctx, src)
	if err != nil {
		return false, err
	}

	marked, err := h.hasPeerMigrationVMOP(ctx, vmop)
	if err != nil || !marked {
		return !marked, client.IgnoreNotFound(err)
	}

	peerClient, err := h.peers.Client(ctx, commonvmop.PeerName(vmop))
	if err != nil {
		// Keep polling even though the operation is Failed: an unknown peer outcome
		// cannot safely restore the source's automatic restart strategy.
		if src != nil && src.Status.Phase == virtv1.MigrationSucceeded {
			res, finishErr := h.finish(ctx, nil, vmop, src, nil)
			return res.IsZero() && finishErr == nil, finishErr
		}

		return false, err
	}

	receiver, err := h.ownReceiver(ctx, peerClient, vmop)
	if err != nil {
		if src != nil && src.Status.Phase == virtv1.MigrationSucceeded {
			res, finishErr := h.finish(ctx, nil, vmop, src, nil)
			return res.IsZero() && finishErr == nil, finishErr
		}

		return false, err
	}

	if receiver != nil {
		received, err := h.receiverMigrationSucceeded(ctx, peerClient, receiver)
		if err != nil {
			return false, err
		}

		if receiver.Status.ReceiverStarted || received {
			return true, h.moved(ctx, peerClient, vmop, receiver)
		}
	}

	if src != nil && src.Status.Phase == virtv1.MigrationSucceeded {
		res, err := h.finish(ctx, peerClient, vmop, src, receiver)
		return res.IsZero() && err == nil, err
	}

	gone, timedOut, err := h.removeReceiver(ctx, peerClient, receiver)
	if err != nil || (!gone && !timedOut) {
		return false, err
	}
	// Deleting a running migration only requests cancellation. Do not release its
	// VM until KubeVirt has finished it, even if the peer request is already gone.
	current, err := h.abortSourceMigration(ctx, src)
	if err == nil && current != nil && current.Status.Phase == virtv1.MigrationSucceeded {
		res, err := h.finish(ctx, peerClient, vmop, current, receiver)
		return res.IsZero() && err == nil, err
	}

	if err != nil || (current != nil && !current.IsFinal()) {
		return false, err
	}

	if timedOut {
		h.recorder.Event(vmop, corev1.EventTypeWarning, v1alpha2.ReasonErrVMOPFailed,
			fmt.Sprintf("The request to receive in peer cluster %q was not removed in %s. The virtual machine is released here: check peer cluster %q before starting it.", commonvmop.PeerName(vmop), peerRequestTimeout, commonvmop.PeerName(vmop)))
	}

	_, err = h.release(ctx, vmop)

	return err == nil, err
}

// failBeforeReceiverCreation ends an operation whose request was never created: there is nothing to undo in the peer,
// and the VM here has not been locked yet.
func (h PeerMigrationHandler) failBeforeReceiverCreation(ctx context.Context, vmop *v1alpha2.VirtualMachineOperation, msg string) error {
	if err := h.clearPeerMigrationVMOP(ctx, vmop); err != nil {
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
// cluster only when the peer says its VM has not run there and has finished with the request; otherwise the
// guest is there and the move is finished instead.
func (h PeerMigrationHandler) fail(ctx context.Context, vmop *v1alpha2.VirtualMachineOperation, peer client.Client, reason vmopcondition.ReasonCompleted, msg string) error {
	peerName := commonvmop.PeerName(vmop)

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

	gone, timedOut, err := h.removeReceiver(ctx, peer, receiver)
	if err != nil {
		return err
	}
	// The peer may still finish its receiving migration while the guest has not arrived here. The migration
	// here stays then, and with it the lock of the VM: the next reconcile comes back to this failure instead
	// of starting a new move, the way the rollback waits for the same answer.
	if !gone && !timedOut && src != nil {
		h.progress(vmop, vmopcondition.ReasonWaitingForPeer, msg+awaitingRemovalSuffix(peerName))
		return nil
	}

	if receiver != nil && (gone || timedOut) {
		// The request is done with, but the receiving migration may have succeeded before the peer removed it:
		// the guest is in the peer then, and the move is finished instead of undone.
		received, err := h.receiverMigrationSucceeded(ctx, peer, receiver)
		if err != nil {
			return err
		}

		if received {
			return h.moved(ctx, peer, vmop, receiver)
		}
	}

	if src != nil {
		current, err := h.abortSourceMigration(ctx, src)
		if err != nil {
			return err
		}

		if current != nil && current.Status.Phase == virtv1.MigrationSucceeded {
			_, err = h.finish(ctx, peer, vmop, current, receiver)
			return err
		}

		if current != nil && !current.IsFinal() {
			h.progress(vmop, reason, msg)
			return nil
		}
	}

	if timedOut {
		h.recorder.Event(vmop, corev1.EventTypeWarning, v1alpha2.ReasonErrVMOPFailed,
			fmt.Sprintf("The request to receive in peer cluster %q was not removed in %s. The virtual machine is released here: check peer cluster %q before starting it.", peerName, peerRequestTimeout, peerName))
	}

	stayed, err := h.release(ctx, vmop)
	if err != nil {
		return err
	}

	if stayed {
		msg += keepsRunningSuffix
	} else {
		msg += stoppedSuffix(peerName)
	}

	h.complete(vmop, v1alpha2.VMOPPhaseFailed, reason, msg)
	h.recorder.Event(vmop, corev1.EventTypeWarning, v1alpha2.ReasonErrVMOPFailed, msg)

	if !vmop.DeletionTimestamp.IsZero() {
		controllerutil.RemoveFinalizer(vmop, v1alpha2.FinalizerVMOPCleanup)
	}

	return nil
}

const keepsRunningSuffix = " The virtual machine keeps running in this cluster; retry the move."

func stoppedSuffix(peer string) string {
	return fmt.Sprintf(" The virtual machine is not running here and may run in peer cluster %q: it is left stopped with the Manual run policy, check it there before starting it here.", peer)
}

func awaitingRemovalSuffix(peer string) string {
	return fmt.Sprintf(" The virtual machine is released once peer cluster %q removes the request to receive it.", peer)
}

// failWithoutReceiver ends a move whose request the peer has removed before the guest arrived. The failure
// that removed it waited for the removal with its cause in the condition, and the cause is all that is left
// of it.
func (h PeerMigrationHandler) failWithoutReceiver(ctx context.Context, vmop *v1alpha2.VirtualMachineOperation, peer client.Client, src *virtv1.VirtualMachineInstanceMigration) error {
	peerName := commonvmop.PeerName(vmop)
	reason := vmopcondition.ReasonPeerRejected

	msg := fmt.Sprintf("Peer cluster %q removed the request to receive the virtual machine.", peerName)
	if src.Status.Phase == virtv1.MigrationFailed {
		reason, msg = vmopcondition.ReasonOperationFailed, peerFailureMessage(src, peerName, "")
	}

	if c, ok := conditions.GetCondition(vmopcondition.TypeCompleted, vmop.Status.Conditions); ok && c.Reason == string(vmopcondition.ReasonWaitingForPeer) {
		if cause, waited := strings.CutSuffix(c.Message, awaitingRemovalSuffix(peerName)); waited {
			msg = cause
		}
	}

	return h.fail(ctx, vmop, peer, reason, msg)
}

// removeReceiver deletes the receiver and reports cleanup completion and timeout.
// Keep the source locked until cleanup finishes to prevent both copies from running.
func (h PeerMigrationHandler) removeReceiver(ctx context.Context, peer client.Client, receiver *intvirtv1alpha1.InternalVirtualizationVirtualMachineReceiver) (gone, timedOut bool, err error) {
	if receiver == nil || peer == nil {
		return true, false, nil
	}

	if receiver.DeletionTimestamp.IsZero() {
		if err = peer.Delete(ctx, receiver); client.IgnoreNotFound(err) != nil {
			return false, false, err
		}

		var current intvirtv1alpha1.InternalVirtualizationVirtualMachineReceiver

		err = peer.Get(ctx, client.ObjectKeyFromObject(receiver), &current)
		switch {
		case k8serrors.IsNotFound(err):
			return true, false, nil
		case err != nil:
			return false, false, err
		}

		receiver = &current
	}

	if receiver.DeletionTimestamp.IsZero() {
		return false, false, nil
	}

	return false, time.Since(receiver.DeletionTimestamp.Time) > peerRequestTimeout, nil
}

// apiErrorMessage is the message of an API error without its "<reason>: " decoration, as the user should see it.
func apiErrorMessage(err error) string {
	var status k8serrors.APIStatus
	if errors.As(err, &status) && status.Status().Message != "" {
		return status.Status().Message
	}

	return err.Error()
}

func refusedMessage(peer string, err error) string {
	return fmt.Sprintf("Peer cluster %q refused the credentials of this cluster: %s. Check the pairing of the clusters and the virtualization capability on both sides.", peer, err)
}

func unreachableMessage(peer string) string {
	return fmt.Sprintf("Peer cluster %q is unreachable. The migration continues when the connection is restored. The virtual machine keeps running in this cluster.", peer)
}

// isAPIError tells an answer of the API server of the peer from a failure to reach it.
func isAPIError(err error) bool {
	var status k8serrors.APIStatus
	return errors.As(err, &status)
}

func (h PeerMigrationHandler) progress(vmop *v1alpha2.VirtualMachineOperation, reason vmopcondition.ReasonCompleted, msg string) {
	vmop.Status.Phase = v1alpha2.VMOPPhaseInProgress
	// The peer creates the receiving VM before the copy starts: the local migration shows its target
	// preparation at the same stage.
	h.setProgress(vmop, progressTargetPreparing)
	conditions.SetCondition(conditions.NewConditionBuilder(vmopcondition.TypeCompleted).
		Generation(vmop.GetGeneration()).Reason(reason).Status(metav1.ConditionFalse).Message(msg), &vmop.Status.Conditions)
}

func (h PeerMigrationHandler) complete(vmop *v1alpha2.VirtualMachineOperation, phase v1alpha2.VMOPPhase, reason vmopcondition.ReasonCompleted, msg string) {
	vmop.Status.Phase = phase

	status := metav1.ConditionFalse
	if phase == v1alpha2.VMOPPhaseCompleted {
		status = metav1.ConditionTrue
		vmop.Status.Progress = migrationprogress.FormatPercent(100)
	}

	if h.progressStrategy != nil {
		h.progressStrategy.Forget(vmop.UID)
	}

	conditions.SetCondition(conditions.NewConditionBuilder(vmopcondition.TypeCompleted).
		Generation(vmop.GetGeneration()).Reason(reason).Status(status).Message(msg), &vmop.Status.Conditions)
}

// setProgress shows a stage of the move before the copy starts, when there are no transfer counters yet.
func (h PeerMigrationHandler) setProgress(vmop *v1alpha2.VirtualMachineOperation, progress int32) {
	if vmop.Status.Progress != "" {
		return
	}

	vmop.Status.Progress = migrationprogress.FormatPercent(progress)
}

// syncProgress reads the transfer counters of the outgoing migration: the source runtime fills them on the
// migration object of this cluster, as it does for a migration within the cluster.
func (h PeerMigrationHandler) syncProgress(vmop *v1alpha2.VirtualMachineOperation, src *virtv1.VirtualMachineInstanceMigration) {
	if src == nil || h.progressStrategy == nil {
		return
	}

	record := migrationprogress.BuildRecord(vmop, src, time.Now())
	vmop.Status.Progress = migrationprogress.FormatPercent(h.progressStrategy.SyncProgress(record))
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

	spec, err := commonvm.LoadLastAppliedSpec(kvvm)
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

// setPeerMigrationVMOP suspends ordinary VM reconciliation until this operation has determined the migration outcome.
func (h PeerMigrationHandler) setPeerMigrationVMOP(ctx context.Context, kvvm *virtv1.VirtualMachine, vmop *v1alpha2.VirtualMachineOperation) error {
	if commonvm.PeerMigrationVMOP(kvvm) == vmop.Name {
		return nil
	}

	patch, _ := json.Marshal(map[string]any{"metadata": map[string]any{"annotations": map[string]string{annotations.AnnPeerMigrationVMOP: vmop.Name}}})
	if err := h.client.Patch(ctx, kvvm, client.RawPatch(types.MergePatchType, patch)); err != nil {
		return fmt.Errorf("mark the virtual machine moved to the peer: %w", err)
	}

	return nil
}

// clearPeerMigrationVMOP hands the VM back to the regular handling: the VM controller restores its KubeVirt VM.
func (h PeerMigrationHandler) clearPeerMigrationVMOP(ctx context.Context, vmop *v1alpha2.VirtualMachineOperation) error {
	var vm v1alpha2.VirtualMachine

	err := h.client.Get(ctx, types.NamespacedName{Namespace: vmop.Namespace, Name: vmop.Spec.VirtualMachine}, &vm)
	if k8serrors.IsNotFound(err) {
		return nil
	}

	if err != nil {
		return err
	}

	var kvvm virtv1.VirtualMachine
	if err = h.client.Get(ctx, types.NamespacedName{Namespace: vm.Namespace, Name: vm.Name}, &kvvm); err != nil {
		return client.IgnoreNotFound(err)
	}

	if commonvm.PeerMigrationVMOP(&kvvm) != vmop.Name {
		return nil
	}

	patch, _ := json.Marshal(map[string]any{
		"metadata": map[string]any{"annotations": map[string]any{annotations.AnnPeerMigrationVMOP: nil}},
	})
	if err = h.client.Patch(ctx, &kvvm, client.RawPatch(types.MergePatchType, patch)); err != nil {
		return fmt.Errorf("release the internal virtual machine: %w", err)
	}

	return nil
}

// refreshSyncPins exchanges synchronization endpoints through the receiver. The source endpoint goes
// into receiver.spec, and the receiving endpoint from receiver.status into the source migration annotations.
// Both addresses and pins are refreshed after synchronization controllers restart.
func (h PeerMigrationHandler) refreshSyncPins(ctx context.Context, peer client.Client, vmop *v1alpha2.VirtualMachineOperation, src *virtv1.VirtualMachineInstanceMigration, receiver *intvirtv1alpha1.InternalVirtualizationVirtualMachineReceiver) error {
	if src == nil {
		return nil
	}

	endpoint, err := peering.MigrationSynchronization(ctx, h.client, types.NamespacedName{Namespace: vmop.Namespace, Name: vmop.Spec.VirtualMachine}, "sourceState", src.UID)
	if err != nil {
		return err
	}

	if endpoint.SyncPin != "" && endpoint.SyncAddress == "" {
		endpoint.SyncAddress = receiver.Spec.SourceSynchronization.SyncAddress
	}

	if endpoint.SyncPin != "" && receiver.Spec.SourceSynchronization != endpoint {
		patch, _ := json.Marshal(map[string]any{"spec": map[string]any{"sourceSynchronization": endpoint}})
		if err = peer.Patch(ctx, receiver, client.RawPatch(types.MergePatchType, patch)); err != nil {
			return fmt.Errorf("update the source synchronization endpoint: %w", err)
		}
	}

	return peering.SetMigrationSynchronization(ctx, h.client, client.ObjectKeyFromObject(src), receiver.Status.Synchronization)
}

// makeReceiver is the request to receive: the VM with the spec it runs with, its VirtualDisks with the
// StorageClass and the size they have here, the KubeVirt VMI with the running domain and the address.
func makeReceiver(vm *v1alpha2.VirtualMachine, spec *v1alpha2.VirtualMachineSpec, kvvmi *virtv1.VirtualMachineInstance, vds []*v1alpha2.VirtualDisk, mid string, target peering.Target) (*intvirtv1alpha1.InternalVirtualizationVirtualMachineReceiver, error) {
	rawVM, err := json.Marshal(map[string]any{
		"metadata": map[string]any{"name": vm.Name, "labels": migrationLabels(vm.Labels), "annotations": migrationAnnotations(vm.Annotations)},
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
			"metadata": map[string]any{"name": vd.Name, "labels": migrationLabels(vd.Labels), "annotations": migrationAnnotations(vd.Annotations)},
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

// migrationAnnotations preserves user settings while dropping source controller state.
func migrationAnnotations(m map[string]string) map[string]string {
	out := annotations.ExcludeAnnotations(m)
	// VIOMMU is a user setting despite its internal prefix; the target validates its feature gate.
	if value, ok := m[annotations.AnnEnableVIOMMU]; ok {
		out[annotations.AnnEnableVIOMMU] = value
	}

	for key := range out {
		if strings.HasPrefix(key, "kubectl.kubernetes.io/") {
			delete(out, key)
		}
	}

	for _, key := range []string{
		annotations.AnnMirrorOfVMIP,
		annotations.AnnMigrationIface,
		annotations.AnnMigrationNodeAffinityTerms,
		annotations.AnnVirtualDiskHadOwnerReference,
	} {
		delete(out, key)
	}

	return out
}

func migrationLabels(m map[string]string) map[string]string {
	out := annotations.ExcludeLabels(m)
	for key := range out {
		for _, prefix := range []string{
			annotations.AnnAPIGroup + "/",
			annotations.AnnAPIGroupLegacy + "/",
			annotations.AnnIntegrityGroup,
			annotations.AnnVersionsGroup + "/",
			"e2e.internal.virtualization.deckhouse.io/",
			"kubectl.kubernetes.io/",
		} {
			if strings.HasPrefix(key, prefix) {
				delete(out, key)
				break
			}
		}
	}

	for _, key := range []string{
		"vm.virtualization.deckhouse.io/name",
		annotations.LabelVirtualMachineUID,
		annotations.LabelVirtualMachineIPAddressUID,
		annotations.LabelVirtualMachineMACAddressUID,
		annotations.PVCImportRoleLabel,
		annotations.InhibitNodeShutdownLabel,
		annotations.USBGatewayLabel,
	} {
		delete(out, key)
	}

	return out
}

// receiverConflictMessage gives a recovery action without suggesting cancellation of an active receive.
func receiverConflictMessage(peer, name, namespace string) string {
	return fmt.Sprintf("Peer cluster %q has a previous migration request for VirtualMachine %q in namespace %q. Ask an administrator of that cluster to check the previous migration and remove its request if it has finished, then retry with a new VirtualMachineOperation.", peer, name, namespace)
}
