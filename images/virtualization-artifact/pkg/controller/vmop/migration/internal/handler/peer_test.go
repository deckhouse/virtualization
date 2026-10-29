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
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/ptr"
	virtv1 "kubevirt.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/deckhouse/virtualization-controller/pkg/common/annotations"
	"github.com/deckhouse/virtualization-controller/pkg/common/testutil"
	"github.com/deckhouse/virtualization-controller/pkg/controller/peering"
	"github.com/deckhouse/virtualization-controller/pkg/controller/reconciler"
	"github.com/deckhouse/virtualization-controller/pkg/controller/vmop/migration/internal/service"
	"github.com/deckhouse/virtualization-controller/pkg/eventrecord"
	"github.com/deckhouse/virtualization-controller/pkg/featuregates"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/vmopcondition"
	intvirtv1alpha1 "github.com/deckhouse/virtualization/api/internalvirtualization/v1alpha1"
)

type fakePeers struct {
	client    client.Client
	err       error
	notPaired error
}

func (f fakePeers) Paired(context.Context, string, string) error { return f.notPaired }

func (f fakePeers) Client(context.Context, string) (client.Client, error) { return f.client, f.err }

func (f fakePeers) Target(_ context.Context, _, namespace string) (peering.Target, error) {
	return peering.Target{Namespace: namespace}, nil
}

var _ = Describe("PeerMigrationHandler", func() {
	const ns = "app-x"

	var (
		ctx      context.Context
		local    client.Client
		peer     client.Client
		vmop     *v1alpha2.VirtualMachineOperation
		h        *PeerMigrationHandler
		recorder *eventrecord.EventRecorderLoggerMock
	)

	newVMOP := func() *v1alpha2.VirtualMachineOperation {
		return &v1alpha2.VirtualMachineOperation{
			ObjectMeta: metav1.ObjectMeta{Name: "move", Namespace: ns, UID: "7c4f", Finalizers: []string{v1alpha2.FinalizerVMOPCleanup}},
			Spec: v1alpha2.VirtualMachineOperationSpec{
				Type:           v1alpha2.VMOPTypeMigrate,
				VirtualMachine: "web",
				Migrate:        &v1alpha2.VirtualMachineOperationMigrateSpec{ProjectPeerName: "cluster-b"},
			},
		}
	}

	BeforeEach(func() {
		ctx = testutil.ContextBackgroundWithNoOpLogger()
		vm := &v1alpha2.VirtualMachine{
			ObjectMeta: metav1.ObjectMeta{
				Name: "web", Namespace: ns, UID: "vm-uid",
				Labels:      map[string]string{"app": "web", "vm.virtualization.deckhouse.io/name": "web"},
				Annotations: map[string]string{"owner": "team-a", "kubectl.kubernetes.io/last-applied-configuration": "x"},
			},
			Spec: v1alpha2.VirtualMachineSpec{
				RunPolicy:               v1alpha2.AlwaysOnPolicy,
				VirtualMachineClassName: "generic",
				NodeSelector:            map[string]string{"kubernetes.io/hostname": "a-0"},
				VirtualMachineIPAddress: "web-ip",
				BlockDeviceRefs:         []v1alpha2.BlockDeviceSpecRef{{Kind: v1alpha2.DiskDevice, Name: "web-root"}},
			},
			Status: v1alpha2.VirtualMachineStatus{Phase: v1alpha2.MachineRunning, IPAddress: "10.66.10.5"},
		}
		// The VM runs with a spec before a change that waits for a restart: the peer gets the running one.
		running := vm.Spec.DeepCopy()
		running.VirtualMachineClassName = "generic-old"
		lastApplied, err := json.Marshal(running)
		Expect(err).NotTo(HaveOccurred())
		kvvm := &virtv1.VirtualMachine{
			ObjectMeta: metav1.ObjectMeta{Name: "web", Namespace: ns, Annotations: map[string]string{annotations.AnnVMLastAppliedSpec: string(lastApplied)}},
			Spec: virtv1.VirtualMachineSpec{
				RunStrategy: ptr.To(virtv1.RunStrategyAlways),
				Template:    &virtv1.VirtualMachineInstanceTemplateSpec{},
			},
		}
		vd := &v1alpha2.VirtualDisk{
			ObjectMeta: metav1.ObjectMeta{Name: "web-root", Namespace: ns},
			Status:     v1alpha2.VirtualDiskStatus{Capacity: "4Gi", StorageClassName: "local"},
		}
		kvvmi := &virtv1.VirtualMachineInstance{
			ObjectMeta: metav1.ObjectMeta{Name: "web", Namespace: ns},
			Spec: virtv1.VirtualMachineInstanceSpec{Domain: virtv1.DomainSpec{Devices: virtv1.Devices{
				Disks: []virtv1.Disk{{Name: "vd-web-root", Serial: "3f2a"}},
			}}},
			Status: virtv1.VirtualMachineInstanceStatus{Phase: virtv1.Running, VolumeStatus: []virtv1.VolumeStatus{{Name: "vd-web-root", Size: 4298203136}}},
		}
		local, err = testutil.NewFakeClientWithObjects(vm, kvvm, kvvmi, vd)
		Expect(err).NotTo(HaveOccurred())
		peer, err = testutil.NewFakeClientWithObjects()
		Expect(err).NotTo(HaveOccurred())

		recorder = &eventrecord.EventRecorderLoggerMock{
			EventFunc:  func(_ client.Object, _, _, _ string) {},
			EventfFunc: func(_ client.Object, _, _, _ string, _ ...interface{}) {},
		}
		vmop = newVMOP()
		h = NewPeerMigrationHandler(local, fakePeers{client: peer}, service.NewMigrationService(local, featuregates.Default()), recorder)
	})

	handle := func() {
		GinkgoHelper()
		_, err := h.Handle(ctx, vmop)
		Expect(errors.Is(err, reconciler.ErrStopHandlerChain)).To(BeTrue(), "unexpected error: %v", err)
	}
	completed := func() metav1.Condition {
		GinkgoHelper()
		for _, c := range vmop.Status.Conditions {
			if c.Type == string(vmopcondition.TypeCompleted) {
				return c
			}
		}
		Fail("no Completed condition")
		return metav1.Condition{}
	}
	setReceiverPhase := func(phase intvirtv1alpha1.VirtualMachineReceiverPhase, message string) {
		GinkgoHelper()
		var r intvirtv1alpha1.InternalVirtualizationVirtualMachineReceiver
		Expect(peer.Get(ctx, types.NamespacedName{Namespace: ns, Name: "web"}, &r)).To(Succeed())
		r.Status.Phase, r.Status.Message = phase, message
		r.Status.ReceiverStarted = r.Status.ReceiverStarted || phase == intvirtv1alpha1.ReceiverRunning
		if phase == intvirtv1alpha1.ReceiverReady {
			// What the receiving cluster publishes for the source to start.
			r.Status.SynchronizationAddress, r.Status.SyncPin = "172.17.0.101:9185", "sha256:B"
		}
		// Objects created during the test have no status subresource in the fake client.
		Expect(peer.Update(ctx, &r)).To(Succeed())
	}
	sourceMigration := func() *virtv1.VirtualMachineInstanceMigration {
		GinkgoHelper()
		var mig virtv1.VirtualMachineInstanceMigration
		Expect(local.Get(ctx, types.NamespacedName{Namespace: ns, Name: service.MigrationName(vmop)}, &mig)).To(Succeed())
		return &mig
	}
	setSourcePhase := func(phase virtv1.VirtualMachineInstanceMigrationPhase, state *virtv1.VirtualMachineInstanceMigrationState) {
		GinkgoHelper()
		mig := sourceMigration()
		mig.Status.Phase, mig.Status.MigrationState = phase, state
		Expect(local.Update(ctx, mig)).To(Succeed())
	}
	isGone := func(c client.Client, obj client.Object) bool {
		GinkgoHelper()
		return k8serrors.IsNotFound(c.Get(ctx, client.ObjectKeyFromObject(obj), obj))
	}

	It("asks the peer to receive, starts the migration and deletes the VM once the peer confirms", func() {
		handle()
		Expect(vmop.Status.Phase).To(Equal(v1alpha2.VMOPPhaseInProgress))
		Expect(completed().Reason).To(Equal(string(vmopcondition.ReasonWaitingForPeer)))

		// The VM here is not restarted while it moves.
		var vm v1alpha2.VirtualMachine
		Expect(local.Get(ctx, types.NamespacedName{Namespace: ns, Name: "web"}, &vm)).To(Succeed())
		Expect(vm.Annotations).To(HaveKeyWithValue(annotations.AnnVMMovedToPeer, "move"))
		var kvvm virtv1.VirtualMachine
		Expect(local.Get(ctx, types.NamespacedName{Namespace: ns, Name: "web"}, &kvvm)).To(Succeed())
		Expect(*kvvm.Spec.RunStrategy).To(Equal(virtv1.RunStrategyManual))

		// The peer gets only the request: the VM as it runs, its disk and its domain.
		var r intvirtv1alpha1.InternalVirtualizationVirtualMachineReceiver
		Expect(peer.Get(ctx, types.NamespacedName{Namespace: ns, Name: "web"}, &r)).To(Succeed())
		Expect(r.Labels).To(HaveKeyWithValue(labelPeerVMOPUID, "peering-7c4f"))
		Expect(r.Spec.IPAddress).To(Equal("10.66.10.5"))
		var sent struct {
			Metadata metav1.ObjectMeta           `json:"metadata"`
			Spec     v1alpha2.VirtualMachineSpec `json:"spec"`
		}
		Expect(json.Unmarshal(r.Spec.VirtualMachineTemplate.Raw, &sent)).To(Succeed())
		Expect(sent.Metadata.Labels).To(Equal(map[string]string{"app": "web"}))
		Expect(sent.Metadata.Annotations).To(Equal(map[string]string{"owner": "team-a"}))
		Expect(sent.Spec.VirtualMachineClassName).To(Equal("generic-old"))
		Expect(sent.Spec.NodeSelector).To(HaveKeyWithValue("kubernetes.io/hostname", "a-0"))
		Expect(r.Spec.VirtualDiskTemplates).To(HaveLen(1))
		var disk v1alpha2.VirtualDisk
		Expect(json.Unmarshal(r.Spec.VirtualDiskTemplates[0].Raw, &disk)).To(Succeed())
		Expect(disk.Name).To(Equal("web-root"))
		Expect(*disk.Spec.PersistentVolumeClaim.StorageClass).To(Equal("local"))
		Expect(disk.Spec.PersistentVolumeClaim.Size.String()).To(Equal("4Gi"))
		var instance virtv1.VirtualMachineInstance
		Expect(json.Unmarshal(r.Spec.InternalVirtualizationVirtualMachineInstance.Raw, &instance)).To(Succeed())
		Expect(instance.Spec.Domain.Devices.Disks[0].Serial).To(Equal("3f2a"))
		Expect(instance.Status.VolumeStatus[0].Size).To(Equal(int64(4298203136)))
		Expect(isGone(peer, &v1alpha2.VirtualMachine{ObjectMeta: metav1.ObjectMeta{Namespace: ns, Name: "web"}})).To(BeTrue())

		// Nothing starts until the peer is ready.
		handle()
		Expect(isGone(local, &virtv1.VirtualMachineInstanceMigration{ObjectMeta: metav1.ObjectMeta{Namespace: ns, Name: service.MigrationName(vmop)}})).To(BeTrue())

		setReceiverPhase(intvirtv1alpha1.ReceiverReady, "")
		handle()
		mig := sourceMigration()
		Expect(mig.Spec.SendTo).To(Equal(&virtv1.VirtualMachineInstanceMigrationSource{MigrationID: string(r.UID), ConnectURL: "172.17.0.101:9185"}))

		// After the switchover the VM stays until the peer confirms.
		now := metav1.Now()
		setSourcePhase(virtv1.MigrationSucceeded, &virtv1.VirtualMachineInstanceMigrationState{StartTimestamp: &now, EndTimestamp: &now})
		handle()
		Expect(completed().Reason).To(Equal(string(vmopcondition.ReasonWaitingForPeer)))
		Expect(isGone(local, &v1alpha2.VirtualMachine{ObjectMeta: metav1.ObjectMeta{Namespace: ns, Name: "web"}})).To(BeFalse())

		setReceiverPhase(intvirtv1alpha1.ReceiverRunning, "")
		handle()
		Expect(vmop.Status.Phase).To(Equal(v1alpha2.VMOPPhaseCompleted))
		Expect(isGone(local, &v1alpha2.VirtualDisk{ObjectMeta: metav1.ObjectMeta{Namespace: ns, Name: "web-root"}})).To(BeTrue())
		Expect(isGone(peer, &intvirtv1alpha1.InternalVirtualizationVirtualMachineReceiver{ObjectMeta: metav1.ObjectMeta{Namespace: ns, Name: "web"}})).To(BeTrue())
		Expect(vmop.Finalizers).To(BeEmpty())
	})

	It("does not lock the VM when the request cannot be built", func() {
		Expect(local.Delete(ctx, &v1alpha2.VirtualDisk{ObjectMeta: metav1.ObjectMeta{Namespace: ns, Name: "web-root"}})).To(Succeed())

		handle()

		Expect(vmop.Status.Phase).To(Equal(v1alpha2.VMOPPhaseFailed))
		Expect(completed().Reason).To(Equal(string(vmopcondition.ReasonPeerRejected)))
		Expect(completed().Message).To(ContainSubstring("cannot move to cluster"))
		var vm v1alpha2.VirtualMachine
		Expect(local.Get(ctx, types.NamespacedName{Namespace: ns, Name: "web"}, &vm)).To(Succeed())
		Expect(vm.Annotations).NotTo(HaveKey(annotations.AnnVMMovedToPeer))
		var kvvm virtv1.VirtualMachine
		Expect(local.Get(ctx, types.NamespacedName{Namespace: ns, Name: "web"}, &kvvm)).To(Succeed())
		Expect(*kvvm.Spec.RunStrategy).To(Equal(virtv1.RunStrategyAlways))
		Expect(isGone(peer, &intvirtv1alpha1.InternalVirtualizationVirtualMachineReceiver{ObjectMeta: metav1.ObjectMeta{Namespace: ns, Name: "web"}})).To(BeTrue())
	})

	It("uses the running spec for the disk templates and the deletion", func() {
		data := &v1alpha2.VirtualDisk{
			ObjectMeta: metav1.ObjectMeta{Name: "web-data", Namespace: ns},
			Status:     v1alpha2.VirtualDiskStatus{Capacity: "1Gi", StorageClassName: "local"},
		}
		Expect(local.Create(ctx, data)).To(Succeed())
		var vm v1alpha2.VirtualMachine
		Expect(local.Get(ctx, types.NamespacedName{Namespace: ns, Name: "web"}, &vm)).To(Succeed())
		vm.Spec.BlockDeviceRefs = append(vm.Spec.BlockDeviceRefs, v1alpha2.BlockDeviceSpecRef{Kind: v1alpha2.DiskDevice, Name: "web-data"})
		Expect(local.Update(ctx, &vm)).To(Succeed())

		handle()

		var r intvirtv1alpha1.InternalVirtualizationVirtualMachineReceiver
		Expect(peer.Get(ctx, types.NamespacedName{Namespace: ns, Name: "web"}, &r)).To(Succeed())
		Expect(r.Spec.VirtualDiskTemplates).To(HaveLen(1))
		var sent struct {
			Spec v1alpha2.VirtualMachineSpec `json:"spec"`
		}
		Expect(json.Unmarshal(r.Spec.VirtualMachineTemplate.Raw, &sent)).To(Succeed())
		Expect(sent.Spec.BlockDeviceRefs).To(HaveLen(1))
		Expect(sent.Spec.BlockDeviceRefs[0].Name).To(Equal("web-root"))

		setReceiverPhase(intvirtv1alpha1.ReceiverReady, "")
		handle()
		now := metav1.Now()
		setSourcePhase(virtv1.MigrationSucceeded, &virtv1.VirtualMachineInstanceMigrationState{StartTimestamp: &now, EndTimestamp: &now})
		handle()
		setReceiverPhase(intvirtv1alpha1.ReceiverRunning, "")
		handle()

		Expect(vmop.Status.Phase).To(Equal(v1alpha2.VMOPPhaseCompleted))
		Expect(isGone(local, &v1alpha2.VirtualDisk{ObjectMeta: metav1.ObjectMeta{Namespace: ns, Name: "web-root"}})).To(BeTrue())
		Expect(isGone(local, &v1alpha2.VirtualDisk{ObjectMeta: metav1.ObjectMeta{Namespace: ns, Name: "web-data"}})).To(BeFalse())
	})

	It("rolls back when the peer rejects the VM", func() {
		handle()
		setReceiverPhase(intvirtv1alpha1.ReceiverFailed, "the IP address is not available")
		handle()
		Expect(vmop.Status.Phase).To(Equal(v1alpha2.VMOPPhaseFailed))
		Expect(completed().Reason).To(Equal(string(vmopcondition.ReasonPeerRejected)))
		Expect(completed().Message).To(ContainSubstring("the IP address is not available"))
		Expect(isGone(peer, &intvirtv1alpha1.InternalVirtualizationVirtualMachineReceiver{ObjectMeta: metav1.ObjectMeta{Namespace: ns, Name: "web"}})).To(BeTrue())
		var vm v1alpha2.VirtualMachine
		Expect(local.Get(ctx, types.NamespacedName{Namespace: ns, Name: "web"}, &vm)).To(Succeed())
		Expect(vm.Annotations).NotTo(HaveKey(annotations.AnnVMMovedToPeer))
		// The guest restarts here as before the move.
		var kvvm virtv1.VirtualMachine
		Expect(local.Get(ctx, types.NamespacedName{Namespace: ns, Name: "web"}, &kvvm)).To(Succeed())
		Expect(*kvvm.Spec.RunStrategy).To(Equal(virtv1.RunStrategyAlways))
		Expect(kvvm.Annotations).NotTo(HaveKey(annRunStrategyBeforeMove))
	})

	It("removes the lock and the request a stale copy of a failed operation left", func() {
		handle()
		setReceiverPhase(intvirtv1alpha1.ReceiverFailed, "the StorageClass \"nfs\" of the disk \"web-root\" does not exist")
		stale := vmop.DeepCopy()
		handle()
		Expect(vmop.Status.Phase).To(Equal(v1alpha2.VMOPPhaseFailed))

		// The stale copy asks the peer again; its status write then loses to the failure.
		_, _ = h.Handle(ctx, stale)
		Expect(isGone(peer, &intvirtv1alpha1.InternalVirtualizationVirtualMachineReceiver{ObjectMeta: metav1.ObjectMeta{Namespace: ns, Name: "web"}})).To(BeFalse())

		handle()
		Expect(vmop.Status.Phase).To(Equal(v1alpha2.VMOPPhaseFailed))
		Expect(isGone(peer, &intvirtv1alpha1.InternalVirtualizationVirtualMachineReceiver{ObjectMeta: metav1.ObjectMeta{Namespace: ns, Name: "web"}})).To(BeTrue())
		var vm v1alpha2.VirtualMachine
		Expect(local.Get(ctx, types.NamespacedName{Namespace: ns, Name: "web"}, &vm)).To(Succeed())
		Expect(vm.Annotations).NotTo(HaveKey(annotations.AnnVMMovedToPeer))
		var kvvm virtv1.VirtualMachine
		Expect(local.Get(ctx, types.NamespacedName{Namespace: ns, Name: "web"}, &kvvm)).To(Succeed())
		Expect(*kvvm.Spec.RunStrategy).To(Equal(virtv1.RunStrategyAlways))

		// Once cleaned up, the operation does not ask the peer again.
		h.peers = fakePeers{err: errors.New("the peer is asked")}
		recorder.EventFunc = func(_ client.Object, _, _, msg string) { Fail(msg) }
		handle()
	})

	It("completes a move whose VM is gone after the migration was sent", func() {
		handle()
		setReceiverPhase(intvirtv1alpha1.ReceiverReady, "")
		handle()
		// The move deleted the VM and the request, and its status was not saved.
		Expect(local.Delete(ctx, sourceMigration())).To(Succeed())
		Expect(local.Delete(ctx, &v1alpha2.VirtualMachine{ObjectMeta: metav1.ObjectMeta{Namespace: ns, Name: "web"}})).To(Succeed())
		Expect(peer.Delete(ctx, &intvirtv1alpha1.InternalVirtualizationVirtualMachineReceiver{ObjectMeta: metav1.ObjectMeta{Namespace: ns, Name: "web"}})).To(Succeed())

		handle()
		Expect(vmop.Status.Phase).To(Equal(v1alpha2.VMOPPhaseCompleted))
		Expect(vmop.Finalizers).To(BeEmpty())
	})

	It("does not touch a request of another migration in the peer", func() {
		other := &intvirtv1alpha1.InternalVirtualizationVirtualMachineReceiver{ObjectMeta: metav1.ObjectMeta{Namespace: ns, Name: "web", Labels: map[string]string{labelPeerVMOPUID: "peering-other"}}}
		Expect(peer.Create(ctx, other)).To(Succeed())
		handle()
		Expect(vmop.Status.Phase).To(Equal(v1alpha2.VMOPPhaseFailed))
		Expect(completed().Reason).To(Equal(string(vmopcondition.ReasonPeerRejected)))
		Expect(isGone(peer, other)).To(BeFalse())
	})

	It("does not start the migration until the peer publishes the key of its synchronization controller", func() {
		handle()
		setReceiverPhase(intvirtv1alpha1.ReceiverReady, "")
		var r intvirtv1alpha1.InternalVirtualizationVirtualMachineReceiver
		Expect(peer.Get(ctx, types.NamespacedName{Namespace: ns, Name: "web"}, &r)).To(Succeed())
		r.Status.SyncPin = ""
		Expect(peer.Update(ctx, &r)).To(Succeed())
		handle()
		Expect(completed().Reason).To(Equal(string(vmopcondition.ReasonWaitingForPeer)))
		Expect(isGone(local, &virtv1.VirtualMachineInstanceMigration{ObjectMeta: metav1.ObjectMeta{Namespace: ns, Name: service.MigrationName(vmop)}})).To(BeTrue())
	})

	It("fails when the peer does not publish the key of its synchronization controller in time", func() {
		handle()
		setReceiverPhase(intvirtv1alpha1.ReceiverReady, "")
		var r intvirtv1alpha1.InternalVirtualizationVirtualMachineReceiver
		Expect(peer.Get(ctx, types.NamespacedName{Namespace: ns, Name: "web"}, &r)).To(Succeed())
		r.Status.SyncPin = ""
		Expect(peer.Update(ctx, &r)).To(Succeed())
		h2 := *h
		// A peer that took the whole prepare timeout still has time to publish the key.
		h2.peers = fakePeers{client: stalePeer{Client: peer, created: time.Now().Add(-peerPrepareTimeout - time.Minute)}}
		_, err := h2.Handle(ctx, vmop)
		Expect(errors.Is(err, reconciler.ErrStopHandlerChain)).To(BeTrue())
		Expect(completed().Reason).To(Equal(string(vmopcondition.ReasonWaitingForPeer)))
		h2.peers = fakePeers{client: stalePeer{Client: peer, created: time.Now().Add(-peerPrepareTimeout - peerPinTimeout - time.Minute)}}
		_, err = h2.Handle(ctx, vmop)
		Expect(errors.Is(err, reconciler.ErrStopHandlerChain)).To(BeTrue())
		Expect(completed().Reason).To(Equal(string(vmopcondition.ReasonPeerRejected)))
		Expect(completed().Message).To(ContainSubstring("same version"))
		Expect(isGone(local, &virtv1.VirtualMachineInstanceMigration{ObjectMeta: metav1.ObjectMeta{Namespace: ns, Name: service.MigrationName(vmop)}})).To(BeTrue())
	})

	It("fails when the peer does not prepare the receiver in time", func() {
		handle()
		var r intvirtv1alpha1.InternalVirtualizationVirtualMachineReceiver
		Expect(peer.Get(ctx, types.NamespacedName{Namespace: ns, Name: "web"}, &r)).To(Succeed())
		r.Status.Message = "waiting for the disk"
		h2 := *h
		h2.peers = fakePeers{client: stalePeer{Client: peer, created: time.Now().Add(-peerPrepareTimeout - time.Minute)}}
		Expect(peer.Update(ctx, &r)).To(Succeed())
		_, err := h2.Handle(ctx, vmop)
		Expect(errors.Is(err, reconciler.ErrStopHandlerChain)).To(BeTrue())
		Expect(completed().Reason).To(Equal(string(vmopcondition.ReasonPeerRejected)))
		Expect(completed().Message).To(ContainSubstring("waiting for the disk"))
	})

	It("fails a peer that does not support virtualization", func() {
		h.peers = fakePeers{err: peering.ErrPeerNotSupported}
		handle()
		Expect(vmop.Status.Phase).To(Equal(v1alpha2.VMOPPhaseFailed))
		Expect(completed().Reason).To(Equal(string(vmopcondition.ReasonPeerNotSupported)))
	})

	It("fails a move of a project that is no longer paired with the peer", func() {
		h.peers = fakePeers{client: peer, notPaired: peering.ErrPeerNotPaired}
		handle()
		Expect(vmop.Status.Phase).To(Equal(v1alpha2.VMOPPhaseFailed))
		Expect(isGone(peer, &intvirtv1alpha1.InternalVirtualizationVirtualMachineReceiver{ObjectMeta: metav1.ObjectMeta{Namespace: ns, Name: "web"}})).To(BeTrue())
		var vm v1alpha2.VirtualMachine
		Expect(local.Get(ctx, types.NamespacedName{Namespace: ns, Name: "web"}, &vm)).To(Succeed())
		Expect(vm.Annotations).NotTo(HaveKey(annotations.AnnVMMovedToPeer))
	})

	It("shows a handler error on the operation", func() {
		h.peers = fakePeers{err: errors.New("peer is down")}
		_, err := h.Handle(ctx, vmop)
		Expect(err).To(HaveOccurred())
		Expect(completed().Reason).To(Equal(string(vmopcondition.ReasonOperationFailed)))
		Expect(completed().Message).To(ContainSubstring("peer is down"))
	})

	It("cancels before the switchover when the operation is deleted", func() {
		handle()
		setReceiverPhase(intvirtv1alpha1.ReceiverReady, "")
		handle()
		vmop.DeletionTimestamp = ptr.To(metav1.Now())
		handle() // the migration here goes first
		handle()
		Expect(isGone(peer, &intvirtv1alpha1.InternalVirtualizationVirtualMachineReceiver{ObjectMeta: metav1.ObjectMeta{Namespace: ns, Name: "web"}})).To(BeTrue())
		Expect(vmop.Finalizers).To(BeEmpty())
	})

	It("finishes the move when the operation is deleted after the guest arrived", func() {
		handle()
		var r intvirtv1alpha1.InternalVirtualizationVirtualMachineReceiver
		Expect(peer.Get(ctx, types.NamespacedName{Namespace: ns, Name: "web"}, &r)).To(Succeed())
		// The fake client does not assign a UID: the receiving migration is named after it.
		r.UID = types.UID("receiver-uid")
		Expect(peer.Update(ctx, &r)).To(Succeed())
		// The peer serves the migration only under the group of the kube-api-rewriter.
		received := &unstructured.Unstructured{Object: map[string]any{"status": map[string]any{"phase": string(virtv1.MigrationSucceeded)}}}
		received.SetGroupVersionKind(peerMigrationGVK)
		received.SetNamespace(ns)
		received.SetName(string(r.UID))
		Expect(peer.Create(ctx, received)).To(Succeed())

		vmop.DeletionTimestamp = ptr.To(metav1.Now())
		handle()

		// The operation itself is deleting; the move must finish, not roll back.
		Expect(vmop.Status.Phase).To(Equal(v1alpha2.VMOPPhaseTerminating))
		Expect(isGone(local, &v1alpha2.VirtualMachine{ObjectMeta: metav1.ObjectMeta{Namespace: ns, Name: "web"}})).To(BeTrue())
		Expect(isGone(peer, &intvirtv1alpha1.InternalVirtualizationVirtualMachineReceiver{ObjectMeta: metav1.ObjectMeta{Namespace: ns, Name: "web"}})).To(BeFalse())
	})

	It("releases a deleted operation when the peer stays unreachable", func() {
		handle()
		h.peers = fakePeers{client: refusingPeer{Client: peer}}
		vmop.DeletionTimestamp = ptr.To(metav1.NewTime(time.Now().Add(-peerDeletionTimeout - time.Minute)))
		handle()

		Expect(vmop.Finalizers).To(BeEmpty())
		Expect(vmop.Status.Phase).To(Equal(v1alpha2.VMOPPhaseTerminating))
		var vm v1alpha2.VirtualMachine
		Expect(local.Get(ctx, types.NamespacedName{Namespace: ns, Name: "web"}, &vm)).To(Succeed())
		Expect(vm.Annotations).NotTo(HaveKey(annotations.AnnVMMovedToPeer))
	})

	It("keeps a deleted operation while its migration may still switch the guest over", func() {
		handle()
		setReceiverPhase(intvirtv1alpha1.ReceiverReady, "")
		handle()
		setSourcePhase(virtv1.MigrationRunning, nil)
		h.peers = fakePeers{client: refusingPeer{Client: peer}}
		vmop.DeletionTimestamp = ptr.To(metav1.NewTime(time.Now().Add(-peerDeletionTimeout - time.Minute)))
		handle()

		Expect(vmop.Finalizers).To(ContainElement(v1alpha2.FinalizerVMOPCleanup))
		var vm v1alpha2.VirtualMachine
		Expect(local.Get(ctx, types.NamespacedName{Namespace: ns, Name: "web"}, &vm)).To(Succeed())
		Expect(vm.Annotations).To(HaveKeyWithValue(annotations.AnnVMMovedToPeer, "move"))
	})

	It("hands a cancelled operation back stopped when the receiver has not run but the VM does not run here", func() {
		handle()
		setReceiverPhase(intvirtv1alpha1.ReceiverReady, "")
		var kvvmi virtv1.VirtualMachineInstance
		Expect(local.Get(ctx, types.NamespacedName{Namespace: ns, Name: "web"}, &kvvmi)).To(Succeed())
		Expect(local.Delete(ctx, &kvvmi)).To(Succeed())
		vmop.DeletionTimestamp = ptr.To(metav1.Now())
		handle()

		Expect(isGone(peer, &intvirtv1alpha1.InternalVirtualizationVirtualMachineReceiver{ObjectMeta: metav1.ObjectMeta{Namespace: ns, Name: "web"}})).To(BeTrue())
		Expect(vmop.Finalizers).To(BeEmpty())
		var vm v1alpha2.VirtualMachine
		Expect(local.Get(ctx, types.NamespacedName{Namespace: ns, Name: "web"}, &vm)).To(Succeed())
		Expect(vm.Annotations).NotTo(HaveKey(annotations.AnnVMMovedToPeer))
		Expect(vm.Spec.RunPolicy).To(Equal(v1alpha2.ManualPolicy))
	})

	It("hands a deleted operation back stopped when the VM does not run here", func() {
		handle()
		var kvvmi virtv1.VirtualMachineInstance
		Expect(local.Get(ctx, types.NamespacedName{Namespace: ns, Name: "web"}, &kvvmi)).To(Succeed())
		Expect(local.Delete(ctx, &kvvmi)).To(Succeed())
		h.peers = fakePeers{client: refusingPeer{Client: peer}}
		vmop.DeletionTimestamp = ptr.To(metav1.NewTime(time.Now().Add(-peerDeletionTimeout - time.Minute)))
		handle()

		Expect(vmop.Finalizers).To(BeEmpty())
		var vm v1alpha2.VirtualMachine
		Expect(local.Get(ctx, types.NamespacedName{Namespace: ns, Name: "web"}, &vm)).To(Succeed())
		Expect(vm.Annotations).NotTo(HaveKey(annotations.AnnVMMovedToPeer))
		Expect(vm.Spec.RunPolicy).To(Equal(v1alpha2.ManualPolicy))
	})

	It("does not delete a VM created again under the name of the moved one", func() {
		Expect(h.deleteSource(ctx, vmop)).To(Succeed())
		Expect(isGone(local, &v1alpha2.VirtualMachine{ObjectMeta: metav1.ObjectMeta{Namespace: ns, Name: "web"}})).To(BeFalse())
		Expect(isGone(local, &v1alpha2.VirtualDisk{ObjectMeta: metav1.ObjectMeta{Namespace: ns, Name: "web-root"}})).To(BeFalse())
	})

	It("fails when the synchronization with the peer does not start", func() {
		handle()
		setReceiverPhase(intvirtv1alpha1.ReceiverReady, "")
		handle()
		mig := sourceMigration()
		mig.CreationTimestamp = metav1.NewTime(time.Now().Add(-peerSyncTimeout - time.Minute))
		h2 := *h
		h2.migration = service.NewMigrationService(fakeGetter{Client: local, mig: mig}, featuregates.Default())
		_, err := h2.Handle(ctx, vmop)
		Expect(errors.Is(err, reconciler.ErrStopHandlerChain)).To(BeTrue())
		Expect(completed().Reason).To(Equal(string(vmopcondition.ReasonMigrationNetworkUnreachable)))
	})

	It("shows why the receiving pod does not start in the peer", func() {
		handle()
		setReceiverPhase(intvirtv1alpha1.ReceiverReady, "")
		handle()
		setReceiverPhase(intvirtv1alpha1.ReceiverReady, "the receiving pod cannot be scheduled: Unschedulable: 0/4 nodes are available")
		mig := sourceMigration()
		withCreated := func(created time.Time) PeerMigrationHandler {
			m := mig.DeepCopy()
			m.CreationTimestamp = metav1.NewTime(created)
			h2 := *h
			h2.migration = service.NewMigrationService(fakeGetter{Client: local, mig: m}, featuregates.Default())
			return h2
		}

		h2 := withCreated(time.Now())
		_, err := h2.Handle(ctx, vmop)
		Expect(errors.Is(err, reconciler.ErrStopHandlerChain)).To(BeTrue())
		Expect(vmop.Status.Phase).To(Equal(v1alpha2.VMOPPhaseInProgress))
		Expect(completed().Message).To(ContainSubstring(`in cluster "cluster-b" the receiving pod cannot be scheduled`))

		h2 = withCreated(time.Now().Add(-peerSyncTimeout - time.Minute))
		_, err = h2.Handle(ctx, vmop)
		Expect(errors.Is(err, reconciler.ErrStopHandlerChain)).To(BeTrue())
		Expect(completed().Reason).To(Equal(string(vmopcondition.ReasonPeerRejected)))
		Expect(completed().Message).To(ContainSubstring("0/4 nodes are available"))
	})

	It("keeps the VM stopped when the peer does not confirm", func() {
		handle()
		setReceiverPhase(intvirtv1alpha1.ReceiverReady, "")
		handle()
		long := metav1.NewTime(time.Now().Add(-peerConfirmTimeout - time.Minute))
		setSourcePhase(virtv1.MigrationSucceeded, &virtv1.VirtualMachineInstanceMigrationState{StartTimestamp: &long, EndTimestamp: &long})
		handle()
		Expect(vmop.Status.Phase).To(Equal(v1alpha2.VMOPPhaseFailed))
		Expect(completed().Reason).To(Equal(string(vmopcondition.ReasonPeerNotConfirmed)))
		// The VM is handed back stopped: startable by hand, not by its policy.
		var vm v1alpha2.VirtualMachine
		Expect(local.Get(ctx, types.NamespacedName{Namespace: ns, Name: "web"}, &vm)).To(Succeed())
		Expect(vm.Annotations).NotTo(HaveKey(annotations.AnnVMMovedToPeer))
		Expect(vm.Spec.RunPolicy).To(Equal(v1alpha2.ManualPolicy))
		var kvvm virtv1.VirtualMachine
		Expect(local.Get(ctx, types.NamespacedName{Namespace: ns, Name: "web"}, &kvvm)).To(Succeed())
		Expect(*kvvm.Spec.RunStrategy).To(Equal(virtv1.RunStrategyManual))
	})

	It("finishes the move when the peer reports the arrival but does not confirm the start in time", func() {
		handle()
		setReceiverPhase(intvirtv1alpha1.ReceiverReady, "")
		handle()
		var r intvirtv1alpha1.InternalVirtualizationVirtualMachineReceiver
		Expect(peer.Get(ctx, types.NamespacedName{Namespace: ns, Name: "web"}, &r)).To(Succeed())
		r.Status.ReceiverStarted = true
		Expect(peer.Update(ctx, &r)).To(Succeed())
		long := metav1.NewTime(time.Now().Add(-peerConfirmTimeout - time.Minute))
		setSourcePhase(virtv1.MigrationSucceeded, &virtv1.VirtualMachineInstanceMigrationState{StartTimestamp: &long, EndTimestamp: &long})
		handle()
		Expect(vmop.Status.Phase).To(Equal(v1alpha2.VMOPPhaseCompleted))
		Expect(isGone(local, &v1alpha2.VirtualMachine{ObjectMeta: metav1.ObjectMeta{Namespace: ns, Name: "web"}})).To(BeTrue())
		// The request stays for the peer to finish with: deleting it would cancel the receive there.
		Expect(isGone(peer, &r)).To(BeFalse())
	})

	It("fails the move when the peer refuses the credentials of this cluster", func() {
		h.peers = fakePeers{client: refusingPeer{Client: peer}}
		handle()
		Expect(vmop.Status.Phase).To(Equal(v1alpha2.VMOPPhaseFailed))
		Expect(completed().Reason).To(Equal(string(vmopcondition.ReasonPeerRejected)))
		Expect(completed().Message).To(ContainSubstring("refused the credentials"))
		var vm v1alpha2.VirtualMachine
		Expect(local.Get(ctx, types.NamespacedName{Namespace: ns, Name: "web"}, &vm)).To(Succeed())
		Expect(vm.Annotations).NotTo(HaveKey(annotations.AnnVMMovedToPeer), "the VM is not left locked")
	})

	It("keeps a deleted operation locked while the peer cannot be asked", func() {
		handle()
		h.peers = fakePeers{client: refusingPeer{Client: peer}}
		vmop.DeletionTimestamp = ptr.To(metav1.Now())
		handle()
		Expect(vmop.Finalizers).To(ContainElement(v1alpha2.FinalizerVMOPCleanup))
		Expect(vmop.Status.Phase).NotTo(Equal(v1alpha2.VMOPPhaseFailed))
		var vm v1alpha2.VirtualMachine
		Expect(local.Get(ctx, types.NamespacedName{Namespace: ns, Name: "web"}, &vm)).To(Succeed())
		Expect(vm.Annotations).To(HaveKeyWithValue(annotations.AnnVMMovedToPeer, "move"))
		Expect(isGone(peer, &intvirtv1alpha1.InternalVirtualizationVirtualMachineReceiver{ObjectMeta: metav1.ObjectMeta{Namespace: ns, Name: "web"}})).To(BeFalse())
		h.peers = fakePeers{client: peer}
		handle()
		Expect(vmop.Finalizers).To(BeEmpty())
	})

	DescribeTable("keeps a possibly created receiver when its state is unknown", func(peerError error, deleting, withMigration bool) {
		handle()
		if withMigration {
			setReceiverPhase(intvirtv1alpha1.ReceiverReady, "")
			handle()
		}
		if deleting {
			vmop.DeletionTimestamp = ptr.To(metav1.Now())
		}
		if peerError != nil {
			h.peers = fakePeers{err: peerError}
		} else {
			h.peers = fakePeers{client: refusingPeer{Client: peer}}
		}
		result, err := h.Handle(ctx, vmop)
		Expect(errors.Is(err, reconciler.ErrStopHandlerChain)).To(BeTrue(), "unexpected error: %v", err)
		Expect(result.RequeueAfter).To(Equal(peerRequeue))
		Expect(vmop.Status.Phase).NotTo(Equal(v1alpha2.VMOPPhaseFailed))
		Expect(vmop.Finalizers).To(ContainElement(v1alpha2.FinalizerVMOPCleanup))
		var vm v1alpha2.VirtualMachine
		Expect(local.Get(ctx, types.NamespacedName{Namespace: ns, Name: "web"}, &vm)).To(Succeed())
		Expect(vm.Annotations).To(HaveKeyWithValue(annotations.AnnVMMovedToPeer, "move"))
		Expect(isGone(peer, &intvirtv1alpha1.InternalVirtualizationVirtualMachineReceiver{ObjectMeta: metav1.ObjectMeta{Namespace: ns, Name: "web"}})).To(BeFalse())
		if withMigration {
			Expect(isGone(local, &virtv1.VirtualMachineInstanceMigration{ObjectMeta: metav1.ObjectMeta{Namespace: ns, Name: service.MigrationName(vmop)}})).To(BeFalse())
		}
	},
		Entry("unsupported peer during deletion", peering.ErrPeerNotSupported, true, true),
		Entry("unauthorized receiver GET with migration", nil, false, true),
		Entry("unauthorized receiver GET during deletion", nil, true, true),
	)

	DescribeTable("fails and releases the VM when the peer refuses before the migration", func(peers fakePeers, reason vmopcondition.ReasonCompleted) {
		handle()
		h.peers = peers
		handle()
		Expect(vmop.Status.Phase).To(Equal(v1alpha2.VMOPPhaseFailed))
		Expect(completed().Reason).To(Equal(string(reason)))
		var vm v1alpha2.VirtualMachine
		Expect(local.Get(ctx, types.NamespacedName{Namespace: ns, Name: "web"}, &vm)).To(Succeed())
		Expect(vm.Annotations).NotTo(HaveKey(annotations.AnnVMMovedToPeer))
		Expect(vm.Spec.RunPolicy).To(Equal(v1alpha2.AlwaysOnPolicy))
	},
		Entry("unsupported peer", fakePeers{err: peering.ErrPeerNotSupported}, vmopcondition.ReasonPeerNotSupported),
		Entry("refused receiver GET", fakePeers{client: refusingPeer{Client: peer}}, vmopcondition.ReasonPeerRejected),
	)

	It("fails and releases the VM when the peer refuses to create the request", func() {
		h.peers = fakePeers{client: refusingCreatePeer{Client: peer}}
		handle()
		Expect(vmop.Status.Phase).To(Equal(v1alpha2.VMOPPhaseFailed))
		Expect(completed().Reason).To(Equal(string(vmopcondition.ReasonPeerRejected)))
		var vm v1alpha2.VirtualMachine
		Expect(local.Get(ctx, types.NamespacedName{Namespace: ns, Name: "web"}, &vm)).To(Succeed())
		Expect(vm.Annotations).NotTo(HaveKey(annotations.AnnVMMovedToPeer))
	})

	It("waits for an unreachable peer without its raw error", func() {
		handle()
		h.peers = fakePeers{client: unreachablePeer{Client: peer}}
		handle()
		Expect(vmop.Status.Phase).To(Equal(v1alpha2.VMOPPhaseInProgress))
		Expect(completed().Reason).To(Equal(string(vmopcondition.ReasonWaitingForPeer)))
		Expect(completed().Message).To(Equal(`Cluster "cluster-b" is unreachable. The migration continues when the connection is restored. The virtual machine keeps running in this cluster.`))
	})

	It("hands back the stopped VM after a switched migration when receiver GET is refused", func() {
		handle()
		setReceiverPhase(intvirtv1alpha1.ReceiverReady, "")
		handle()
		long := metav1.NewTime(time.Now().Add(-peerConfirmTimeout - time.Minute))
		setSourcePhase(virtv1.MigrationSucceeded, &virtv1.VirtualMachineInstanceMigrationState{EndTimestamp: &long})
		h.peers = fakePeers{client: refusingPeer{Client: peer}}
		handle()
		Expect(vmop.Status.Phase).To(Equal(v1alpha2.VMOPPhaseFailed))
		Expect(completed().Reason).To(Equal(string(vmopcondition.ReasonPeerNotConfirmed)))
		var vm v1alpha2.VirtualMachine
		Expect(local.Get(ctx, types.NamespacedName{Namespace: ns, Name: "web"}, &vm)).To(Succeed())
		Expect(vm.Spec.RunPolicy).To(Equal(v1alpha2.ManualPolicy))
		Expect(vm.Annotations).NotTo(HaveKey(annotations.AnnVMMovedToPeer))
		Expect(isGone(peer, &intvirtv1alpha1.InternalVirtualizationVirtualMachineReceiver{ObjectMeta: metav1.ObjectMeta{Namespace: ns, Name: "web"}})).To(BeFalse())
	})

	It("hands the VM back at once when the peer reports that its VM did not start after the switchover", func() {
		handle()
		setReceiverPhase(intvirtv1alpha1.ReceiverReady, "")
		handle()
		now := metav1.Now()
		setSourcePhase(virtv1.MigrationSucceeded, &virtv1.VirtualMachineInstanceMigrationState{StartTimestamp: &now, EndTimestamp: &now})
		setReceiverPhase(intvirtv1alpha1.ReceiverFailed, "the receiving pod did not start")
		handle()
		Expect(vmop.Status.Phase).To(Equal(v1alpha2.VMOPPhaseFailed))
		Expect(completed().Reason).To(Equal(string(vmopcondition.ReasonPeerNotConfirmed)))
		Expect(completed().Message).To(ContainSubstring("the receiving pod did not start"))
		var vm v1alpha2.VirtualMachine
		Expect(local.Get(ctx, types.NamespacedName{Namespace: ns, Name: "web"}, &vm)).To(Succeed())
		Expect(vm.Spec.RunPolicy).To(Equal(v1alpha2.ManualPolicy))
		Expect(isGone(peer, &intvirtv1alpha1.InternalVirtualizationVirtualMachineReceiver{ObjectMeta: metav1.ObjectMeta{Namespace: ns, Name: "web"}})).To(BeTrue())
	})

	It("hands back a switched VM safely even when the operation is deleted and the peer refuses GET", func() {
		handle()
		setReceiverPhase(intvirtv1alpha1.ReceiverReady, "")
		handle()
		long := metav1.NewTime(time.Now().Add(-peerConfirmTimeout - time.Minute))
		setSourcePhase(virtv1.MigrationSucceeded, &virtv1.VirtualMachineInstanceMigrationState{EndTimestamp: &long})
		vmop.DeletionTimestamp = ptr.To(metav1.Now())
		h.peers = fakePeers{client: refusingPeer{Client: peer}}
		handle()
		Expect(vmop.Status.Phase).To(Equal(v1alpha2.VMOPPhaseFailed))
		Expect(vmop.Finalizers).To(BeEmpty())
		var vm v1alpha2.VirtualMachine
		Expect(local.Get(ctx, types.NamespacedName{Namespace: ns, Name: "web"}, &vm)).To(Succeed())
		Expect(vm.Spec.RunPolicy).To(Equal(v1alpha2.ManualPolicy))
		Expect(isGone(peer, &intvirtv1alpha1.InternalVirtualizationVirtualMachineReceiver{ObjectMeta: metav1.ObjectMeta{Namespace: ns, Name: "web"}})).To(BeFalse())
	})

	It("keeps waiting for the confirmation when the peer cannot be asked after the switchover", func() {
		handle()
		setReceiverPhase(intvirtv1alpha1.ReceiverReady, "")
		handle()
		now := metav1.Now()
		setSourcePhase(virtv1.MigrationSucceeded, &virtv1.VirtualMachineInstanceMigrationState{StartTimestamp: &now, EndTimestamp: &now})
		h.peers = fakePeers{err: peering.ErrPeerNotSupported}
		handle()
		Expect(vmop.Status.Phase).To(Equal(v1alpha2.VMOPPhaseInProgress))
		Expect(completed().Reason).To(Equal(string(vmopcondition.ReasonWaitingForPeer)))
		var kvvm virtv1.VirtualMachine
		Expect(local.Get(ctx, types.NamespacedName{Namespace: ns, Name: "web"}, &kvvm)).To(Succeed())
		Expect(*kvvm.Spec.RunStrategy).To(Equal(virtv1.RunStrategyManual))
	})

	It("finishes the move when the VM has run in the peer although the migration here failed", func() {
		handle()
		setReceiverPhase(intvirtv1alpha1.ReceiverReady, "")
		handle()
		setReceiverPhase(intvirtv1alpha1.ReceiverRunning, "")
		setSourcePhase(virtv1.MigrationFailed, nil)
		handle()
		Expect(vmop.Status.Phase).To(Equal(v1alpha2.VMOPPhaseCompleted))
		Expect(isGone(local, &v1alpha2.VirtualMachine{ObjectMeta: metav1.ObjectMeta{Namespace: ns, Name: "web"}})).To(BeTrue())
		Expect(isGone(peer, &intvirtv1alpha1.InternalVirtualizationVirtualMachineReceiver{ObjectMeta: metav1.ObjectMeta{Namespace: ns, Name: "web"}})).To(BeTrue())
	})

	It("finishes the move when the operation is deleted after the VM has run in the peer", func() {
		handle()
		setReceiverPhase(intvirtv1alpha1.ReceiverReady, "")
		handle()
		vmop.DeletionTimestamp = ptr.To(metav1.Now())
		handle() // the migration here goes first
		setReceiverPhase(intvirtv1alpha1.ReceiverRunning, "")
		handle()
		Expect(vmop.Finalizers).To(BeEmpty())
		Expect(isGone(local, &v1alpha2.VirtualMachine{ObjectMeta: metav1.ObjectMeta{Namespace: ns, Name: "web"}})).To(BeTrue())
		Expect(isGone(peer, &intvirtv1alpha1.InternalVirtualizationVirtualMachineReceiver{ObjectMeta: metav1.ObjectMeta{Namespace: ns, Name: "web"}})).To(BeTrue())
	})
})

// fakeGetter returns the given migration with its creation time, which the fake client does not keep.
type fakeGetter struct {
	client.Client
	mig *virtv1.VirtualMachineInstanceMigration
}

func (f fakeGetter) Get(ctx context.Context, key client.ObjectKey, obj client.Object, opts ...client.GetOption) error {
	if m, ok := obj.(*virtv1.VirtualMachineInstanceMigration); ok && key.Name == f.mig.Name {
		*m = *f.mig.DeepCopy()
		return nil
	}
	return f.Client.Get(ctx, key, obj, opts...)
}

// stalePeer returns the request to receive as created long ago, which the fake client does not let set.
// refusingPeer answers every read as a peer that does not accept the credentials of this cluster.
type refusingPeer struct {
	client.Client
}

func (r refusingPeer) Get(context.Context, client.ObjectKey, client.Object, ...client.GetOption) error {
	return k8serrors.NewUnauthorized("the server has asked for the client to provide credentials")
}

type refusingCreatePeer struct {
	client.Client
}

func (r refusingCreatePeer) Create(context.Context, client.Object, ...client.CreateOption) error {
	return k8serrors.NewForbidden(intvirtv1alpha1.SchemeGroupVersion.WithResource("internalvirtualizationvirtualmachinereceivers").GroupResource(), "web", errors.New("access denied"))
}

type stalePeer struct {
	client.Client
	created time.Time
}

func (s stalePeer) Get(ctx context.Context, key client.ObjectKey, obj client.Object, opts ...client.GetOption) error {
	if err := s.Client.Get(ctx, key, obj, opts...); err != nil {
		return err
	}
	if r, ok := obj.(*intvirtv1alpha1.InternalVirtualizationVirtualMachineReceiver); ok {
		r.CreationTimestamp = metav1.NewTime(s.created)
	}
	return nil
}

type unreachablePeer struct {
	client.Client
}

func (r unreachablePeer) Get(context.Context, client.ObjectKey, client.Object, ...client.GetOption) error {
	return errors.New("dial tcp 10.0.0.1:443: connect: connection refused")
}
