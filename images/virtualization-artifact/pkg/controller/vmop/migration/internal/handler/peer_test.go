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
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/ptr"
	virtv1 "kubevirt.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/deckhouse/virtualization-controller/pkg/common/annotations"
	"github.com/deckhouse/virtualization-controller/pkg/common/testutil"
	"github.com/deckhouse/virtualization-controller/pkg/controller/peering"
	"github.com/deckhouse/virtualization-controller/pkg/controller/reconciler"
	migrationprogress "github.com/deckhouse/virtualization-controller/pkg/controller/vmop/migration/internal/progress"
	"github.com/deckhouse/virtualization-controller/pkg/controller/vmop/migration/internal/service"
	"github.com/deckhouse/virtualization-controller/pkg/eventrecord"
	"github.com/deckhouse/virtualization-controller/pkg/featuregates"
	coordinationv1alpha1 "github.com/deckhouse/virtualization/api/coordination/v1alpha1"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/vmcondition"
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
				RunStrategy: ptr.To(virtv1.RunStrategyManual),
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
		h = NewPeerMigrationHandler(local, local, fakePeers{client: peer}, service.NewMigrationService(local, featuregates.Default()), recorder)
	})

	// movedMark is the operation the KubeVirt VM here is marked moved by.
	movedMark := func() string {
		GinkgoHelper()

		var kvvm virtv1.VirtualMachine
		if err := local.Get(ctx, types.NamespacedName{Namespace: ns, Name: "web"}, &kvvm); err != nil {
			Expect(k8serrors.IsNotFound(err)).To(BeTrue())
			return ""
		}

		return kvvm.Annotations[annotations.AnnPeerMigrationVMOP]
	}
	// showMove does what the VM controller does with the mark: the VM shows the move in its condition. It
	// reports whether the VM began to show the move.
	showMove := func() bool {
		GinkgoHelper()

		var vm v1alpha2.VirtualMachine
		if err := local.Get(ctx, types.NamespacedName{Namespace: ns, Name: "web"}, &vm); err != nil {
			return false
		}

		marked := movedMark() != ""

		shown := meta.IsStatusConditionTrue(vm.Status.Conditions, vmcondition.TypeMovedToPeer.String())
		switch {
		case marked && !shown:
			meta.SetStatusCondition(&vm.Status.Conditions, metav1.Condition{Type: vmcondition.TypeMovedToPeer.String(), Status: metav1.ConditionTrue, Reason: "InProgress"})
		case !marked && shown:
			meta.RemoveStatusCondition(&vm.Status.Conditions, vmcondition.TypeMovedToPeer.String())
		default:
			return false
		}

		Expect(local.Status().Update(ctx, &vm)).To(Succeed())

		return marked
	}
	handleResult := func() reconcile.Result {
		GinkgoHelper()

		res, err := h.Handle(ctx, vmop)
		Expect(errors.Is(err, reconciler.ErrStopHandlerChain)).To(BeTrue(), "unexpected error: %v", err)
		// The VM controller shows the move, and the operation goes on once the VM shows it.
		if showMove() {
			res, err = h.Handle(ctx, vmop)
			Expect(errors.Is(err, reconciler.ErrStopHandlerChain)).To(BeTrue(), "unexpected error: %v", err)
			showMove()
		}

		return res
	}
	handle := func() {
		GinkgoHelper()
		handleResult()
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
			r.Status.Synchronization.SyncAddress, r.Status.Synchronization.SyncPin = "172.17.0.101:9185", "sha256:B"
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
	// finishRequest lets the request of the peer go, as the peer does once its receiving migration is over.
	finishRequest := func() {
		GinkgoHelper()

		var r intvirtv1alpha1.InternalVirtualizationVirtualMachineReceiver
		Expect(peer.Get(ctx, types.NamespacedName{Namespace: ns, Name: "web"}, &r)).To(Succeed())
		r.Finalizers = nil
		Expect(peer.Update(ctx, &r)).To(Succeed())

		if !isGone(peer, &r) {
			Expect(peer.Delete(ctx, &r)).To(Succeed())
		}
	}

	// The VM here is locked once it shows the move: a request created before that could carry a spec the
	// user changes in between.
	It("asks the peer only once the VM shows the move", func() {
		res, err := h.Handle(ctx, vmop)
		Expect(errors.Is(err, reconciler.ErrStopHandlerChain)).To(BeTrue(), "unexpected error: %v", err)
		Expect(res.RequeueAfter).To(Equal(movedConditionRequeue))
		Expect(movedMark()).To(Equal("move"))

		var kvvm virtv1.VirtualMachine
		Expect(local.Get(ctx, types.NamespacedName{Namespace: ns, Name: "web"}, &kvvm)).To(Succeed())
		Expect(*kvvm.Spec.RunStrategy).To(Equal(virtv1.RunStrategyManual))
		Expect(isGone(peer, &intvirtv1alpha1.InternalVirtualizationVirtualMachineReceiver{ObjectMeta: metav1.ObjectMeta{Namespace: ns, Name: "web"}})).To(BeTrue())

		Expect(showMove()).To(BeTrue())
		handle()
		Expect(isGone(peer, &intvirtv1alpha1.InternalVirtualizationVirtualMachineReceiver{ObjectMeta: metav1.ObjectMeta{Namespace: ns, Name: "web"}})).To(BeFalse())
	})

	handleWithoutShow := func() {
		GinkgoHelper()

		_, err := h.Handle(ctx, vmop)
		Expect(errors.Is(err, reconciler.ErrStopHandlerChain)).To(BeTrue(), "unexpected error: %v", err)
	}

	DescribeTable("rejects snapshotting or freezing before migration starts and permits a later retry", func(condition vmcondition.Type, stage int) {
		var before virtv1.VirtualMachineInstance

		key := types.NamespacedName{Namespace: ns, Name: "web"}
		Expect(local.Get(ctx, key, &before)).To(Succeed())

		switch stage {
		case 1:
			handleWithoutShow()
		case 2:
			handle()
			setReceiverPhase(intvirtv1alpha1.ReceiverReady, "")
		}

		var vm v1alpha2.VirtualMachine
		Expect(local.Get(ctx, key, &vm)).To(Succeed())
		meta.SetStatusCondition(&vm.Status.Conditions, metav1.Condition{Type: condition.String(), Status: metav1.ConditionTrue, Reason: "InProgress"})
		Expect(local.Status().Update(ctx, &vm)).To(Succeed())

		handle()
		Expect(vmop.Status.Phase).To(Equal(v1alpha2.VMOPPhaseFailed))
		Expect(completed().Message).To(ContainSubstring("retry the migration"))
		Expect(movedMark()).To(BeEmpty())
		Expect(isGone(peer, &intvirtv1alpha1.InternalVirtualizationVirtualMachineReceiver{ObjectMeta: metav1.ObjectMeta{Name: key.Name, Namespace: key.Namespace}})).To(BeTrue())
		Expect(isGone(local, &virtv1.VirtualMachineInstanceMigration{ObjectMeta: metav1.ObjectMeta{Name: service.MigrationName(vmop), Namespace: ns}})).To(BeTrue())

		var after virtv1.VirtualMachineInstance
		Expect(local.Get(ctx, key, &after)).To(Succeed())
		Expect(after).To(Equal(before))
		Expect(local.Get(ctx, key, &vm)).To(Succeed())
		Expect(vm.Spec.RunPolicy).To(Equal(v1alpha2.AlwaysOnPolicy))

		meta.RemoveStatusCondition(&vm.Status.Conditions, condition.String())
		Expect(local.Status().Update(ctx, &vm)).To(Succeed())

		vmop = newVMOP()

		handle()
		setReceiverPhase(intvirtv1alpha1.ReceiverReady, "")
		handle()
		Expect(vmop.Status.Phase).To(Equal(v1alpha2.VMOPPhaseInProgress))
		Expect(sourceMigration().Spec.SendTo).NotTo(BeNil())
	},
		Entry("snapshot before source preparation", vmcondition.TypeSnapshotting, 0),
		Entry("freeze before source preparation", vmcondition.TypeFilesystemFrozen, 0),
		Entry("snapshot while waiting for source preparation", vmcondition.TypeSnapshotting, 1),
		Entry("freeze while waiting for source preparation", vmcondition.TypeFilesystemFrozen, 1),
		Entry("snapshot after receiver preparation", vmcondition.TypeSnapshotting, 2),
		Entry("freeze after receiver preparation", vmcondition.TypeFilesystemFrozen, 2),
	)

	It("keeps waiting for the VM to show the move before the deadline", func() {
		handleWithoutShow()

		vmop.CreationTimestamp = metav1.NewTime(time.Now().Add(-peerSourcePreparationTimeout + time.Minute))

		handleWithoutShow()
		Expect(vmop.Status.Phase).To(Equal(v1alpha2.VMOPPhaseInProgress))
		Expect(movedMark()).To(Equal("move"))
	})

	// The VM controller may never show the move: the operation would wait for good with the VM locked.
	It("gives the move up when the VM does not show it in time", func() {
		handleWithoutShow()

		vmop.CreationTimestamp = metav1.NewTime(time.Now().Add(-peerSourcePreparationTimeout - time.Minute))

		handleWithoutShow()
		Expect(vmop.Status.Phase).To(Equal(v1alpha2.VMOPPhaseFailed))
		Expect(completed().Reason).To(Equal(string(vmopcondition.ReasonOperationFailed)))
		Expect(completed().Message).To(Equal(`The migration to peer cluster "cluster-b" did not start: the VM controller did not confirm source preparation within 5m0s of operation creation. The virtual machine keeps running in this cluster; retry the move.`))
		Expect(movedMark()).To(BeEmpty())

		var kvvm virtv1.VirtualMachine
		Expect(local.Get(ctx, types.NamespacedName{Namespace: ns, Name: "web"}, &kvvm)).To(Succeed())
		Expect(isGone(peer, &intvirtv1alpha1.InternalVirtualizationVirtualMachineReceiver{ObjectMeta: metav1.ObjectMeta{Namespace: ns, Name: "web"}})).To(BeTrue())
	})

	It("creates the source migration with its peer endpoint before it is visible in the cache", func() {
		handle()
		setReceiverPhase(intvirtv1alpha1.ReceiverReady, "")

		var created *virtv1.VirtualMachineInstanceMigration

		h.client = interceptor.NewClient(local.(client.WithWatch), interceptor.Funcs{
			Create: func(ctx context.Context, cl client.WithWatch, obj client.Object, opts ...client.CreateOption) error {
				if migration, ok := obj.(*virtv1.VirtualMachineInstanceMigration); ok {
					created = migration.DeepCopy()
				}

				return cl.Create(ctx, obj, opts...)
			},
			Get: func(ctx context.Context, cl client.WithWatch, key client.ObjectKey, obj client.Object, opts ...client.GetOption) error {
				if _, ok := obj.(*virtv1.VirtualMachineInstanceMigration); ok {
					return k8serrors.NewNotFound(virtv1.Resource("virtualmachineinstancemigrations"), key.Name)
				}

				return cl.Get(ctx, key, obj, opts...)
			},
		})

		handle()
		Expect(created).NotTo(BeNil())
		Expect(created.Annotations).To(HaveKeyWithValue("kubevirt.io/migration-peer-sync-address", "172.17.0.101:9185"))
		Expect(created.Annotations).To(HaveKeyWithValue("kubevirt.io/migration-peer-sync-pin", "sha256:B"))
		Expect(sourceMigration().Annotations).To(Equal(created.Annotations))
		Expect(vmop.Status.Phase).To(Equal(v1alpha2.VMOPPhaseInProgress))
	})

	It("preserves user VM and disk metadata without copying source controller state", func() {
		key := types.NamespacedName{Namespace: ns, Name: "web"}

		var vm v1alpha2.VirtualMachine
		Expect(local.Get(ctx, key, &vm)).To(Succeed())

		userAnnotations := map[string]string{
			annotations.AnnVideo:           "vga",
			annotations.AnnOsType:          "Windows",
			annotations.AnnEnableVIOMMU:    "true",
			"owner":                        "team-a",
			"example.deckhouse.io/setting": "keep",
		}

		vm.Annotations = map[string]string{}
		for k, v := range userAnnotations {
			vm.Annotations[k] = v
		}

		for _, k := range []string{
			annotations.AnnPeerMigrationVMOP, annotations.AnnVMLastAppliedSpec,
			annotations.AnnVMStartRequested, annotations.AnnVMRestartRequested,
			annotations.AnnVMRestorePowerState, annotations.AnnVMFilesystemRequest,
			annotations.AnnMigrationNodeAffinityTerms,
			annotations.AnnMirrorOfVMIP, annotations.AnnMigrationIface,
			annotations.AnnVirtualDiskOriginalAnnotations, annotations.AnnVirtualDiskHadOwnerReference,
			"kubevirt.io/migration-peer-sync-pin", "network.deckhouse.io/networks-status",
			"resource-quota-overrides.deckhouse.io/ignore", "security.deckhouse.io/skip-pss-check",
			"kubectl.kubernetes.io/last-applied-configuration",
		} {
			vm.Annotations[k] = "source-state"
		}

		userLabels := map[string]string{"app": "web", "example.deckhouse.io/team": "a"}
		vm.Labels = map[string]string{
			"app": "web", "example.deckhouse.io/team": "a",
			"vm.virtualization.deckhouse.io/name":          "web",
			annotations.LabelVirtualMachineUID:             "source-uid",
			annotations.LiveMigratableLabel:                "true",
			peering.LabelReceiverVMOPUID:                   "old-operation",
			annotations.QuotaExcludeLabel:                  "true",
			annotations.SkipPodSecurityStandardsCheckLabel: "true",
			"kubevirt.io/created-by":                       "old-instance",
		}
		Expect(local.Update(ctx, &vm)).To(Succeed())

		var vd v1alpha2.VirtualDisk
		Expect(local.Get(ctx, types.NamespacedName{Namespace: ns, Name: "web-root"}, &vd)).To(Succeed())
		vd.Labels, vd.Annotations = vm.Labels, vm.Annotations
		Expect(local.Update(ctx, &vd)).To(Succeed())

		handle()

		var receiver intvirtv1alpha1.InternalVirtualizationVirtualMachineReceiver
		Expect(peer.Get(ctx, key, &receiver)).To(Succeed())
		sent, err := peering.ParseReceiver(&receiver)
		Expect(err).NotTo(HaveOccurred())
		Expect(sent.VirtualMachine.Annotations).To(Equal(userAnnotations))
		Expect(sent.VirtualMachine.Labels).To(Equal(userLabels))
		Expect(sent.VirtualDisks).To(HaveLen(1))
		Expect(sent.VirtualDisks[0].Annotations).To(Equal(userAnnotations))
		Expect(sent.VirtualDisks[0].Labels).To(Equal(userLabels))

		var original v1alpha2.VirtualMachine
		Expect(local.Get(ctx, key, &original)).To(Succeed())
		Expect(original.Annotations).To(Equal(vm.Annotations))
		Expect(original.Labels).To(Equal(vm.Labels))
	})

	It("waits for a snapshot in progress before asking the peer to receive", func() {
		vmop.CreationTimestamp = metav1.Now()
		claim := &coordinationv1alpha1.VirtualMachineAccessClaim{
			ObjectMeta: metav1.ObjectMeta{
				Name: "vms-daily-01-uid", Namespace: ns, CreationTimestamp: metav1.NewTime(vmop.CreationTimestamp.Add(-time.Minute)),
				OwnerReferences: []metav1.OwnerReference{{
					APIVersion: v1alpha2.SchemeGroupVersion.String(), Kind: v1alpha2.VirtualMachineSnapshotKind,
					Name: "daily-01", UID: "daily-01-uid", Controller: ptr.To(true),
				}},
			},
			Spec: coordinationv1alpha1.VirtualMachineAccessClaimSpec{
				VirtualMachine: coordinationv1alpha1.VirtualMachineAccessClaimTarget{Name: "web", UID: "vm-uid"},
				Access:         []coordinationv1alpha1.AccessMode{coordinationv1alpha1.AccessModeSnapshot},
			},
			Status: coordinationv1alpha1.VirtualMachineAccessClaimStatus{Phase: coordinationv1alpha1.AccessClaimPhaseGranted},
		}
		Expect(local.Create(ctx, claim)).To(Succeed())

		handle()
		Expect(vmop.Status.Phase).To(Equal(v1alpha2.VMOPPhasePending))
		Expect(completed().Reason).To(Equal(string(vmopcondition.ReasonWaitingForSnapshot)))
		Expect(completed().Message).To(Equal(`Waiting for the VirtualMachineSnapshot "daily-01" to complete.`))
		Expect(isGone(peer, &intvirtv1alpha1.InternalVirtualizationVirtualMachineReceiver{ObjectMeta: metav1.ObjectMeta{Namespace: ns, Name: "web"}})).To(BeTrue())

		claim.Status.Phase = coordinationv1alpha1.AccessClaimPhaseCompleted
		Expect(local.Status().Update(ctx, claim)).To(Succeed())
		handle()
		Expect(completed().Reason).To(Equal(string(vmopcondition.ReasonWaitingForPeer)))
		Expect(isGone(peer, &intvirtv1alpha1.InternalVirtualizationVirtualMachineReceiver{ObjectMeta: metav1.ObjectMeta{Namespace: ns, Name: "web"}})).To(BeFalse())
	})

	It("asks the peer to receive, starts the migration and deletes the VM once the peer confirms", func() {
		handle()
		Expect(vmop.Status.Phase).To(Equal(v1alpha2.VMOPPhaseInProgress))
		Expect(completed().Reason).To(Equal(string(vmopcondition.ReasonWaitingForPeer)))

		// The VM here is not restarted while it moves.
		var vm v1alpha2.VirtualMachine
		Expect(local.Get(ctx, types.NamespacedName{Namespace: ns, Name: "web"}, &vm)).To(Succeed())
		Expect(movedMark()).To(Equal("move"))

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

	DescribeTable("does not change KubeVirt runStrategy when marking, releasing, or handing back a move", func(strategy *virtv1.VirtualMachineRunStrategy) {
		var kvvm virtv1.VirtualMachine

		key := types.NamespacedName{Namespace: ns, Name: "web"}
		Expect(local.Get(ctx, key, &kvvm)).To(Succeed())
		kvvm.Spec.RunStrategy = strategy
		Expect(local.Update(ctx, &kvvm)).To(Succeed())
		before := kvvm.Spec.DeepCopy()
		Expect(h.setPeerMigrationVMOP(ctx, &kvvm, vmop)).To(Succeed())
		Expect(local.Get(ctx, key, &kvvm)).To(Succeed())
		Expect(&kvvm.Spec).To(Equal(before))
		Expect(kvvm.Annotations).To(HaveKeyWithValue(annotations.AnnPeerMigrationVMOP, vmop.Name))
		Expect(h.clearPeerMigrationVMOP(ctx, vmop)).To(Succeed())
		Expect(local.Get(ctx, key, &kvvm)).To(Succeed())
		Expect(&kvvm.Spec).To(Equal(before))
		Expect(h.setPeerMigrationVMOP(ctx, &kvvm, vmop)).To(Succeed())
		Expect(h.releaseVMWithManualPolicy(ctx, vmop)).To(Succeed())
		Expect(local.Get(ctx, key, &kvvm)).To(Succeed())
		Expect(&kvvm.Spec).To(Equal(before))
	},
		Entry("Manual", ptr.To(virtv1.RunStrategyManual)),
		Entry("Always", ptr.To(virtv1.RunStrategyAlways)),
		Entry("Halted", ptr.To(virtv1.RunStrategyHalted)),
		Entry("unset", (*virtv1.VirtualMachineRunStrategy)(nil)),
	)

	It("does not lock the VM when the request cannot be built", func() {
		Expect(local.Delete(ctx, &v1alpha2.VirtualDisk{ObjectMeta: metav1.ObjectMeta{Namespace: ns, Name: "web-root"}})).To(Succeed())

		handle()

		Expect(vmop.Status.Phase).To(Equal(v1alpha2.VMOPPhaseFailed))
		Expect(completed().Reason).To(Equal(string(vmopcondition.ReasonPeerRejected)))
		Expect(completed().Message).To(ContainSubstring("cannot move to peer cluster"))

		var vm v1alpha2.VirtualMachine
		Expect(local.Get(ctx, types.NamespacedName{Namespace: ns, Name: "web"}, &vm)).To(Succeed())
		Expect(movedMark()).To(BeEmpty())

		var kvvm virtv1.VirtualMachine
		Expect(local.Get(ctx, types.NamespacedName{Namespace: ns, Name: "web"}, &kvvm)).To(Succeed())
		Expect(*kvvm.Spec.RunStrategy).To(Equal(virtv1.RunStrategyManual))
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
		Expect(movedMark()).To(BeEmpty())
		// Ordinary power management resumes with the strategy unchanged.
		var kvvm virtv1.VirtualMachine
		Expect(local.Get(ctx, types.NamespacedName{Namespace: ns, Name: "web"}, &kvvm)).To(Succeed())
		Expect(*kvvm.Spec.RunStrategy).To(Equal(virtv1.RunStrategyManual))
	})

	DescribeTable("keeps a failed operation locked until stale peer cleanup finishes", func(deleting bool) {
		handle()
		setReceiverPhase(intvirtv1alpha1.ReceiverReady, "")
		handle()
		setSourcePhase(virtv1.MigrationRunning, nil)

		var r intvirtv1alpha1.InternalVirtualizationVirtualMachineReceiver
		Expect(peer.Get(ctx, types.NamespacedName{Namespace: ns, Name: "web"}, &r)).To(Succeed())
		r.Finalizers = []string{peering.FinalizerReceiverCleanup}
		Expect(peer.Update(ctx, &r)).To(Succeed())

		vmop.Status.Phase = v1alpha2.VMOPPhaseFailed
		if deleting {
			vmop.DeletionTimestamp = ptr.To(metav1.Now())
		}

		Expect(handleResult().RequeueAfter).To(Equal(peerRequeue))
		Expect(movedMark()).To(Equal("move"))
		Expect(vmop.Finalizers).To(ContainElement(v1alpha2.FinalizerVMOPCleanup))
		Expect(peer.Get(ctx, types.NamespacedName{Namespace: ns, Name: "web"}, &r)).To(Succeed())
		Expect(r.DeletionTimestamp).NotTo(BeNil())
		finishRequest()
		handle()
		Expect(movedMark()).To(BeEmpty())

		if deleting {
			Expect(vmop.Finalizers).NotTo(ContainElement(v1alpha2.FinalizerVMOPCleanup))
		}
	}, Entry("after failure", false), Entry("while deleting a failed operation", true))

	It("keeps a failed operation locked until its source abort finishes", func() {
		handle()
		setReceiverPhase(intvirtv1alpha1.ReceiverReady, "")
		handle()

		mig := sourceMigration()
		mig.Finalizers = []string{"test/abort-pending"}
		mig.Status.Phase = virtv1.MigrationRunning
		Expect(local.Update(ctx, mig)).To(Succeed())

		vmop.Status.Phase = v1alpha2.VMOPPhaseFailed

		Expect(handleResult().RequeueAfter).To(Equal(peerRequeue))
		Expect(movedMark()).To(Equal("move"))

		mig = sourceMigration()
		Expect(mig.DeletionTimestamp).NotTo(BeNil())
		mig.Status.Phase = virtv1.MigrationFailed
		Expect(local.Update(ctx, mig)).To(Succeed())
		handle()
		Expect(movedMark()).To(BeEmpty())
	})

	It("keeps the run policy of a running source when failed peer cleanup times out", func() {
		handle()
		setReceiverPhase(intvirtv1alpha1.ReceiverReady, "")
		handle()
		setSourcePhase(virtv1.MigrationFailed, nil)

		vmop.Status.Phase = v1alpha2.VMOPPhaseFailed
		h.peers = fakePeers{client: terminatingPeer{Client: peer, since: time.Now().Add(-peerRequestTimeout - time.Minute)}}

		handle()
		Expect(movedMark()).To(BeEmpty())

		var vm v1alpha2.VirtualMachine
		Expect(local.Get(ctx, types.NamespacedName{Namespace: ns, Name: "web"}, &vm)).To(Succeed())
		Expect(vm.Spec.RunPolicy).To(Equal(v1alpha2.AlwaysOnPolicy))

		var kvvm virtv1.VirtualMachine
		Expect(local.Get(ctx, types.NamespacedName{Namespace: ns, Name: "web"}, &kvvm)).To(Succeed())
		Expect(kvvm.Spec.RunStrategy).To(Equal(ptr.To(virtv1.RunStrategyManual)))
	})

	It("does not unlock failed cleanup while the peer outcome is unknown", func() {
		handle()
		setReceiverPhase(intvirtv1alpha1.ReceiverReady, "")
		handle()
		h.complete(vmop, v1alpha2.VMOPPhaseFailed, vmopcondition.ReasonPeerRejected, "original failure")
		h.peers = fakePeers{err: errors.New("peer unavailable")}
		_, err := h.Handle(ctx, vmop)
		Expect(err).To(MatchError("peer unavailable"))
		Expect(movedMark()).To(Equal("move"))
		Expect(vmop.Finalizers).To(ContainElement(v1alpha2.FinalizerVMOPCleanup))
		Expect(vmop.Status.Phase).To(Equal(v1alpha2.VMOPPhaseFailed))
		Expect(completed().Reason).To(Equal(string(vmopcondition.ReasonPeerRejected)))
		Expect(completed().Message).To(Equal("original failure"))

		h.peers = fakePeers{client: peer}

		handle()
		handle()
		Expect(vmop.Status.Phase).To(Equal(v1alpha2.VMOPPhaseFailed))
		Expect(isGone(local, &virtv1.VirtualMachineInstanceMigration{ObjectMeta: metav1.ObjectMeta{Namespace: ns, Name: service.MigrationName(vmop)}})).To(BeTrue())
	})

	DescribeTable("keeps source data when failed cleanup has no confirmation from the receiver", func(receiverFailed bool) {
		handle()
		setReceiverPhase(intvirtv1alpha1.ReceiverReady, "")
		handle()

		now := metav1.Now()
		setSourcePhase(virtv1.MigrationSucceeded, &virtv1.VirtualMachineInstanceMigrationState{EndTimestamp: &now})

		vmop.Status.Phase = v1alpha2.VMOPPhaseFailed

		if receiverFailed {
			setReceiverPhase(intvirtv1alpha1.ReceiverFailed, "receiver did not start")
		} else {
			Expect(peer.Delete(ctx, &intvirtv1alpha1.InternalVirtualizationVirtualMachineReceiver{ObjectMeta: metav1.ObjectMeta{Namespace: ns, Name: "web"}})).To(Succeed())
		}

		handle()
		Expect(vmop.Status.Phase).To(Equal(v1alpha2.VMOPPhaseFailed))
		Expect(isGone(local, &v1alpha2.VirtualMachine{ObjectMeta: metav1.ObjectMeta{Namespace: ns, Name: "web"}})).To(BeFalse())
		Expect(isGone(local, &v1alpha2.VirtualDisk{ObjectMeta: metav1.ObjectMeta{Namespace: ns, Name: "web-root"}})).To(BeFalse())

		if !receiverFailed {
			Expect(movedMark()).To(Equal("move"))

			past := metav1.NewTime(time.Now().Add(-peerConfirmTimeout - time.Minute))
			setSourcePhase(virtv1.MigrationSucceeded, &virtv1.VirtualMachineInstanceMigrationState{EndTimestamp: &past})
			handle()
		}

		Expect(completed().Reason).To(Equal(string(vmopcondition.ReasonPeerNotConfirmed)))

		var vm v1alpha2.VirtualMachine
		Expect(local.Get(ctx, types.NamespacedName{Namespace: ns, Name: "web"}, &vm)).To(Succeed())
		Expect(vm.Spec.RunPolicy).To(Equal(v1alpha2.ManualPolicy))
	}, Entry("receiver failed", true), Entry("receiver missing", false))

	It("keeps source data and hands back a failed successful source when the peer stays unavailable", func() {
		handle()
		setReceiverPhase(intvirtv1alpha1.ReceiverReady, "")
		handle()

		past := metav1.NewTime(time.Now().Add(-peerConfirmTimeout - time.Minute))
		setSourcePhase(virtv1.MigrationSucceeded, &virtv1.VirtualMachineInstanceMigrationState{EndTimestamp: &past})

		vmop.Status.Phase = v1alpha2.VMOPPhaseFailed
		h.peers = fakePeers{err: errors.New("peer unavailable")}

		handle()
		Expect(vmop.Status.Phase).To(Equal(v1alpha2.VMOPPhaseFailed))
		Expect(completed().Reason).To(Equal(string(vmopcondition.ReasonPeerNotConfirmed)))
		Expect(isGone(local, &v1alpha2.VirtualDisk{ObjectMeta: metav1.ObjectMeta{Namespace: ns, Name: "web-root"}})).To(BeFalse())

		var vm v1alpha2.VirtualMachine
		Expect(local.Get(ctx, types.NamespacedName{Namespace: ns, Name: "web"}, &vm)).To(Succeed())
		Expect(vm.Spec.RunPolicy).To(Equal(v1alpha2.ManualPolicy))
		Expect(movedMark()).To(BeEmpty())
	})

	It("keeps the source fenced while failure cancellation is still running after the peer cleanup timeout", func() {
		handle()
		setReceiverPhase(intvirtv1alpha1.ReceiverReady, "")
		handle()

		mig := sourceMigration()
		mig.Finalizers = []string{"test/abort-pending"}
		mig.Status.Phase = virtv1.MigrationRunning
		Expect(local.Update(ctx, mig)).To(Succeed())
		setReceiverPhase(intvirtv1alpha1.ReceiverFailed, "receiver failed")

		h.peers = fakePeers{client: terminatingPeer{Client: peer, since: time.Now().Add(-peerRequestTimeout - time.Minute)}}

		handle()
		Expect(movedMark()).To(Equal("move"))
		Expect(sourceMigration().DeletionTimestamp).NotTo(BeNil())
		Expect(vmop.Finalizers).To(ContainElement(v1alpha2.FinalizerVMOPCleanup))

		mig = sourceMigration()
		mig.Status.Phase = virtv1.MigrationFailed
		Expect(local.Update(ctx, mig)).To(Succeed())
		handle()
		Expect(movedMark()).To(BeEmpty())
		Expect(vmop.Status.Phase).To(Equal(v1alpha2.VMOPPhaseFailed))
	})

	It("finishes a failed operation whose stale migration arrived at the peer", func() {
		handle()
		setReceiverPhase(intvirtv1alpha1.ReceiverReady, "")
		handle()
		setSourcePhase(virtv1.MigrationRunning, nil)

		vmop.Status.Phase = v1alpha2.VMOPPhaseFailed

		setReceiverPhase(intvirtv1alpha1.ReceiverRunning, "")
		handle()
		Expect(vmop.Status.Phase).To(Equal(v1alpha2.VMOPPhaseCompleted))
		Expect(isGone(local, &v1alpha2.VirtualMachine{ObjectMeta: metav1.ObjectMeta{Namespace: ns, Name: "web"}})).To(BeTrue())
	})

	It("links the operation to its VM by uid and keeps the link after a rejection", func() {
		handle()
		Expect(vmop.Labels).To(HaveKeyWithValue(annotations.LabelVirtualMachineUID, "vm-uid"))
		setReceiverPhase(intvirtv1alpha1.ReceiverFailed, "the IP address is not available")
		handle()
		Expect(vmop.Status.Phase).To(Equal(v1alpha2.VMOPPhaseFailed))
		Expect(vmop.Labels).To(HaveKeyWithValue(annotations.LabelVirtualMachineUID, "vm-uid"))
	})

	It("links a finished operation that has no link yet to its VM", func() {
		vmop.CreationTimestamp = metav1.NewTime(time.Now().UTC().Truncate(time.Second))
		vmop.Status.Phase = v1alpha2.VMOPPhaseCompleted

		handle()
		Expect(vmop.Labels).To(HaveKeyWithValue(annotations.LabelVirtualMachineUID, "vm-uid"))
	})

	It("does not link the operation to a VM created after it", func() {
		var vm v1alpha2.VirtualMachine
		Expect(local.Get(ctx, types.NamespacedName{Namespace: ns, Name: "web"}, &vm)).To(Succeed())
		vm.CreationTimestamp = metav1.NewTime(time.Now())
		Expect(local.Update(ctx, &vm)).To(Succeed())

		vmop.CreationTimestamp = metav1.NewTime(time.Now().Add(-time.Hour))
		vmop.Status.Phase = v1alpha2.VMOPPhaseCompleted

		handle()
		Expect(vmop.Labels).NotTo(HaveKey(annotations.LabelVirtualMachineUID))
	})

	DescribeTable("does not backfill a finished operation onto a recreated VM in the same timestamp second", func(phase v1alpha2.VMOPPhase) {
		var vm v1alpha2.VirtualMachine
		Expect(local.Get(ctx, types.NamespacedName{Namespace: ns, Name: "web"}, &vm)).To(Succeed())
		Expect(local.Delete(ctx, &vm)).To(Succeed())
		// The API timestamps cannot distinguish the old operation from a VM recreated within that second.
		stamp := metav1.NewTime(time.Now().UTC().Truncate(time.Second))
		vmop.CreationTimestamp = stamp
		vmop.Status.Phase = phase
		vm.UID = "new-vm-uid"
		vm.ResourceVersion = ""
		vm.CreationTimestamp = stamp
		Expect(local.Create(ctx, &vm)).To(Succeed())
		handle()
		Expect(vmop.Labels).NotTo(HaveKey(annotations.LabelVirtualMachineUID), "a historical operation must not become history of the new VM")
	},
		Entry("completed", v1alpha2.VMOPPhaseCompleted),
		Entry("failed", v1alpha2.VMOPPhaseFailed),
	)

	It("links an active operation created in the same second as its VM and keeps the link when finished", func() {
		var vm v1alpha2.VirtualMachine
		Expect(local.Get(ctx, types.NamespacedName{Namespace: ns, Name: "web"}, &vm)).To(Succeed())

		stamp := metav1.NewTime(time.Now().UTC().Truncate(time.Second))
		vm.CreationTimestamp = stamp
		Expect(local.Update(ctx, &vm)).To(Succeed())

		vmop.CreationTimestamp = stamp

		handle()
		Expect(vmop.Labels).To(HaveKeyWithValue(annotations.LabelVirtualMachineUID, "vm-uid"), "a new move must stay visible in the console")
		vmop.Status.Phase = v1alpha2.VMOPPhaseCompleted

		handle()
		Expect(vmop.Labels).To(HaveKeyWithValue(annotations.LabelVirtualMachineUID, "vm-uid"), "the established UID link must survive completion")
	})

	It("keeps the link of an operation whose VM is gone", func() {
		vmop.Labels = map[string]string{annotations.LabelVirtualMachineUID: "old-uid"}
		vmop.Status.Phase = v1alpha2.VMOPPhaseCompleted

		Expect(local.Delete(ctx, &v1alpha2.VirtualMachine{ObjectMeta: metav1.ObjectMeta{Namespace: ns, Name: "web"}})).To(Succeed())
		handle()
		Expect(vmop.Labels).To(HaveKeyWithValue(annotations.LabelVirtualMachineUID, "old-uid"))
	})

	It("removes the lock and the request a stale copy of a failed operation left", func() {
		handle()
		setReceiverPhase(intvirtv1alpha1.ReceiverFailed, "the StorageClass \"nfs\" of the disk \"web-root\" does not exist")

		stale := vmop.DeepCopy()

		handle()
		Expect(vmop.Status.Phase).To(Equal(v1alpha2.VMOPPhaseFailed))

		// The stale copy marks the VM again and, once the VM shows the move, asks the peer again; its status
		// write then loses to the failure.
		_, _ = h.Handle(ctx, stale)

		Expect(showMove()).To(BeTrue())

		_, _ = h.Handle(ctx, stale)

		Expect(isGone(peer, &intvirtv1alpha1.InternalVirtualizationVirtualMachineReceiver{ObjectMeta: metav1.ObjectMeta{Namespace: ns, Name: "web"}})).To(BeFalse())

		handle()
		Expect(vmop.Status.Phase).To(Equal(v1alpha2.VMOPPhaseFailed))
		Expect(isGone(peer, &intvirtv1alpha1.InternalVirtualizationVirtualMachineReceiver{ObjectMeta: metav1.ObjectMeta{Namespace: ns, Name: "web"}})).To(BeTrue())

		var vm v1alpha2.VirtualMachine
		Expect(local.Get(ctx, types.NamespacedName{Namespace: ns, Name: "web"}, &vm)).To(Succeed())
		Expect(movedMark()).To(BeEmpty())

		var kvvm virtv1.VirtualMachine
		Expect(local.Get(ctx, types.NamespacedName{Namespace: ns, Name: "web"}, &kvvm)).To(Succeed())
		Expect(*kvvm.Spec.RunStrategy).To(Equal(virtv1.RunStrategyManual))

		// Once cleaned up, the operation does not ask the peer again.
		h.peers = fakePeers{err: errors.New("the peer is asked")}
		recorder.EventFunc = func(_ client.Object, _, _, msg string) { Fail(msg) }

		handle()
	})

	It("finishes the move when the peer reports the arrival while the migration here still runs", func() {
		handle()
		setReceiverPhase(intvirtv1alpha1.ReceiverReady, "")
		handle()
		// The synchronization broke at the end: the migration here never learns that the guest left.
		started := metav1.Now()
		setSourcePhase(virtv1.MigrationRunning, &virtv1.VirtualMachineInstanceMigrationState{StartTimestamp: &started})
		handle()
		Expect(vmop.Status.Phase).To(Equal(v1alpha2.VMOPPhaseInProgress))
		Expect(completed().Reason).To(Equal(string(vmopcondition.ReasonMigrationRunning)))

		setReceiverPhase(intvirtv1alpha1.ReceiverRunning, "")
		handle()
		Expect(vmop.Status.Phase).To(Equal(v1alpha2.VMOPPhaseCompleted))
		Expect(isGone(local, &v1alpha2.VirtualMachine{ObjectMeta: metav1.ObjectMeta{Namespace: ns, Name: "web"}})).To(BeTrue())
		Expect(isGone(local, &v1alpha2.VirtualDisk{ObjectMeta: metav1.ObjectMeta{Namespace: ns, Name: "web-root"}})).To(BeTrue())
	})

	It("finishes the move whose VM is being deleted after the guest arrived", func() {
		handle()
		setReceiverPhase(intvirtv1alpha1.ReceiverReady, "")
		handle()

		var r intvirtv1alpha1.InternalVirtualizationVirtualMachineReceiver
		Expect(peer.Get(ctx, types.NamespacedName{Namespace: ns, Name: "web"}, &r)).To(Succeed())
		r.Status.ReceiverStarted = true
		Expect(peer.Update(ctx, &r)).To(Succeed())
		// The move deletes the VM here, and the status of the operation is saved before it ends: the instance
		// goes with the VM, so the migration is gone with it.
		var vm v1alpha2.VirtualMachine
		Expect(local.Get(ctx, types.NamespacedName{Namespace: ns, Name: "web"}, &vm)).To(Succeed())
		vm.Finalizers = append(vm.Finalizers, "test/finalizer")
		Expect(local.Update(ctx, &vm)).To(Succeed())
		Expect(local.Delete(ctx, &vm)).To(Succeed())
		Expect(local.Delete(ctx, sourceMigration())).To(Succeed())

		handle()
		Expect(vmop.Status.Phase).To(Equal(v1alpha2.VMOPPhaseCompleted))
		Expect(completed().Message).To(Equal(`The virtual machine has moved to peer cluster "cluster-b".`))
		// The disks of the move go, and the request in the peer never stays behind.
		Expect(isGone(local, &v1alpha2.VirtualDisk{ObjectMeta: metav1.ObjectMeta{Namespace: ns, Name: "web-root"}})).To(BeTrue())
		Expect(isGone(peer, &r)).To(BeTrue())
	})

	It("ends the move of a VM that is deleted with its request gone", func() {
		handle()
		setReceiverPhase(intvirtv1alpha1.ReceiverReady, "")
		handle()
		// The request is gone with the synchronization: only the migration sent and the VM going with the move
		// are left. Failing here would overwrite a finished move with "it is not running".
		Expect(peer.Delete(ctx, &intvirtv1alpha1.InternalVirtualizationVirtualMachineReceiver{ObjectMeta: metav1.ObjectMeta{Namespace: ns, Name: "web"}})).To(Succeed())

		vmop.Status.Conditions = append(vmop.Status.Conditions, metav1.Condition{Type: string(vmopcondition.TypeSignalSent), Status: metav1.ConditionTrue})

		var vm v1alpha2.VirtualMachine
		Expect(local.Get(ctx, types.NamespacedName{Namespace: ns, Name: "web"}, &vm)).To(Succeed())
		vm.Finalizers = append(vm.Finalizers, "test/finalizer")
		Expect(local.Update(ctx, &vm)).To(Succeed())
		Expect(local.Delete(ctx, &vm)).To(Succeed())
		Expect(local.Delete(ctx, sourceMigration())).To(Succeed())

		handle()
		Expect(vmop.Status.Phase).To(Equal(v1alpha2.VMOPPhaseCompleted))
		Expect(vmop.Finalizers).To(BeEmpty())
	})

	It("fails the move of a VM deleted before the guest left", func() {
		handle()
		setReceiverPhase(intvirtv1alpha1.ReceiverReady, "")
		handle()

		var vm v1alpha2.VirtualMachine
		Expect(local.Get(ctx, types.NamespacedName{Namespace: ns, Name: "web"}, &vm)).To(Succeed())
		vm.Finalizers = append(vm.Finalizers, "test/finalizer")
		Expect(local.Update(ctx, &vm)).To(Succeed())
		Expect(local.Delete(ctx, &vm)).To(Succeed())
		Expect(local.Delete(ctx, sourceMigration())).To(Succeed())

		handle()
		Expect(vmop.Status.Phase).To(Equal(v1alpha2.VMOPPhaseFailed))
		Expect(completed().Message).To(ContainSubstring("it is being deleted in this cluster"))
	})

	It("fails at once when the peer gives up receiving while the migration copies", func() {
		handle()
		setReceiverPhase(intvirtv1alpha1.ReceiverReady, "")
		handle()

		started := metav1.Now()
		setSourcePhase(virtv1.MigrationRunning, &virtv1.VirtualMachineInstanceMigrationState{StartTimestamp: &started})
		handle()
		Expect(vmop.Status.Phase).To(Equal(v1alpha2.VMOPPhaseInProgress))
		// The node of the VM here is gone: the migration here never ends, the peer gives up.
		setReceiverPhase(intvirtv1alpha1.ReceiverFailed, "the receiving migration failed: Migration failed because target pod is down")
		handle()
		Expect(vmop.Status.Phase).To(Equal(v1alpha2.VMOPPhaseFailed))
		Expect(completed().Reason).To(Equal(string(vmopcondition.ReasonPeerRejected)))
		Expect(completed().Message).To(ContainSubstring("target pod is down"))
	})

	It("fails when KubeVirt refuses the migration", func() {
		handle()
		setReceiverPhase(intvirtv1alpha1.ReceiverReady, "")

		h.client = refusingMigrationClient{Client: local}

		handle()
		Expect(vmop.Status.Phase).To(Equal(v1alpha2.VMOPPhaseFailed))
		Expect(completed().Reason).To(Equal(string(vmopcondition.ReasonOperationFailed)))
		Expect(completed().Message).To(Equal(`The migration to the peer cluster was refused: admission webhook "migration-create-validator.kubevirt.io" denied the request: the guest has no invariant TSC. The virtual machine keeps running in this cluster; retry the move.`))
		Expect(isGone(peer, &intvirtv1alpha1.InternalVirtualizationVirtualMachineReceiver{ObjectMeta: metav1.ObjectMeta{Namespace: ns, Name: "web"}})).To(BeTrue())
	})

	It("waits for the peer to report why the migration failed", func() {
		handle()
		setReceiverPhase(intvirtv1alpha1.ReceiverReady, "")
		handle()

		failAt := func(at time.Time) {
			GinkgoHelper()

			mig := sourceMigration()
			mig.Status.Phase = virtv1.MigrationFailed
			mig.Status.PhaseTransitionTimestamps = []virtv1.VirtualMachineInstanceMigrationPhaseTransitionTimestamp{
				{Phase: virtv1.MigrationFailed, PhaseTransitionTimestamp: metav1.NewTime(at)},
			}
			Expect(local.Update(ctx, mig)).To(Succeed())
		}

		failAt(time.Now())
		handle()
		Expect(vmop.Status.Phase).To(Equal(v1alpha2.VMOPPhaseInProgress))
		Expect(completed().Reason).To(Equal(string(vmopcondition.ReasonWaitingForPeer)))

		setReceiverPhase(intvirtv1alpha1.ReceiverFailed, "the receiving node cannot prepare the virtual machine: no device; then the receiving migration failed: Target pod is down")
		handle()
		Expect(vmop.Status.Phase).To(Equal(v1alpha2.VMOPPhaseFailed))
		Expect(completed().Reason).To(Equal(string(vmopcondition.ReasonOperationFailed)))
		Expect(completed().Message).To(Equal(`The migration to the peer cluster failed: in peer cluster "cluster-b" the receiving node cannot prepare the virtual machine: no device; then the receiving migration failed: Target pod is down. The virtual machine keeps running in this cluster; retry the move.`))
	})

	It("fails with what it knows when the peer does not report why the migration failed in time", func() {
		handle()
		setReceiverPhase(intvirtv1alpha1.ReceiverReady, "the receiving node cannot prepare the virtual machine: no device")
		handle()

		mig := sourceMigration()
		mig.Status.Phase = virtv1.MigrationFailed
		mig.Status.PhaseTransitionTimestamps = []virtv1.VirtualMachineInstanceMigrationPhaseTransitionTimestamp{
			{Phase: virtv1.MigrationFailed, PhaseTransitionTimestamp: metav1.NewTime(time.Now().Add(-peerCauseTimeout - time.Second))},
		}
		Expect(local.Update(ctx, mig)).To(Succeed())
		handle()
		Expect(vmop.Status.Phase).To(Equal(v1alpha2.VMOPPhaseFailed))
		Expect(completed().Reason).To(Equal(string(vmopcondition.ReasonOperationFailed)))
		Expect(completed().Message).To(Equal(`The migration to the peer cluster failed: in peer cluster "cluster-b" the receiving node cannot prepare the virtual machine: no device. The virtual machine keeps running in this cluster; retry the move.`))
	})

	It("shows only the latest problem of the peer while the migration waits", func() {
		handle()
		setReceiverPhase(intvirtv1alpha1.ReceiverReady, "")
		handle()
		setReceiverPhase(intvirtv1alpha1.ReceiverReady, "the receiving pod cannot start: ErrImagePull: 401; then the receiving node cannot prepare the virtual machine: no device")

		mig := sourceMigration()
		mig.CreationTimestamp = metav1.Now()
		h2 := *h
		h2.migration = service.NewMigrationService(fakeGetter{Client: local, mig: mig}, featuregates.Default())
		_, err := h2.Handle(ctx, vmop)
		Expect(errors.Is(err, reconciler.ErrStopHandlerChain)).To(BeTrue())
		Expect(vmop.Status.Phase).To(Equal(v1alpha2.VMOPPhaseInProgress))
		Expect(completed().Message).To(Equal(`The migration to the peer cluster is waiting: in peer cluster "cluster-b" the receiving node cannot prepare the virtual machine: no device.`))
	})

	It("does not release the VM from a failed operation until the peer removes the request", func() {
		handle()
		setReceiverPhase(intvirtv1alpha1.ReceiverReady, "")
		handle()

		mig := sourceMigration()
		// The peer keeps its request until it removes what it created for it.
		var r intvirtv1alpha1.InternalVirtualizationVirtualMachineReceiver
		Expect(peer.Get(ctx, types.NamespacedName{Namespace: ns, Name: "web"}, &r)).To(Succeed())
		r.Finalizers = append(r.Finalizers, "internal.virtualization.deckhouse.io/vmr-cleanup")
		Expect(peer.Update(ctx, &r)).To(Succeed())

		stale := vmop.DeepCopy()

		setSourcePhase(virtv1.MigrationFailed, nil)
		// The peer removing the request wakes nothing here: the operation must come back by itself.
		Expect(handleResult().RequeueAfter).To(Equal(peerRequeue))
		// The receiving migration of the peer may still succeed: the VM stays locked here meanwhile.
		Expect(vmop.Status.Phase).To(Equal(v1alpha2.VMOPPhaseInProgress))
		Expect(completed().Reason).To(Equal(string(vmopcondition.ReasonWaitingForPeer)))
		Expect(completed().Message).To(ContainSubstring(`is released once peer cluster "cluster-b" removes the request`))
		Expect(isGone(local, mig)).To(BeFalse())

		var vm v1alpha2.VirtualMachine
		Expect(local.Get(ctx, types.NamespacedName{Namespace: ns, Name: "web"}, &vm)).To(Succeed())
		Expect(movedMark()).To(Equal("move"))
		Expect(peer.Get(ctx, types.NamespacedName{Namespace: ns, Name: "web"}, &r)).To(Succeed())
		Expect(r.DeletionTimestamp).NotTo(BeNil())
		Expect(r.Status.Phase).To(Equal(intvirtv1alpha1.ReceiverReady))

		// The copy from before the failure starts nothing meanwhile.
		_, _ = h.Handle(ctx, stale)

		Expect(isGone(local, mig)).To(BeFalse())
		Expect(local.Get(ctx, types.NamespacedName{Namespace: ns, Name: "web"}, &vm)).To(Succeed())
		Expect(movedMark()).To(Equal("move"))

		// The peer finishes: the VM is released, the operation fails and the migration goes.
		finishRequest()
		handle()
		Expect(vmop.Status.Phase).To(Equal(v1alpha2.VMOPPhaseFailed))
		Expect(completed().Reason).To(Equal(string(vmopcondition.ReasonOperationFailed)))
		Expect(isGone(local, mig)).To(BeTrue())
		Expect(local.Get(ctx, types.NamespacedName{Namespace: ns, Name: "web"}, &vm)).To(Succeed())
		Expect(movedMark()).To(BeEmpty())
	})

	It("releases the VM when the peer does not remove the request in time", func() {
		handle()
		setReceiverPhase(intvirtv1alpha1.ReceiverReady, "")
		handle()

		mig := sourceMigration()

		setSourcePhase(virtv1.MigrationFailed, nil)
		// The request has been going for longer than the wait allows.
		h.peers = fakePeers{client: terminatingPeer{Client: peer, since: time.Now().Add(-peerRequestTimeout - time.Minute)}}

		handle()
		Expect(vmop.Status.Phase).To(Equal(v1alpha2.VMOPPhaseFailed))
		Expect(completed().Reason).To(Equal(string(vmopcondition.ReasonOperationFailed)))
		Expect(isGone(local, mig)).To(BeTrue())

		var vm v1alpha2.VirtualMachine
		Expect(local.Get(ctx, types.NamespacedName{Namespace: ns, Name: "web"}, &vm)).To(Succeed())
		Expect(movedMark()).To(BeEmpty())
	})

	It("gives up on a request the peer has been removing for long", func() {
		handle()
		setReceiverPhase(intvirtv1alpha1.ReceiverReady, "")

		h.peers = fakePeers{client: terminatingPeer{Client: peer, since: time.Now().Add(-peerRequestTimeout - time.Minute)}}

		handle()
		Expect(vmop.Status.Phase).To(Equal(v1alpha2.VMOPPhaseFailed))
		Expect(completed().Reason).To(Equal(string(vmopcondition.ReasonPeerNotConfirmed)))
		Expect(completed().Message).To(ContainSubstring("did not remove the request to receive the virtual machine"))

		var vm v1alpha2.VirtualMachine
		Expect(local.Get(ctx, types.NamespacedName{Namespace: ns, Name: "web"}, &vm)).To(Succeed())
		Expect(movedMark()).To(BeEmpty())
	})

	It("finishes the move when the guest arrives while the request is being removed", func() {
		handle()
		setReceiverPhase(intvirtv1alpha1.ReceiverReady, "")
		handle()

		var r intvirtv1alpha1.InternalVirtualizationVirtualMachineReceiver
		Expect(peer.Get(ctx, types.NamespacedName{Namespace: ns, Name: "web"}, &r)).To(Succeed())
		// The fake client does not assign a UID: the receiving migration is named after it.
		r.UID = types.UID("receiver-uid")
		Expect(peer.Update(ctx, &r)).To(Succeed())

		received := &unstructured.Unstructured{Object: map[string]any{"status": map[string]any{"phase": string(virtv1.MigrationSucceeded)}}}
		received.SetGroupVersionKind(peerMigrationGVK)
		received.SetNamespace(ns)
		received.SetName("receiver-uid")

		setSourcePhase(virtv1.MigrationFailed, nil)
		// The peer finishes its receiving migration and removes the request right after it.
		h.peers = fakePeers{client: finishingPeer{Client: peer, migration: received}}

		handle()
		Expect(vmop.Status.Phase).To(Equal(v1alpha2.VMOPPhaseCompleted))
		Expect(isGone(local, &v1alpha2.VirtualMachine{ObjectMeta: metav1.ObjectMeta{Namespace: ns, Name: "web"}})).To(BeTrue())
		Expect(isGone(local, &v1alpha2.VirtualDisk{ObjectMeta: metav1.ObjectMeta{Namespace: ns, Name: "web-root"}})).To(BeTrue())
	})

	It("removes a migration a stale copy of a failed operation left", func() {
		handle()
		setReceiverPhase(intvirtv1alpha1.ReceiverReady, "")
		handle()

		mig := sourceMigration()
		left := mig.DeepCopy()

		setSourcePhase(virtv1.MigrationFailed, nil)
		handle()
		Expect(vmop.Status.Phase).To(Equal(v1alpha2.VMOPPhaseFailed))
		Expect(isGone(local, mig)).To(BeTrue())

		// A stale copy started the migration again after the failure was saved.
		left.ResourceVersion = ""
		Expect(local.Create(ctx, left)).To(Succeed())
		handle()
		Expect(isGone(local, left)).To(BeTrue())
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
		Expect(completed().Message).To(ContainSubstring("Ask an administrator"))
		Expect(completed().Message).To(ContainSubstring("if it has finished"))
		Expect(completed().Message).To(ContainSubstring("new VirtualMachineOperation"))
	})

	It("does not start the migration until the peer publishes the key of its synchronization controller", func() {
		handle()
		setReceiverPhase(intvirtv1alpha1.ReceiverReady, "")

		var r intvirtv1alpha1.InternalVirtualizationVirtualMachineReceiver
		Expect(peer.Get(ctx, types.NamespacedName{Namespace: ns, Name: "web"}, &r)).To(Succeed())
		r.Status.Synchronization.SyncPin = ""
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
		r.Status.Synchronization.SyncPin = ""
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
		Expect(movedMark()).To(BeEmpty())
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
		handle() // then the request in the peer
		Expect(isGone(peer, &intvirtv1alpha1.InternalVirtualizationVirtualMachineReceiver{ObjectMeta: metav1.ObjectMeta{Namespace: ns, Name: "web"}})).To(BeTrue())
		Expect(vmop.Finalizers).To(ContainElement(v1alpha2.FinalizerVMOPCleanup))
		handle() // the VM is released once the request is gone
		Expect(vmop.Finalizers).To(BeEmpty())
	})

	It("releases a cancelled VM only once the peer has removed the request", func() {
		handle()
		setReceiverPhase(intvirtv1alpha1.ReceiverReady, "")
		handle()

		var r intvirtv1alpha1.InternalVirtualizationVirtualMachineReceiver

		key := types.NamespacedName{Namespace: ns, Name: "web"}
		Expect(peer.Get(ctx, key, &r)).To(Succeed())
		r.Finalizers = append(r.Finalizers, "internal.virtualization.deckhouse.io/vmr-cleanup")
		Expect(peer.Update(ctx, &r)).To(Succeed())
		vmop.DeletionTimestamp = ptr.To(metav1.Now())

		handle() // the migration here goes
		handle() // the request is deleted, the peer keeps it while its receiving migration may still end
		handle()
		Expect(vmop.Finalizers).To(ContainElement(v1alpha2.FinalizerVMOPCleanup))

		var vm v1alpha2.VirtualMachine
		Expect(local.Get(ctx, key, &vm)).To(Succeed())
		Expect(movedMark()).To(Equal("move"))

		// The guest arrives meanwhile: the move is finished, not undone.
		Expect(peer.Get(ctx, key, &r)).To(Succeed())
		r.Status.ReceiverStarted = true
		Expect(peer.Update(ctx, &r)).To(Succeed())
		handle()
		Expect(vmop.Status.Phase).To(Equal(v1alpha2.VMOPPhaseTerminating))
		Expect(isGone(local, &v1alpha2.VirtualMachine{ObjectMeta: metav1.ObjectMeta{Namespace: ns, Name: "web"}})).To(BeTrue())
	})

	It("finishes a cancelled move whose guest arrived while the migration here still runs", func() {
		handle()
		setReceiverPhase(intvirtv1alpha1.ReceiverReady, "")
		handle()

		started := metav1.Now()
		setSourcePhase(virtv1.MigrationRunning, &virtv1.VirtualMachineInstanceMigrationState{StartTimestamp: &started})
		setReceiverPhase(intvirtv1alpha1.ReceiverRunning, "")

		vmop.DeletionTimestamp = ptr.To(metav1.Now())

		handle()
		// The operation itself is deleting; the move must finish, not roll back.
		Expect(vmop.Status.Phase).To(Equal(v1alpha2.VMOPPhaseTerminating))
		Expect(isGone(local, &v1alpha2.VirtualMachine{ObjectMeta: metav1.ObjectMeta{Namespace: ns, Name: "web"}})).To(BeTrue())
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
		Expect(movedMark()).To(BeEmpty())
	})

	// The network to the peer is down, not the credentials: the request fails with no answer from its API.
	It("releases a deleted operation when the network to the peer stays down", func() {
		handle()

		h.peers = fakePeers{client: unreachablePeer{Client: peer}}
		vmop.DeletionTimestamp = ptr.To(metav1.NewTime(time.Now().Add(-peerDeletionTimeout - time.Minute)))

		handle()

		Expect(vmop.Finalizers).To(BeEmpty())
		Expect(vmop.Status.Phase).To(Equal(v1alpha2.VMOPPhaseTerminating))
		Expect(movedMark()).To(BeEmpty())
	})

	It("releases a deleted operation with a failed migration when the network to the peer stays down", func() {
		handle()
		setReceiverPhase(intvirtv1alpha1.ReceiverReady, "")
		handle()
		setSourcePhase(virtv1.MigrationFailed, nil)

		h.peers = fakePeers{client: unreachablePeer{Client: peer}}
		vmop.DeletionTimestamp = ptr.To(metav1.NewTime(time.Now().Add(-peerDeletionTimeout - time.Minute)))

		handle()

		Expect(vmop.Finalizers).To(BeEmpty())
		Expect(movedMark()).To(BeEmpty())
	})

	It("keeps a deleted operation while the network to the peer is down for less than the timeout", func() {
		handle()

		h.peers = fakePeers{client: unreachablePeer{Client: peer}}
		vmop.DeletionTimestamp = ptr.To(metav1.Now())

		handle()

		Expect(vmop.Finalizers).To(ContainElement(v1alpha2.FinalizerVMOPCleanup))
		Expect(movedMark()).To(Equal("move"))
	})

	It("keeps a deleted operation while its migration may still switch the guest over", func() {
		handle()
		setReceiverPhase(intvirtv1alpha1.ReceiverReady, "")
		handle()
		setSourcePhase(virtv1.MigrationRunning, nil)

		mig := sourceMigration()
		mig.Finalizers = []string{"test/abort-pending"}
		Expect(local.Update(ctx, mig)).To(Succeed())

		h.peers = fakePeers{client: refusingPeer{Client: peer}}
		vmop.DeletionTimestamp = ptr.To(metav1.NewTime(time.Now().Add(-peerDeletionTimeout - time.Minute)))

		handle()

		Expect(vmop.Finalizers).To(ContainElement(v1alpha2.FinalizerVMOPCleanup))

		var vm v1alpha2.VirtualMachine
		Expect(local.Get(ctx, types.NamespacedName{Namespace: ns, Name: "web"}, &vm)).To(Succeed())
		Expect(movedMark()).To(Equal("move"))
	})

	It("hands a cancelled operation back stopped when the receiver has not run but the VM does not run here", func() {
		handle()
		setReceiverPhase(intvirtv1alpha1.ReceiverReady, "")

		var kvvmi virtv1.VirtualMachineInstance
		Expect(local.Get(ctx, types.NamespacedName{Namespace: ns, Name: "web"}, &kvvmi)).To(Succeed())
		Expect(local.Delete(ctx, &kvvmi)).To(Succeed())
		vmop.DeletionTimestamp = ptr.To(metav1.Now())

		handle() // the request in the peer goes
		handle() // the VM is released once it is gone

		Expect(isGone(peer, &intvirtv1alpha1.InternalVirtualizationVirtualMachineReceiver{ObjectMeta: metav1.ObjectMeta{Namespace: ns, Name: "web"}})).To(BeTrue())
		Expect(vmop.Finalizers).To(BeEmpty())

		var vm v1alpha2.VirtualMachine
		Expect(local.Get(ctx, types.NamespacedName{Namespace: ns, Name: "web"}, &vm)).To(Succeed())
		Expect(movedMark()).To(BeEmpty())
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
		Expect(movedMark()).To(BeEmpty())
		Expect(vm.Spec.RunPolicy).To(Equal(v1alpha2.ManualPolicy))
	})

	// The cache of this controller may lag behind the API server: the instance it shows running may have
	// failed with the handover, the guest running in the peer.
	It("hands a deleted operation back stopped when the cache still shows the VM running here", func() {
		h = NewPeerMigrationHandler(local, failedInstanceReader{Client: local}, fakePeers{client: peer}, h.migration, recorder)

		handle()

		h.peers = fakePeers{client: refusingPeer{Client: peer}}
		vmop.DeletionTimestamp = ptr.To(metav1.NewTime(time.Now().Add(-peerDeletionTimeout - time.Minute)))

		handle()

		Expect(vmop.Finalizers).To(BeEmpty())

		var vm v1alpha2.VirtualMachine
		Expect(local.Get(ctx, types.NamespacedName{Namespace: ns, Name: "web"}, &vm)).To(Succeed())
		Expect(movedMark()).To(BeEmpty())
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
		Expect(completed().Message).To(ContainSubstring(`in peer cluster "cluster-b" the receiving pod cannot be scheduled`))

		h2 = withCreated(time.Now().Add(-peerSyncTimeout - time.Minute))
		_, err = h2.Handle(ctx, vmop)
		Expect(errors.Is(err, reconciler.ErrStopHandlerChain)).To(BeTrue())
		Expect(completed().Reason).To(Equal(string(vmopcondition.ReasonPeerRejected)))
		Expect(completed().Message).To(ContainSubstring("0/4 nodes are available"))
	})

	It("fails at once when the peer gives up receiving before the migration starts", func() {
		handle()
		setReceiverPhase(intvirtv1alpha1.ReceiverReady, "")
		handle()
		Expect(vmop.Status.Phase).To(Equal(v1alpha2.VMOPPhaseInProgress))

		mig := sourceMigration()

		setReceiverPhase(intvirtv1alpha1.ReceiverFailed, "the receiving migration failed")
		handle()
		Expect(vmop.Status.Phase).To(Equal(v1alpha2.VMOPPhaseFailed))
		Expect(completed().Reason).To(Equal(string(vmopcondition.ReasonPeerRejected)))
		Expect(completed().Message).To(Equal(`The migration to the peer cluster failed: in peer cluster "cluster-b" the receiving migration failed. The virtual machine keeps running in this cluster; retry the move.`))
		Expect(isGone(local, mig)).To(BeTrue())

		var vm v1alpha2.VirtualMachine
		Expect(local.Get(ctx, types.NamespacedName{Namespace: ns, Name: "web"}, &vm)).To(Succeed())
		Expect(movedMark()).To(BeEmpty())
	})

	// The peer keeps a failed request under its finalizer while it removes what it created for it: the
	// failure must survive the request.
	It("fails with the cause of the peer once the peer has removed the failed request", func() {
		handle()
		setReceiverPhase(intvirtv1alpha1.ReceiverReady, "")
		handle()

		mig := sourceMigration()

		var r intvirtv1alpha1.InternalVirtualizationVirtualMachineReceiver
		Expect(peer.Get(ctx, types.NamespacedName{Namespace: ns, Name: "web"}, &r)).To(Succeed())
		r.Finalizers = []string{peering.FinalizerReceiverCleanup}
		Expect(peer.Update(ctx, &r)).To(Succeed())
		setReceiverPhase(intvirtv1alpha1.ReceiverFailed, `the receiving VirtualMachine "web" was deleted`)

		handle()
		Expect(vmop.Status.Phase).To(Equal(v1alpha2.VMOPPhaseInProgress))
		Expect(completed().Reason).To(Equal(string(vmopcondition.ReasonWaitingForPeer)))
		finishRequest()
		handle()
		Expect(vmop.Status.Phase).To(Equal(v1alpha2.VMOPPhaseFailed))
		Expect(completed().Reason).To(Equal(string(vmopcondition.ReasonPeerRejected)))
		Expect(completed().Message).To(Equal(`The migration to the peer cluster failed: in peer cluster "cluster-b" the receiving VirtualMachine "web" was deleted. The virtual machine keeps running in this cluster; retry the move.`))
		Expect(isGone(local, mig)).To(BeTrue())
		Expect(movedMark()).To(BeEmpty())
	})

	It("fails when the request of a started migration is gone from the peer", func() {
		handle()
		setReceiverPhase(intvirtv1alpha1.ReceiverReady, "")
		handle()

		mig := sourceMigration()

		Expect(peer.Delete(ctx, &intvirtv1alpha1.InternalVirtualizationVirtualMachineReceiver{ObjectMeta: metav1.ObjectMeta{Namespace: ns, Name: "web"}})).To(Succeed())
		handle()
		Expect(vmop.Status.Phase).To(Equal(v1alpha2.VMOPPhaseFailed))
		Expect(completed().Reason).To(Equal(string(vmopcondition.ReasonPeerRejected)))
		Expect(completed().Message).To(Equal(`Peer cluster "cluster-b" removed the request to receive the virtual machine. The virtual machine keeps running in this cluster; retry the move.`))
		Expect(isGone(local, mig)).To(BeTrue())
		Expect(movedMark()).To(BeEmpty())
	})

	// The receiving pod is deleted: the migration here fails too, and the peer reports the cause on its
	// request a moment later, then keeps the request under its finalizer while it cleans up.
	It("fails with the cause of the peer when the migration failed and the peer has removed the failed request", func() {
		handle()
		setReceiverPhase(intvirtv1alpha1.ReceiverReady, "")
		handle()

		var r intvirtv1alpha1.InternalVirtualizationVirtualMachineReceiver
		Expect(peer.Get(ctx, types.NamespacedName{Namespace: ns, Name: "web"}, &r)).To(Succeed())
		r.Finalizers = []string{peering.FinalizerReceiverCleanup}
		Expect(peer.Update(ctx, &r)).To(Succeed())
		setSourcePhase(virtv1.MigrationFailed, nil)
		setReceiverPhase(intvirtv1alpha1.ReceiverFailed, "the receiving virtual machine was shut down before the migration ended")

		handle()
		Expect(vmop.Status.Phase).To(Equal(v1alpha2.VMOPPhaseInProgress))
		Expect(completed().Reason).To(Equal(string(vmopcondition.ReasonWaitingForPeer)))
		finishRequest()
		handle()
		Expect(vmop.Status.Phase).To(Equal(v1alpha2.VMOPPhaseFailed))
		Expect(completed().Reason).To(Equal(string(vmopcondition.ReasonOperationFailed)))
		Expect(completed().Message).To(Equal(`The migration to the peer cluster failed: in peer cluster "cluster-b" the receiving virtual machine was shut down before the migration ended. The virtual machine keeps running in this cluster; retry the move.`))
		Expect(movedMark()).To(BeEmpty())
	})

	// The project here is deleted during the move: its pods go first, and the guest goes with the pod.
	It("does not tell that the VM keeps running when it does not run here", func() {
		handle()
		setReceiverPhase(intvirtv1alpha1.ReceiverReady, "")
		handle()

		var r intvirtv1alpha1.InternalVirtualizationVirtualMachineReceiver
		Expect(peer.Get(ctx, types.NamespacedName{Namespace: ns, Name: "web"}, &r)).To(Succeed())
		r.Finalizers = []string{peering.FinalizerReceiverCleanup}
		Expect(peer.Update(ctx, &r)).To(Succeed())
		setSourcePhase(virtv1.MigrationFailed, nil)
		setReceiverPhase(intvirtv1alpha1.ReceiverFailed, "the receiving virtual machine was shut down before the migration ended")
		handle()

		var kvvmi virtv1.VirtualMachineInstance
		Expect(local.Get(ctx, types.NamespacedName{Namespace: ns, Name: "web"}, &kvvmi)).To(Succeed())
		Expect(local.Delete(ctx, &kvvmi)).To(Succeed())
		finishRequest()

		handle()
		Expect(vmop.Status.Phase).To(Equal(v1alpha2.VMOPPhaseFailed))
		Expect(completed().Message).To(Equal(`The migration to the peer cluster failed: in peer cluster "cluster-b" the receiving virtual machine was shut down before the migration ended. The virtual machine is not running here and may run in peer cluster "cluster-b": it is left stopped with the Manual run policy, check it there before starting it here.`))

		var vm v1alpha2.VirtualMachine
		Expect(local.Get(ctx, types.NamespacedName{Namespace: ns, Name: "web"}, &vm)).To(Succeed())
		Expect(vm.Spec.RunPolicy).To(Equal(v1alpha2.ManualPolicy))
		Expect(movedMark()).To(BeEmpty())
	})

	DescribeTable("reports the actual VM state after releasing a failed migration", func(running bool, suffix string) {
		handle()

		if !running {
			var kvvmi virtv1.VirtualMachineInstance
			Expect(local.Get(ctx, types.NamespacedName{Namespace: ns, Name: "web"}, &kvvmi)).To(Succeed())
			Expect(local.Delete(ctx, &kvvmi)).To(Succeed())
		}

		const cause = "The peer rejected the migration for a newly worded reason."
		Expect(h.fail(ctx, vmop, peer, vmopcondition.ReasonPeerRejected, cause)).To(Succeed())
		Expect(vmop.Status.Phase).To(Equal(v1alpha2.VMOPPhaseFailed))
		Expect(completed().Message).To(Equal(cause + suffix))
		Expect(movedMark()).To(BeEmpty())
		Expect(recorder.EventCalls()).NotTo(BeEmpty())
		Expect(recorder.EventCalls()[len(recorder.EventCalls())-1].Message).To(Equal(completed().Message))
	},
		Entry("the source VM is running", true, " The virtual machine keeps running in this cluster; retry the move."),
		Entry("the source VM is no longer running", false, ` The virtual machine is not running here and may run in peer cluster "cluster-b": it is left stopped with the Manual run policy, check it there before starting it here.`),
	)

	It("does not tell that the VM keeps running when the peer removed the request and the VM does not run here", func() {
		handle()
		setReceiverPhase(intvirtv1alpha1.ReceiverReady, "")
		handle()

		var kvvmi virtv1.VirtualMachineInstance
		Expect(local.Get(ctx, types.NamespacedName{Namespace: ns, Name: "web"}, &kvvmi)).To(Succeed())
		Expect(local.Delete(ctx, &kvvmi)).To(Succeed())
		finishRequest()

		handle()
		Expect(vmop.Status.Phase).To(Equal(v1alpha2.VMOPPhaseFailed))
		Expect(completed().Message).To(Equal(`Peer cluster "cluster-b" removed the request to receive the virtual machine. The virtual machine is not running here and may run in peer cluster "cluster-b": it is left stopped with the Manual run policy, check it there before starting it here.`))
	})

	It("fails with the cause of KubeVirt when the migration failed and the request is gone", func() {
		handle()
		setReceiverPhase(intvirtv1alpha1.ReceiverReady, "")
		handle()
		setSourcePhase(virtv1.MigrationFailed, &virtv1.VirtualMachineInstanceMigrationState{FailureReason: "Target pod is down"})
		Expect(peer.Delete(ctx, &intvirtv1alpha1.InternalVirtualizationVirtualMachineReceiver{ObjectMeta: metav1.ObjectMeta{Namespace: ns, Name: "web"}})).To(Succeed())
		handle()
		Expect(vmop.Status.Phase).To(Equal(v1alpha2.VMOPPhaseFailed))
		Expect(completed().Reason).To(Equal(string(vmopcondition.ReasonOperationFailed)))
		Expect(completed().Message).To(Equal("The migration to the peer cluster failed: Target pod is down. The virtual machine keeps running in this cluster; retry the move."))
		Expect(movedMark()).To(BeEmpty())
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
		Expect(movedMark()).To(BeEmpty())
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
		// The arrival is the fact, so the request goes even though the receive mode is not lifted yet: left
		// behind, it keeps the name of the VM busy in the peer and the next move there fails with
		// "already receives". The peer keeps the guest and completes the receive on its own.
		Expect(isGone(peer, &r)).To(BeTrue())
	})

	It("fails the move when the peer refuses the credentials of this cluster", func() {
		h.peers = fakePeers{client: refusingPeer{Client: peer}}

		handle()
		Expect(vmop.Status.Phase).To(Equal(v1alpha2.VMOPPhaseFailed))
		Expect(completed().Reason).To(Equal(string(vmopcondition.ReasonPeerRejected)))
		Expect(completed().Message).To(ContainSubstring("refused the credentials"))

		var vm v1alpha2.VirtualMachine
		Expect(local.Get(ctx, types.NamespacedName{Namespace: ns, Name: "web"}, &vm)).To(Succeed())
		Expect(movedMark()).To(BeEmpty(), "the VM is not left locked")
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
		Expect(movedMark()).To(Equal("move"))
		Expect(isGone(peer, &intvirtv1alpha1.InternalVirtualizationVirtualMachineReceiver{ObjectMeta: metav1.ObjectMeta{Namespace: ns, Name: "web"}})).To(BeFalse())
		h.peers = fakePeers{client: peer}

		handle() // the request in the peer goes
		handle() // the VM is released once it is gone
		Expect(vmop.Finalizers).To(BeEmpty())
	})

	DescribeTable("cancels a scheduled source after the peer revokes access and releases it only after cancellation", func(deleting bool) {
		handle()
		setReceiverPhase(intvirtv1alpha1.ReceiverReady, "")
		handle()

		mig := sourceMigration()
		mig.Status.Phase = virtv1.MigrationScheduling
		mig.Finalizers = []string{"test/abort-pending"}
		Expect(local.Update(ctx, mig)).To(Succeed())

		h.peers = fakePeers{client: refusingPeer{Client: peer}}

		if deleting {
			vmop.DeletionTimestamp = ptr.To(metav1.NewTime(time.Now().Add(-peerDeletionTimeout - time.Minute)))
		}

		handle()
		Expect(sourceMigration().DeletionTimestamp).NotTo(BeNil())
		Expect(movedMark()).To(Equal("move"), "a pending abort is not proof that the guest cannot switch over")

		mig = sourceMigration()
		mig.Status.Phase = virtv1.MigrationFailed
		Expect(local.Update(ctx, mig)).To(Succeed())
		handle()
		Expect(movedMark()).To(BeEmpty())

		if deleting {
			Expect(vmop.Finalizers).NotTo(ContainElement(v1alpha2.FinalizerVMOPCleanup))
		} else {
			Expect(vmop.Status.Phase).To(Equal(v1alpha2.VMOPPhaseFailed))
			Expect(completed().Reason).To(Equal(string(vmopcondition.ReasonPeerRejected)))
		}
	}, Entry("operation running", false), Entry("operation deleting", true))

	DescribeTable("keeps a possibly created receiver when its state is unknown", func(peerError error, deleting, withMigration bool) {
		handle()

		if withMigration {
			setReceiverPhase(intvirtv1alpha1.ReceiverReady, "")
			handle()

			mig := sourceMigration()
			mig.Status.Phase = virtv1.MigrationRunning
			mig.Finalizers = []string{"test/abort-pending"}
			Expect(local.Update(ctx, mig)).To(Succeed())
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
		Expect(movedMark()).To(Equal("move"))
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
		Expect(movedMark()).To(BeEmpty())
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
		Expect(movedMark()).To(BeEmpty())
	})

	It("waits for an unreachable peer without its raw error", func() {
		handle()

		h.peers = fakePeers{client: unreachablePeer{Client: peer}}

		handle()
		Expect(vmop.Status.Phase).To(Equal(v1alpha2.VMOPPhaseInProgress))
		Expect(completed().Reason).To(Equal(string(vmopcondition.ReasonWaitingForPeer)))
		Expect(completed().Message).To(Equal(`Peer cluster "cluster-b" is unreachable. The migration continues when the connection is restored. The virtual machine keeps running in this cluster.`))
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
		Expect(movedMark()).To(BeEmpty())
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

// failedInstanceReader is the API server behind a lagging cache: the instance has failed there.
type failedInstanceReader struct {
	client.Client
}

func (r failedInstanceReader) Get(ctx context.Context, key client.ObjectKey, obj client.Object, opts ...client.GetOption) error {
	if err := r.Client.Get(ctx, key, obj, opts...); err != nil {
		return err
	}

	if kvvmi, ok := obj.(*virtv1.VirtualMachineInstance); ok {
		kvvmi.Status.Phase = virtv1.Failed
	}

	return nil
}

type refusingCreatePeer struct {
	client.Client
}

func (r refusingCreatePeer) Create(context.Context, client.Object, ...client.CreateOption) error {
	return k8serrors.NewForbidden(intvirtv1alpha1.SchemeGroupVersion.WithResource("internalvirtualizationvirtualmachinereceivers").GroupResource(), "web", errors.New("access denied"))
}

// refusingMigrationClient refuses to create a migration as the admission webhook of KubeVirt does.
type refusingMigrationClient struct {
	client.Client
}

func (r refusingMigrationClient) Create(ctx context.Context, obj client.Object, opts ...client.CreateOption) error {
	if _, ok := obj.(*virtv1.VirtualMachineInstanceMigration); ok {
		return k8serrors.NewBadRequest(`admission webhook "migration-create-validator.kubevirt.io" denied the request: the guest has no invariant TSC`)
	}

	return r.Client.Create(ctx, obj, opts...)
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

// terminatingPeer reports the request of the operation as one the peer has been removing for long.
type terminatingPeer struct {
	client.Client
	since time.Time
}

func (p terminatingPeer) Get(ctx context.Context, key client.ObjectKey, obj client.Object, opts ...client.GetOption) error {
	if err := p.Client.Get(ctx, key, obj, opts...); err != nil {
		return err
	}

	if r, ok := obj.(*intvirtv1alpha1.InternalVirtualizationVirtualMachineReceiver); ok && r.DeletionTimestamp == nil {
		r.DeletionTimestamp = &metav1.Time{Time: p.since}
	}

	return nil
}

// finishingPeer removes the request once its receiving migration has succeeded, as the peer does.
type finishingPeer struct {
	client.Client
	migration client.Object
}

func (p finishingPeer) Delete(ctx context.Context, obj client.Object, opts ...client.DeleteOption) error {
	err := p.Client.Delete(ctx, obj, opts...)
	if err != nil && !k8serrors.IsNotFound(err) {
		return err
	}

	if _, ok := obj.(*intvirtv1alpha1.InternalVirtualizationVirtualMachineReceiver); !ok {
		return nil
	}

	return p.Create(ctx, p.migration)
}

func (r unreachablePeer) Get(context.Context, client.ObjectKey, client.Object, ...client.GetOption) error {
	return errors.New("dial tcp 10.0.0.1:443: connect: connection refused")
}

var _ = Describe("peerFailureMessage", func() {
	It("tells the cause the peer reports instead of the one of KubeVirt", func() {
		src := &virtv1.VirtualMachineInstanceMigration{}
		Expect(peerFailureMessage(src, "peer-b", "")).To(Equal("The migration to the peer cluster failed."))
		Expect(peerFailureMessage(src, "peer-b", "the receiving migration failed: no device")).
			To(Equal(`The migration to the peer cluster failed: in peer cluster "peer-b" the receiving migration failed: no device.`))
		src.Status.MigrationState = &virtv1.VirtualMachineInstanceMigrationState{FailureReason: "virError(Code=1, Domain=7, Message='internal error: client socket is closed')"}
		Expect(peerFailureMessage(src, "peer-b", "")).To(Equal("The migration to the peer cluster failed: virError(Code=1, Domain=7, Message='internal error: client socket is closed')."))
		Expect(peerFailureMessage(src, "peer-b", `the receiving VirtualMachine "web" was deleted`)).
			To(Equal(`The migration to the peer cluster failed: in peer cluster "peer-b" the receiving VirtualMachine "web" was deleted.`))
		Expect(peerFailureMessage(src, "peer-b", "the receiving migration failed: Migration failed vmi shutdown during migration.")).
			To(Equal(`The migration to the peer cluster failed: in peer cluster "peer-b" the receiving migration failed: Migration failed vmi shutdown during migration.`))
	})
})

var _ = Describe("PeerMigrationHandler progress", func() {
	newVMOP := func() *v1alpha2.VirtualMachineOperation {
		return &v1alpha2.VirtualMachineOperation{ObjectMeta: metav1.ObjectMeta{Name: "move", Namespace: "app-x", UID: "7c4f"}}
	}

	It("shows the preparation of the peer until the copy starts", func() {
		vmop := newVMOP()
		h := NewPeerMigrationHandler(nil, nil, nil, nil, nil)
		h.progress(vmop, vmopcondition.ReasonWaitingForPeer, "Waiting for the peer.")
		Expect(vmop.Status.Progress).To(Equal(migrationprogress.FormatPercent(progressTargetPreparing)))

		// A later stage does not roll the value back.
		h.setProgress(vmop, progressWaitingForSyncSlot)
		Expect(vmop.Status.Progress).To(Equal(migrationprogress.FormatPercent(progressTargetPreparing)))
	})

	It("reports the transfer counters of the outgoing migration", func() {
		vmop := newVMOP()
		vmop.Status.Progress = migrationprogress.FormatPercent(progressTargetPreparing)
		h := NewPeerMigrationHandler(nil, nil, nil, nil, nil)
		started := metav1.NewTime(time.Now().Add(-time.Minute))
		src := &virtv1.VirtualMachineInstanceMigration{Status: virtv1.VirtualMachineInstanceMigrationStatus{
			Phase: virtv1.MigrationRunning,
			MigrationState: &virtv1.VirtualMachineInstanceMigrationState{
				StartTimestamp: &started,
				TransferStatus: &virtv1.VirtualMachineInstanceMigrationTransferStatus{
					DataTotalBytes:     ptr.To(uint64(4 * 1024 * 1024 * 1024)),
					DataProcessedBytes: ptr.To(uint64(3 * 1024 * 1024 * 1024)),
					DataRemainingBytes: ptr.To(uint64(1 * 1024 * 1024 * 1024)),
				},
			},
		}}

		h.syncProgress(vmop, src)
		progress := migrationprogress.ParsePercent(vmop.Status.Progress)
		Expect(progress).To(BeNumerically(">", progressTargetPreparing), "the copy stands ahead of the preparation")
		Expect(progress).To(BeNumerically("<", 100), "the move is not over while the copy runs")
	})

	It("reports the move as complete and keeps the last value of a failure", func() {
		h := NewPeerMigrationHandler(nil, nil, nil, nil, nil)

		done := newVMOP()
		done.Status.Progress = migrationprogress.FormatPercent(42)
		h.complete(done, v1alpha2.VMOPPhaseCompleted, vmopcondition.ReasonMigrationCompleted, "")
		Expect(done.Status.Progress).To(Equal(migrationprogress.FormatPercent(100)))

		failed := newVMOP()
		failed.Status.Progress = migrationprogress.FormatPercent(42)
		h.complete(failed, v1alpha2.VMOPPhaseFailed, vmopcondition.ReasonPeerRejected, "refused")
		Expect(failed.Status.Progress).To(Equal(migrationprogress.FormatPercent(42)))
	})
})
