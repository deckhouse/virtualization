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
	"errors"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/ptr"
	virtv1 "kubevirt.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"

	"github.com/deckhouse/virtualization-controller/pkg/common/annotations"
	"github.com/deckhouse/virtualization-controller/pkg/common/testutil"
	"github.com/deckhouse/virtualization-controller/pkg/controller/conditions"
	"github.com/deckhouse/virtualization-controller/pkg/controller/netmanager"
	"github.com/deckhouse/virtualization-controller/pkg/controller/peering"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/vmcondition"
	intvirtv1alpha1 "github.com/deckhouse/virtualization/api/internalvirtualization/v1alpha1"
)

type denyVMIPCreateClient struct{ client.Client }

func (c denyVMIPCreateClient) Create(ctx context.Context, obj client.Object, opts ...client.CreateOption) error {
	if _, ok := obj.(*v1alpha2.VirtualMachineIPAddress); ok {
		return errors.New(`admission webhook "vmip.validate" denied the request: the IP address is taken`)
	}

	return c.Client.Create(ctx, obj, opts...)
}

var _ = Describe("checkReceiverCapacity", func() {
	source := &virtv1.VirtualMachineInstance{Status: virtv1.VirtualMachineInstanceStatus{
		VolumeStatus: []virtv1.VolumeStatus{{Name: "vd-root", Size: 1144762368}},
	}}
	here := func(capacity string) map[string]*v1alpha2.VirtualDisk {
		return map[string]*v1alpha2.VirtualDisk{"root": {Status: v1alpha2.VirtualDiskStatus{Capacity: capacity}}}
	}

	It("refuses a disk whose capacity is below the size of the source before the copy", func() {
		Expect(checkReceiverCapacity(source, here("1Gi"))).To(Equal(`the disk "root" in the receiving cluster has a capacity of 1Gi, smaller than the disk in the source cluster (1144762368 bytes)`))
	})
	It("accepts a disk that can hold the source", func() {
		Expect(checkReceiverCapacity(source, here("2Gi"))).To(BeEmpty())
		Expect(checkReceiverCapacity(source, here(""))).To(BeEmpty())
	})
})

var _ = Describe("checkReceiverDisks", func() {
	source := &virtv1.VirtualMachineInstance{Status: virtv1.VirtualMachineInstanceStatus{
		VolumeStatus: []virtv1.VolumeStatus{{Name: "vd-root", Size: 1144762368}},
	}}
	here := func(size int64) *virtv1.VirtualMachineInstance {
		return &virtv1.VirtualMachineInstance{Status: virtv1.VirtualMachineInstanceStatus{
			VolumeStatus: []virtv1.VolumeStatus{{Name: "vd-root", Size: size}},
		}}
	}

	It("refuses a block device smaller than the one of the source", func() {
		Expect(checkReceiverDisks(source, here(1144717312))).To(Equal(`the disk "vd-root" in the receiving cluster is 1144717312 bytes, smaller than the disk in the source cluster (1144762368 bytes)`))
	})
	It("accepts a block device not smaller than the source", func() {
		Expect(checkReceiverDisks(source, here(1144762368))).To(BeEmpty())
		Expect(checkReceiverDisks(source, here(1144762368+4096))).To(BeEmpty())
	})
	It("waits for the sizes of the receiving pod", func() {
		Expect(checkReceiverDisks(source, nil)).To(BeEmpty())
		Expect(checkReceiverDisks(source, here(0))).To(BeEmpty())
	})
})

var _ = Describe("isReceiving", func() {
	It("is true until the VM runs here, fails or the request goes", func() {
		r := &intvirtv1alpha1.InternalVirtualizationVirtualMachineReceiver{}
		Expect(isReceiving(r)).To(BeTrue())
		r.Status.Phase = intvirtv1alpha1.ReceiverReady
		Expect(isReceiving(r)).To(BeTrue())
		r.Status.Phase = intvirtv1alpha1.ReceiverRunning
		Expect(isReceiving(r)).To(BeFalse())
		r.Status.Phase = intvirtv1alpha1.ReceiverFailed
		Expect(isReceiving(r)).To(BeFalse())
		Expect(isReceiving(nil)).To(BeFalse())
	})
})

var _ = Describe("loadMigrationReceiver", func() {
	newClient := func(objs ...client.Object) client.Client {
		scheme := runtime.NewScheme()
		Expect(intvirtv1alpha1.AddToScheme(scheme)).To(Succeed())

		return fake.NewClientBuilder().WithScheme(scheme).WithObjects(objs...).Build()
	}
	request := func(st intvirtv1alpha1.VirtualMachineReceiverStatus) *intvirtv1alpha1.InternalVirtualizationVirtualMachineReceiver {
		return &intvirtv1alpha1.InternalVirtualizationVirtualMachineReceiver{
			ObjectMeta: metav1.ObjectMeta{Namespace: "app-x", Name: "web-01", UID: "uid-1"},
			Status:     st,
		}
	}
	vm := func(owner types.UID) *v1alpha2.VirtualMachine {
		v := &v1alpha2.VirtualMachine{ObjectMeta: metav1.ObjectMeta{Namespace: "app-x", Name: "web-01", UID: "vm-1"}}
		if owner != "" {
			v.OwnerReferences = []metav1.OwnerReference{{Kind: intvirtv1alpha1.InternalVirtualizationVirtualMachineReceiverKind, Name: "web-01", UID: owner}}
		}

		return v
	}
	load := func(cl client.Client, v *v1alpha2.VirtualMachine) (*intvirtv1alpha1.InternalVirtualizationVirtualMachineReceiver, bool) {
		r, decided, err := loadMigrationReceiver(context.Background(), cl, v)
		Expect(err).NotTo(HaveOccurred())

		return r, decided
	}

	It("returns the request the VM refers to, before the request records it", func() {
		r, decided := load(newClient(request(intvirtv1alpha1.VirtualMachineReceiverStatus{})), vm("uid-1"))
		Expect(r).NotTo(BeNil())
		Expect(decided).To(BeTrue())
	})
	It("returns the request that recorded the VM once the reference is gone", func() {
		r, decided := load(newClient(request(intvirtv1alpha1.VirtualMachineReceiverStatus{VirtualMachineUID: "vm-1"})), vm(""))
		Expect(r).NotTo(BeNil())
		Expect(decided).To(BeTrue())
	})
	// A VM of the user of the same name as a request waits for nothing: it has no reference to the request.
	It("decides at once that a VM without the reference is a regular one", func() {
		r, decided := load(newClient(request(intvirtv1alpha1.VirtualMachineReceiverStatus{})), vm(""))
		Expect(r).To(BeNil())
		Expect(decided).To(BeTrue())
		r, decided = load(newClient(), vm(""))
		Expect(r).To(BeNil())
		Expect(decided).To(BeTrue())
	})
	It("leaves undecided a VM that refers to a request not seen yet", func() {
		r, decided := load(newClient(), vm("uid-1"))
		Expect(r).To(BeNil())
		Expect(decided).To(BeFalse())
	})
	It("leaves undecided a VM that refers to an earlier request of the same name", func() {
		r, decided := load(newClient(request(intvirtv1alpha1.VirtualMachineReceiverStatus{})), vm("uid-0"))
		Expect(r).To(BeNil())
		Expect(decided).To(BeFalse())
	})
})

var _ = Describe("isAdmissionDenial", func() {
	It("hands KubeVirt the sizes of the source images", func() {
		kvvm := &virtv1.VirtualMachine{Spec: virtv1.VirtualMachineSpec{Template: &virtv1.VirtualMachineInstanceTemplateSpec{}}}
		Expect(setSourceVolumeSizes(kvvm, &virtv1.VirtualMachineInstance{Status: virtv1.VirtualMachineInstanceStatus{
			VolumeStatus: []virtv1.VolumeStatus{{Name: "vd-root", Size: 2028994560}, {Name: "cloudinit"}},
		}})).To(Succeed())
		Expect(kvvm.Spec.Template.ObjectMeta.Annotations).To(HaveKeyWithValue(annotations.AnnSourceVolumeSizes, `{"vd-root":2028994560}`))
	})

	It("tells a webhook denial from other errors", func() {
		Expect(isAdmissionDenial(errors.New(`admission webhook "vmip.validate" denied the request: the IP address is taken`))).To(BeTrue())
		Expect(isAdmissionDenial(errors.New("connection refused"))).To(BeFalse())
		Expect(isAdmissionDenial(k8serrors.NewForbidden(schema.GroupResource{Resource: "virtualmachineipaddresses"}, "web", errors.New("RBAC: access denied")))).To(BeFalse())
		Expect(isAdmissionDenial(nil)).To(BeFalse())
	})
})

var _ = Describe("receivingPodMessage", func() {
	It("tells why the receiving pod does not start", func() {
		kvvm := &virtv1.VirtualMachine{}
		Expect(receivingPodMessage(kvvm)).To(BeEmpty())
		kvvm.Status.Conditions = []virtv1.VirtualMachineCondition{{
			Type: virtv1.VirtualMachineConditionType(corev1.PodScheduled), Status: corev1.ConditionFalse,
			Reason: "Unschedulable", Message: "0/4 nodes are available: 2 node(s) didn't match Pod's node affinity/selector",
		}}
		Expect(receivingPodMessage(kvvm)).To(Equal("the receiving pod cannot be scheduled: Unschedulable: 0/4 nodes are available: 2 node(s) didn't match Pod's node affinity/selector"))
	})

	It("tells why the scheduled receiving pod does not start", func() {
		kvvm := &virtv1.VirtualMachine{}
		kvvm.Status.Conditions = []virtv1.VirtualMachineCondition{
			{Type: virtv1.VirtualMachineReady, Status: corev1.ConditionFalse, Reason: "GuestNotRunning"},
			{Type: virtv1.VirtualMachineConditionType(corev1.PodScheduled), Status: corev1.ConditionTrue},
			{Type: conditions.VirtualMachineSynchronized, Status: corev1.ConditionFalse, Reason: "ErrImagePull", Message: "failed to pull image: 401 Unauthorized"},
		}
		Expect(receivingPodMessage(kvvm)).To(Equal("the receiving pod cannot start: ErrImagePull: failed to pull image: 401 Unauthorized"))
	})

	It("tells nothing while the receiving pod is fine", func() {
		kvvm := &virtv1.VirtualMachine{}
		kvvm.Status.Conditions = []virtv1.VirtualMachineCondition{
			{Type: virtv1.VirtualMachineConditionType(corev1.PodScheduled), Status: corev1.ConditionTrue},
			{Type: conditions.VirtualMachineSynchronized, Status: corev1.ConditionTrue},
		}
		Expect(receivingPodMessage(kvvm)).To(BeEmpty())
	})
})

var _ = Describe("receivingTargetFailures", func() {
	kvvmi := &virtv1.VirtualMachineInstance{ObjectMeta: metav1.ObjectMeta{Name: "web", UID: "vmi-uid"}}
	event := func(uid types.UID, typ, reason, msg string, first time.Time) corev1.Event {
		return corev1.Event{
			InvolvedObject: corev1.ObjectReference{Name: "web", UID: uid},
			Type:           typ, Reason: reason, Message: msg,
			FirstTimestamp: metav1.NewTime(first), LastTimestamp: metav1.NewTime(first.Add(time.Hour)),
		}
	}
	now := time.Now()

	const prefix = "the receiving node cannot prepare the virtual machine: SyncFailed: "

	It("tells the failures of virt-handler to prepare the receiving VMI in the order they first occurred", func() {
		events := []corev1.Event{
			event("vmi-uid", corev1.EventTypeWarning, "SyncFailed", "failed to detect VMI pod: connection refused", now),
			event("vmi-uid", corev1.EventTypeWarning, "SyncFailed", "failed to mount hotplug volumes: no device in root", now.Add(-time.Minute)),
			event("vmi-uid", corev1.EventTypeWarning, "VMIUpdateStatusFailedForDRADevices", "the server rejected our request", now.Add(-2*time.Minute)),
		}
		Expect(receivingTargetFailures(kvvmi, events)).To(Equal([]string{
			prefix + "failed to mount hotplug volumes: no device in root",
			prefix + "failed to detect VMI pod: connection refused",
		}))
	})

	It("ignores the events of another VMI with the same name and normal events", func() {
		events := []corev1.Event{
			event("old-uid", corev1.EventTypeWarning, "SyncFailed", "failed to mount", now),
			event("vmi-uid", corev1.EventTypeNormal, "SyncFailed", "not a failure", now),
		}
		Expect(receivingTargetFailures(kvvmi, events)).To(BeEmpty())
	})
})

var _ = Describe("receivingQuotaExceeded", func() {
	kvvmi := &virtv1.VirtualMachineInstance{ObjectMeta: metav1.ObjectMeta{Name: "web", Namespace: "pt-dr-test", UID: "vmi-uid"}}
	event := func(uid types.UID, typ, reason, msg string) corev1.Event {
		return corev1.Event{InvolvedObject: corev1.ObjectReference{Name: "web", UID: uid}, Type: typ, Reason: reason, Message: msg}
	}

	const refused = `Error creating pod: pods "virt-launcher-web-x7k2p" is forbidden: exceeded quota: c22-quota, requested: requests.memory=640Mi, used: requests.memory=1Gi, limited: requests.memory=64Mi`

	It("names the quota that refuses the receiving pod", func() {
		Expect(receivingQuotaExceeded(kvvmi, []corev1.Event{event("vmi-uid", corev1.EventTypeWarning, "FailedCreate", refused)})).To(Equal(
			`the ResourceQuota "c22-quota" of namespace "pt-dr-test" does not allow the virtual machine (requested: requests.memory=640Mi, used: requests.memory=1Gi, limited: requests.memory=64Mi): free resources there or raise the quota`,
		))
	})

	It("keeps the resources a quota lists together", func() {
		msg := `Error creating pod: pods "p" is forbidden: exceeded quota: q, requested: limits.cpu=1,requests.memory=640Mi, used: limits.cpu=3,requests.memory=1Gi, limited: limits.cpu=3,requests.memory=1Gi`
		Expect(receivingQuotaExceeded(kvvmi, []corev1.Event{event("vmi-uid", corev1.EventTypeWarning, "FailedCreate", msg)})).To(Equal(
			`the ResourceQuota "q" of namespace "pt-dr-test" does not allow the virtual machine (requested: limits.cpu=1,requests.memory=640Mi, used: limits.cpu=3,requests.memory=1Gi, limited: limits.cpu=3,requests.memory=1Gi): free resources there or raise the quota`,
		))
	})

	It("ignores other failures, normal events and the events of another VMI with the same name", func() {
		Expect(receivingQuotaExceeded(kvvmi, []corev1.Event{
			event("old-uid", corev1.EventTypeWarning, "FailedCreate", refused),
			event("vmi-uid", corev1.EventTypeWarning, "FailedCreate", `Error creating pod: pods "p" is forbidden: violates PodSecurity`),
			event("vmi-uid", corev1.EventTypeWarning, "FailedCreate", "exceeded quota: q"),
			event("vmi-uid", corev1.EventTypeWarning, "SyncFailed", refused),
			event("vmi-uid", corev1.EventTypeNormal, "FailedCreate", refused),
		})).To(BeEmpty())
	})
})

var _ = Describe("receivingProblemsMessage", func() {
	It("adds the problems seen now to those seen before", func() {
		Expect(receivingProblemsMessage("", []string{"", "no device"})).To(Equal("no device"))
		Expect(receivingProblemsMessage("no device", []string{"", "no device", "connection refused"})).To(Equal("no device; then connection refused"))
	})

	It("forgets the problems once none is seen", func() {
		Expect(receivingProblemsMessage("no device", []string{""})).To(BeEmpty())
	})
})

var _ = Describe("receivingMigrationFailedMessage", func() {
	received := &virtv1.VirtualMachineInstanceMigration{ObjectMeta: metav1.ObjectMeta{UID: "vmim-uid"}}

	It("ends the chain of problems with the cause KubeVirt found", func() {
		r := &intvirtv1alpha1.InternalVirtualizationVirtualMachineReceiver{}
		Expect(receivingMigrationFailedMessage(r, received, nil)).To(Equal("the receiving migration failed"))

		r.Status.Message = "the receiving node cannot prepare the virtual machine: SyncFailed: no device"
		withCause := received.DeepCopy()
		withCause.Status.MigrationState = &virtv1.VirtualMachineInstanceMigrationState{FailureReason: "Target pod is down"}
		Expect(receivingMigrationFailedMessage(r, withCause, nil)).To(Equal(
			"the receiving node cannot prepare the virtual machine: SyncFailed: no device; then the receiving migration failed: Target pod is down",
		))
	})

	It("prefers the Failed condition of the migration, set together with the failure", func() {
		r := &intvirtv1alpha1.InternalVirtualizationVirtualMachineReceiver{}
		failed := received.DeepCopy()
		failed.Status.Conditions = []virtv1.VirtualMachineInstanceMigrationCondition{{
			Type: virtv1.VirtualMachineInstanceMigrationFailed, Status: corev1.ConditionTrue,
			Reason: "TargetPodWasDeletedBecauseTimeoutExceeded", Message: "Migration failed because target pod app-x/d8v-vm-web-abc was deleted",
		}}
		failed.Status.MigrationState = &virtv1.VirtualMachineInstanceMigrationState{FailureReason: "Target pod is down"}
		Expect(receivingMigrationFailedMessage(r, failed, nil)).To(Equal(
			"the receiving migration failed: Migration failed because target pod app-x/d8v-vm-web-abc was deleted",
		))
	})

	It("tells in words that the receiving VM was shut down", func() {
		for _, reason := range []string{
			virtv1.VirtualMachineInstanceMigrationFailedReasonVMIIsShutdown,
			virtv1.VirtualMachineInstanceMigrationFailedReasonTargetPodShutdownDuringMigration,
			virtv1.VirtualMachineInstanceMigrationFailedReasonTargetPodDisappearedDuringMigration,
		} {
			r := &intvirtv1alpha1.InternalVirtualizationVirtualMachineReceiver{}
			failed := received.DeepCopy()
			failed.Status.Conditions = []virtv1.VirtualMachineInstanceMigrationCondition{{
				Type: virtv1.VirtualMachineInstanceMigrationFailed, Status: corev1.ConditionTrue,
				Reason: reason, Message: "Migration failed vmi shutdown during migration.",
			}}
			Expect(receivingMigrationFailedMessage(r, failed, nil)).To(Equal(
				"the receiving virtual machine was shut down before the migration ended",
			), reason)
		}
	})

	It("reads the cause from the VMI when the migration has not copied it", func() {
		r := &intvirtv1alpha1.InternalVirtualizationVirtualMachineReceiver{}
		kvvmi := &virtv1.VirtualMachineInstance{Status: virtv1.VirtualMachineInstanceStatus{
			MigrationState: &virtv1.VirtualMachineInstanceMigrationState{MigrationUID: "vmim-uid", FailureReason: "Target pod is down"},
		}}
		Expect(receivingMigrationFailedMessage(r, received, kvvmi)).To(Equal("the receiving migration failed: Target pod is down"))
		// The state of another migration of the VMI is not this one's cause.
		kvvmi.Status.MigrationState.MigrationUID = "other"
		Expect(receivingMigrationFailedMessage(r, received, kvvmi)).To(Equal("the receiving migration failed"))
	})

	It("keeps the message of a request that has already failed", func() {
		r := &intvirtv1alpha1.InternalVirtualizationVirtualMachineReceiver{}
		r.Status.Phase = intvirtv1alpha1.ReceiverFailed
		r.Status.Message = "the receiving migration failed: no device"
		Expect(receivingMigrationFailedMessage(r, received, nil)).To(Equal("the receiving migration failed: no device"))
	})
})

var _ = Describe("syncMigrationReceiver", func() {
	It("finishes a deleting receiver when the migration has succeeded", func() {
		const ns = "app-x"

		vm := &v1alpha2.VirtualMachine{
			ObjectMeta: metav1.ObjectMeta{Name: "web", Namespace: ns},
			Spec:       v1alpha2.VirtualMachineSpec{RunPolicy: v1alpha2.ManualPolicy},
		}
		received := &virtv1.VirtualMachineInstanceMigration{
			ObjectMeta: metav1.ObjectMeta{Name: "receiver-uid", Namespace: ns},
			Spec: virtv1.VirtualMachineInstanceMigrationSpec{
				VMIName: "web",
				Receive: &virtv1.VirtualMachineInstanceMigrationTarget{MigrationID: "receiver-uid"},
			},
			Status: virtv1.VirtualMachineInstanceMigrationStatus{Phase: virtv1.MigrationSucceeded},
		}
		kvvm := newEmptyKVVM("web", ns)
		kvvm.UID = "kvvm-uid"
		kvvm.OwnerReferences = []metav1.OwnerReference{*metav1.NewControllerRef(vm, v1alpha2.SchemeGroupVersion.WithKind(v1alpha2.VirtualMachineKind))}
		kvvm.Spec.RunStrategy = ptr.To(virtv1.RunStrategyWaitAsReceiver)
		receiver := &intvirtv1alpha1.InternalVirtualizationVirtualMachineReceiver{
			ObjectMeta: metav1.ObjectMeta{Name: "web", Namespace: ns, UID: "receiver-uid", Finalizers: []string{peering.FinalizerReceiverCleanup}, DeletionTimestamp: ptr.To(metav1.Now())},
			Status:     intvirtv1alpha1.VirtualMachineReceiverStatus{InternalVirtualMachineUID: "kvvm-uid"},
		}
		fakeClient, _, vmState := setupEnvironment(vm, received, kvvm, receiver)
		h := &SyncKvvmHandler{client: fakeClient}

		done, err := h.syncMigrationReceiver(context.Background(), vmState, receiver)
		Expect(err).NotTo(HaveOccurred())
		Expect(done).To(BeTrue())

		var got virtv1.VirtualMachine
		Expect(fakeClient.Get(context.Background(), types.NamespacedName{Namespace: ns, Name: "web"}, &got)).To(Succeed())
		Expect(got.Annotations).NotTo(HaveKey("internal.virtualization.deckhouse.io/migration-receiver"))
		Expect(*got.Spec.RunStrategy).To(Equal(virtv1.RunStrategyManual))
	})

	Describe("releases the received objects from the request", func() {
		const ns = "app-x"

		var (
			vm       *v1alpha2.VirtualMachine
			vd       *v1alpha2.VirtualDisk
			received *virtv1.VirtualMachineInstanceMigration
			kvvm     *virtv1.VirtualMachine
			receiver *intvirtv1alpha1.InternalVirtualizationVirtualMachineReceiver
		)

		BeforeEach(func() {
			receiver = &intvirtv1alpha1.InternalVirtualizationVirtualMachineReceiver{
				ObjectMeta: metav1.ObjectMeta{Name: "web", Namespace: ns, UID: "receiver-uid"},
				Status:     intvirtv1alpha1.VirtualMachineReceiverStatus{Phase: intvirtv1alpha1.ReceiverReady},
			}
			ref := metav1.OwnerReference{
				APIVersion: intvirtv1alpha1.SchemeGroupVersion.String(), Kind: intvirtv1alpha1.InternalVirtualizationVirtualMachineReceiverKind,
				Name: "web", UID: "receiver-uid", Controller: ptr.To(true),
			}
			vm = &v1alpha2.VirtualMachine{
				ObjectMeta: metav1.ObjectMeta{Name: "web", Namespace: ns, UID: "vm-uid", OwnerReferences: []metav1.OwnerReference{ref}},
				Spec: v1alpha2.VirtualMachineSpec{
					RunPolicy:       v1alpha2.ManualPolicy,
					BlockDeviceRefs: []v1alpha2.BlockDeviceSpecRef{{Kind: v1alpha2.DiskDevice, Name: "web-root"}},
				},
			}
			vd = &v1alpha2.VirtualDisk{ObjectMeta: metav1.ObjectMeta{Name: "web-root", Namespace: ns, UID: "vd-uid", OwnerReferences: []metav1.OwnerReference{ref}}}
			received = &virtv1.VirtualMachineInstanceMigration{
				ObjectMeta: metav1.ObjectMeta{Name: "receiver-uid", Namespace: ns},
				Spec: virtv1.VirtualMachineInstanceMigrationSpec{
					VMIName: "web",
					Receive: &virtv1.VirtualMachineInstanceMigrationTarget{MigrationID: "receiver-uid"},
				},
				Status: virtv1.VirtualMachineInstanceMigrationStatus{Phase: virtv1.MigrationSucceeded},
			}
			kvvm = newEmptyKVVM("web", ns)
			kvvm.UID = "kvvm-uid"
			kvvm.OwnerReferences = []metav1.OwnerReference{*metav1.NewControllerRef(vm, v1alpha2.SchemeGroupVersion.WithKind(v1alpha2.VirtualMachineKind))}
			kvvm.Spec.RunStrategy = ptr.To(virtv1.RunStrategyWaitAsReceiver)
		})

		DescribeTable("leaves a finalized receive under ordinary power management", func(replaced bool) {
			receiver.Status.Phase = intvirtv1alpha1.ReceiverRunning
			receiver.Status.InternalVirtualMachineUID = kvvm.UID
			vm.Spec.RunPolicy = v1alpha2.AlwaysOnUnlessStoppedManually

			kvvm.Spec.RunStrategy = ptr.To(virtv1.RunStrategyManual)
			if replaced {
				kvvm.UID = "replacement-kvvm-uid"
			}

			cl, _, vmState := setupEnvironment(vm, vd, received, kvvm, receiver)
			done, err := (&SyncKvvmHandler{client: cl}).syncMigrationReceiver(context.Background(), vmState, receiver)
			Expect(err).NotTo(HaveOccurred())
			Expect(done).To(BeTrue())

			var actual virtv1.VirtualMachine
			Expect(cl.Get(context.Background(), client.ObjectKeyFromObject(kvvm), &actual)).To(Succeed())
			Expect(actual.Spec.RunStrategy).To(Equal(ptr.To(virtv1.RunStrategyManual)))
		},
			Entry("with its original KVVM", false),
			Entry("after its KVVM was replaced normally", true),
		)

		It("recovers the KVVM UID after Create without a status update", func() {
			fakeClient, _, vmState := setupEnvironment(vm, vd, received, kvvm, receiver)
			h := &SyncKvvmHandler{client: fakeClient}
			done, err := h.syncMigrationReceiver(context.Background(), vmState, receiver)
			Expect(err).NotTo(HaveOccurred())
			Expect(done).To(BeTrue())

			var got intvirtv1alpha1.InternalVirtualizationVirtualMachineReceiver
			Expect(fakeClient.Get(context.Background(), client.ObjectKeyFromObject(receiver), &got)).To(Succeed())
			Expect(got.Status.InternalVirtualMachineUID).To(Equal(kvvm.UID))
		})

		It("does not recreate a recorded KVVM that is missing from the cache", func() {
			receiver.Status.InternalVirtualMachineUID = "kvvm-uid"
			receiver.Spec.VirtualMachineTemplate = runtime.RawExtension{Raw: []byte(`{"metadata":{"name":"web"},"spec":{}}`)}
			receiver.Spec.InternalVirtualizationVirtualMachineInstance = runtime.RawExtension{Raw: []byte(`{"spec":{}}`)}
			vm.Status.IPAddress = "10.66.10.1"
			received.Status.Phase = virtv1.MigrationPending
			fakeClient, _, vmState := setupEnvironment(vm, vd, received, receiver)
			done, err := (&SyncKvvmHandler{client: fakeClient}).syncMigrationReceiver(context.Background(), vmState, receiver)
			Expect(err).NotTo(HaveOccurred())
			Expect(done).To(BeFalse())
			Expect(k8serrors.IsNotFound(fakeClient.Get(context.Background(), client.ObjectKeyFromObject(kvvm), &virtv1.VirtualMachine{}))).To(BeTrue())
		})

		It("does not finalize a same-name replacement with another UID", func() {
			receiver.Status.InternalVirtualMachineUID = "previous-kvvm-uid"
			fakeClient, _, vmState := setupEnvironment(vm, vd, received, kvvm, receiver)
			done, err := (&SyncKvvmHandler{client: fakeClient}).syncMigrationReceiver(context.Background(), vmState, receiver)
			Expect(err).To(MatchError(ContainSubstring("UID changed")))
			Expect(done).To(BeFalse())

			var got virtv1.VirtualMachine
			Expect(fakeClient.Get(context.Background(), client.ObjectKeyFromObject(kvvm), &got)).To(Succeed())
			Expect(*got.Spec.RunStrategy).To(Equal(virtv1.RunStrategyWaitAsReceiver))

			var gotVM v1alpha2.VirtualMachine
			Expect(fakeClient.Get(context.Background(), client.ObjectKeyFromObject(vm), &gotVM)).To(Succeed())
			Expect(gotVM.OwnerReferences).NotTo(BeEmpty())
		})

		It("does not adopt a KVVM owned by another VM", func() {
			kvvm.OwnerReferences[0].UID = "another-vm"
			fakeClient, _, vmState := setupEnvironment(vm, vd, received, kvvm, receiver)
			done, err := (&SyncKvvmHandler{client: fakeClient}).syncMigrationReceiver(context.Background(), vmState, receiver)
			Expect(err).To(MatchError(ContainSubstring("not owned")))
			Expect(done).To(BeFalse())
		})

		It("before the request confirms the arrival", func() {
			fakeClient, _, vmState := setupEnvironment(vm, vd, received, kvvm, receiver)
			h := &SyncKvvmHandler{client: fakeClient}

			done, err := h.syncMigrationReceiver(context.Background(), vmState, receiver)
			Expect(err).NotTo(HaveOccurred())
			Expect(done).To(BeTrue())

			var gotVM v1alpha2.VirtualMachine
			Expect(fakeClient.Get(context.Background(), client.ObjectKeyFromObject(vm), &gotVM)).To(Succeed())
			Expect(gotVM.OwnerReferences).To(BeEmpty())

			var gotVD v1alpha2.VirtualDisk
			Expect(fakeClient.Get(context.Background(), client.ObjectKeyFromObject(vd), &gotVD)).To(Succeed())
			Expect(gotVD.OwnerReferences).To(BeEmpty())

			var gotReceiver intvirtv1alpha1.InternalVirtualizationVirtualMachineReceiver
			Expect(fakeClient.Get(context.Background(), client.ObjectKeyFromObject(receiver), &gotReceiver)).To(Succeed())
			Expect(gotReceiver.Status.Phase).To(Equal(intvirtv1alpha1.ReceiverRunning))
		})

		// The source deletes the request once it confirms the arrival: a VM still referring to it then would
		// go with it.
		It("and does not confirm the arrival while a reference stays", func() {
			fakeClient, _, vmState := setupEnvironment(vm, vd, received, kvvm, receiver)
			failing := interceptor.NewClient(fakeClient, interceptor.Funcs{
				Patch: func(ctx context.Context, cl client.WithWatch, obj client.Object, patch client.Patch, opts ...client.PatchOption) error {
					if _, ok := obj.(*v1alpha2.VirtualMachine); ok {
						return errors.New("conflict")
					}

					return cl.Patch(ctx, obj, patch, opts...)
				},
			})
			h := &SyncKvvmHandler{client: failing}

			_, err := h.syncMigrationReceiver(context.Background(), vmState, receiver)
			Expect(err).To(HaveOccurred())

			var gotReceiver intvirtv1alpha1.InternalVirtualizationVirtualMachineReceiver
			Expect(fakeClient.Get(context.Background(), client.ObjectKeyFromObject(receiver), &gotReceiver)).To(Succeed())
			Expect(gotReceiver.Status.Phase).NotTo(Equal(intvirtv1alpha1.ReceiverRunning))
			Expect(gotReceiver.Status.ReceiverStarted).To(BeFalse())
		})
	})

	It("keeps a receiving VM whose migration succeeded after the request goes", func() {
		const ns = "app-x"
		// The source removes the request as soon as its receiving migration succeeded, a moment before the
		// instance of this cluster reports Running: the guest must not go with the request.
		vm := &v1alpha2.VirtualMachine{
			ObjectMeta: metav1.ObjectMeta{Name: "web", Namespace: ns},
			Spec:       v1alpha2.VirtualMachineSpec{RunPolicy: v1alpha2.AlwaysOnPolicy},
		}
		received := &virtv1.VirtualMachineInstanceMigration{
			ObjectMeta: metav1.ObjectMeta{Name: "receiver-uid", Namespace: ns},
			Spec: virtv1.VirtualMachineInstanceMigrationSpec{
				VMIName: "web",
				Receive: &virtv1.VirtualMachineInstanceMigrationTarget{MigrationID: "receiver-uid"},
			},
			Status: virtv1.VirtualMachineInstanceMigrationStatus{Phase: virtv1.MigrationSucceeded},
		}
		kvvmi := newEmptyKVVMI("web", ns)
		kvvmi.Status.Phase = virtv1.Pending
		kvvm := newEmptyKVVM("web", ns)
		kvvm.UID = "kvvm-uid"
		kvvm.OwnerReferences = []metav1.OwnerReference{*metav1.NewControllerRef(vm, v1alpha2.SchemeGroupVersion.WithKind(v1alpha2.VirtualMachineKind))}
		kvvm.Spec.RunStrategy = ptr.To(virtv1.RunStrategyWaitAsReceiver)
		receiver := &intvirtv1alpha1.InternalVirtualizationVirtualMachineReceiver{
			ObjectMeta: metav1.ObjectMeta{Name: "web", Namespace: ns, UID: "receiver-uid"},
		}
		fakeClient, _, vmState := setupEnvironment(vm, received, kvvmi, kvvm, receiver)
		h := &SyncKvvmHandler{client: fakeClient}

		// The request goes before the receive is completed here.
		Expect(fakeClient.Delete(context.Background(), receiver)).To(Succeed())
		Expect(h.cancelVMReception(context.Background(), vmState, receiver)).To(Succeed())

		var got virtv1.VirtualMachine
		Expect(fakeClient.Get(context.Background(), types.NamespacedName{Namespace: ns, Name: "web"}, &got)).To(Succeed())
		Expect(got.DeletionTimestamp).To(BeNil())
	})
})

var _ = Describe("cancelVMReception", func() {
	DescribeTable("deletes only the unfinished migration belonging to the receiver without listing migrations",
		func(change func(*virtv1.VirtualMachineInstanceMigration), receiverPresent, deleted bool) {
			const ns = "app-x"

			ctx := context.Background()
			vm := &v1alpha2.VirtualMachine{ObjectMeta: metav1.ObjectMeta{Name: "web", Namespace: ns}}
			received := &virtv1.VirtualMachineInstanceMigration{
				ObjectMeta: metav1.ObjectMeta{Name: "receiver-uid", Namespace: ns, UID: "migration-uid"},
				Spec: virtv1.VirtualMachineInstanceMigrationSpec{
					VMIName: "web", Receive: &virtv1.VirtualMachineInstanceMigrationTarget{MigrationID: "receiver-uid"},
				},
			}
			change(received)
			fakeClient, _, vmState := setupEnvironment(vm, received)
			c := interceptor.NewClient(fakeClient, interceptor.Funcs{
				List: func(context.Context, client.WithWatch, client.ObjectList, ...client.ListOption) error {
					return errors.New("unexpected List during receive cancellation")
				},
				Delete: func(ctx context.Context, cl client.WithWatch, obj client.Object, opts ...client.DeleteOption) error {
					if _, ok := obj.(*virtv1.VirtualMachineInstanceMigration); ok {
						options := (&client.DeleteOptions{}).ApplyOptions(opts)
						Expect(options.Preconditions).NotTo(BeNil())
						Expect(options.Preconditions.UID).To(Equal(ptr.To(types.UID("migration-uid"))))
						Expect(options.Preconditions.ResourceVersion).To(Equal(ptr.To(obj.GetResourceVersion())))
					}

					return cl.Delete(ctx, obj, opts...)
				},
			})

			var receiver *intvirtv1alpha1.InternalVirtualizationVirtualMachineReceiver
			if receiverPresent {
				receiver = &intvirtv1alpha1.InternalVirtualizationVirtualMachineReceiver{
					ObjectMeta: metav1.ObjectMeta{Name: "web", Namespace: ns, UID: "receiver-uid"},
				}
			}

			Expect((&SyncKvvmHandler{client: c}).cancelVMReception(ctx, vmState, receiver)).To(Succeed())

			err := fakeClient.Get(ctx, client.ObjectKeyFromObject(received), &virtv1.VirtualMachineInstanceMigration{})
			if deleted {
				Expect(k8serrors.IsNotFound(err)).To(BeTrue())
			} else {
				Expect(err).NotTo(HaveOccurred())
			}
		},
		Entry("the receiver's migration", func(*virtv1.VirtualMachineInstanceMigration) {}, true, true),
		Entry("a local migration", func(m *virtv1.VirtualMachineInstanceMigration) { m.Spec.Receive = nil }, true, false),
		Entry("another VM", func(m *virtv1.VirtualMachineInstanceMigration) { m.Spec.VMIName = "other" }, true, false),
		Entry("another receiver", func(m *virtv1.VirtualMachineInstanceMigration) { m.Spec.Receive.MigrationID = "other" }, true, false),
		Entry("a succeeded migration", func(m *virtv1.VirtualMachineInstanceMigration) { m.Status.Phase = virtv1.MigrationSucceeded }, true, false),
		Entry("an unknown receiver", func(*virtv1.VirtualMachineInstanceMigration) {}, false, false),
	)
})

var _ = Describe("createReceiverIPAddress", func() {
	const (
		ns      = "app-x"
		address = "10.66.10.5"
	)

	newVM := func() *v1alpha2.VirtualMachine {
		return &v1alpha2.VirtualMachine{
			TypeMeta:   metav1.TypeMeta{APIVersion: v1alpha2.SchemeGroupVersion.String(), Kind: v1alpha2.VirtualMachineKind},
			ObjectMeta: metav1.ObjectMeta{Name: "web", Namespace: ns, UID: "vm-uid"},
		}
	}
	newReceiver := func() *intvirtv1alpha1.InternalVirtualizationVirtualMachineReceiver {
		return &intvirtv1alpha1.InternalVirtualizationVirtualMachineReceiver{
			ObjectMeta: metav1.ObjectMeta{Name: "web", Namespace: ns, UID: "receiver-uid"},
			Status:     intvirtv1alpha1.VirtualMachineReceiverStatus{Phase: intvirtv1alpha1.ReceiverPending},
		}
	}

	It("accepts the address created by a previous reconcile", func() {
		vm := newVM()
		receiver := newReceiver()
		own := &v1alpha2.VirtualMachineIPAddress{
			ObjectMeta: metav1.ObjectMeta{Name: "web-ip", Namespace: ns, Labels: map[string]string{annotations.LabelVirtualMachineUID: string(vm.UID)}},
			Spec:       v1alpha2.VirtualMachineIPAddressSpec{Type: v1alpha2.VirtualMachineIPAddressTypeStatic, StaticIP: address},
		}
		base, err := testutil.NewFakeClientWithObjects(vm, receiver, own)
		Expect(err).NotTo(HaveOccurred())

		h := &IPAMHandler{ipam: netmanager.NewIPAM(), client: denyVMIPCreateClient{base}, apiReader: base}

		Expect(h.createReceiverIPAddress(context.Background(), vm, receiver, address)).To(Succeed())
		Expect(receiver.Status.Phase).To(Equal(intvirtv1alpha1.ReceiverPending))
	})

	It("fails the receiver when the address belongs to no address of this VM", func() {
		vm := newVM()
		receiver := newReceiver()
		base, err := testutil.NewFakeClientWithObjects(vm, receiver)
		Expect(err).NotTo(HaveOccurred())

		h := &IPAMHandler{ipam: netmanager.NewIPAM(), client: denyVMIPCreateClient{base}, apiReader: base}

		Expect(h.createReceiverIPAddress(context.Background(), vm, receiver, address)).To(Succeed())

		var got intvirtv1alpha1.InternalVirtualizationVirtualMachineReceiver
		Expect(base.Get(context.Background(), types.NamespacedName{Namespace: ns, Name: "web"}, &got)).To(Succeed())
		Expect(got.Status.Phase).To(Equal(intvirtv1alpha1.ReceiverFailed))
		Expect(got.Status.Reason).To(Equal(peering.ReceiverReasonIPAddressNotAvailable))
		Expect(got.Status.Message).To(ContainSubstring("the IP address is taken. Free the address there"))
		Expect(got.Status.Message).NotTo(ContainSubstring("admission webhook"))
	})
})

var _ = Describe("syncMigrationReceiver address", func() {
	It("fails the receiver when the source address is replaced by an Auto address", func() {
		const ns = "app-x"

		vm := &v1alpha2.VirtualMachine{
			ObjectMeta: metav1.ObjectMeta{Name: "web", Namespace: ns, UID: "vm-uid"},
			Status:     v1alpha2.VirtualMachineStatus{IPAddress: "10.66.10.1"},
		}
		receiver := &intvirtv1alpha1.InternalVirtualizationVirtualMachineReceiver{
			ObjectMeta: metav1.ObjectMeta{Name: "web", Namespace: ns, UID: "receiver-uid"},
			Spec: intvirtv1alpha1.VirtualMachineReceiverSpec{
				VirtualMachineTemplate:                       runtime.RawExtension{Raw: []byte(`{"metadata":{"name":"web"},"spec":{}}`)},
				InternalVirtualizationVirtualMachineInstance: runtime.RawExtension{Raw: []byte(`{"spec":{}}`)},
				IPAddress: "10.66.12.200",
			},
		}
		vmip := &v1alpha2.VirtualMachineIPAddress{
			ObjectMeta: metav1.ObjectMeta{
				Name: "web-ip", Namespace: ns,
				Labels: map[string]string{annotations.LabelVirtualMachineUID: "vm-uid"},
			},
			Spec:   v1alpha2.VirtualMachineIPAddressSpec{Type: v1alpha2.VirtualMachineIPAddressTypeAuto},
			Status: v1alpha2.VirtualMachineIPAddressStatus{Address: "10.66.10.1"},
		}
		fakeClient, _, vmState := setupEnvironment(vm, receiver, vmip)
		h := &SyncKvvmHandler{client: fakeClient}

		done, err := h.syncMigrationReceiver(context.Background(), vmState, receiver)
		Expect(err).NotTo(HaveOccurred())
		Expect(done).To(BeFalse())

		var got intvirtv1alpha1.InternalVirtualizationVirtualMachineReceiver
		Expect(fakeClient.Get(context.Background(), types.NamespacedName{Namespace: ns, Name: "web"}, &got)).To(Succeed())
		Expect(got.Status.Phase).To(Equal(intvirtv1alpha1.ReceiverFailed))
		Expect(got.Status.Reason).To(Equal(peering.ReceiverReasonIPAddressNotAvailable))
	})
})

var _ = Describe("syncMigrationReceiver quota", func() {
	It("fails the receiver at once when a ResourceQuota refuses the receiving pod", func() {
		const ns = "pt-dr-test"

		vm := &v1alpha2.VirtualMachine{
			ObjectMeta: metav1.ObjectMeta{Name: "web", Namespace: ns, UID: "vm-uid"},
			Status:     v1alpha2.VirtualMachineStatus{IPAddress: "10.66.10.1"},
		}
		receiver := &intvirtv1alpha1.InternalVirtualizationVirtualMachineReceiver{
			ObjectMeta: metav1.ObjectMeta{Name: "web", Namespace: ns, UID: "receiver-uid"},
			Spec: intvirtv1alpha1.VirtualMachineReceiverSpec{
				VirtualMachineTemplate:                       runtime.RawExtension{Raw: []byte(`{"metadata":{"name":"web"},"spec":{}}`)},
				InternalVirtualizationVirtualMachineInstance: runtime.RawExtension{Raw: []byte(`{"spec":{}}`)},
			},
			Status: intvirtv1alpha1.VirtualMachineReceiverStatus{Phase: intvirtv1alpha1.ReceiverPending, VirtualMachineUID: "vm-uid"},
		}
		kvvm := newEmptyKVVM("web", ns)
		kvvm.UID = "kvvm-uid"
		kvvm.OwnerReferences = []metav1.OwnerReference{*metav1.NewControllerRef(vm, v1alpha2.SchemeGroupVersion.WithKind(v1alpha2.VirtualMachineKind))}
		kvvm.Spec.RunStrategy = ptr.To(virtv1.RunStrategyWaitAsReceiver)
		kvvmi := newEmptyKVVMI("web", ns)
		kvvmi.UID = "vmi-uid"
		refused := &corev1.Event{
			ObjectMeta:     metav1.ObjectMeta{Name: "web.1", Namespace: ns},
			InvolvedObject: corev1.ObjectReference{Name: "web", Namespace: ns, UID: "vmi-uid"},
			Type:           corev1.EventTypeWarning, Reason: "FailedCreate",
			Message: `Error creating pod: pods "virt-launcher-web-x7k2p" is forbidden: exceeded quota: c22-quota, requested: requests.memory=640Mi, used: requests.memory=1Gi, limited: requests.memory=64Mi`,
		}
		fakeClient, _, vmState := setupEnvironment(vm, receiver, kvvm, kvvmi, refused)
		h := &SyncKvvmHandler{client: fakeClient}

		done, err := h.syncMigrationReceiver(context.Background(), vmState, receiver)
		Expect(err).NotTo(HaveOccurred())
		Expect(done).To(BeFalse())

		var got intvirtv1alpha1.InternalVirtualizationVirtualMachineReceiver
		Expect(fakeClient.Get(context.Background(), client.ObjectKeyFromObject(receiver), &got)).To(Succeed())
		Expect(got.Status.Phase).To(Equal(intvirtv1alpha1.ReceiverFailed))
		Expect(got.Status.Reason).To(Equal(peering.ReceiverReasonQuotaExceeded))
		Expect(got.Status.Message).To(Equal(`the ResourceQuota "c22-quota" of namespace "pt-dr-test" does not allow the virtual machine (requested: requests.memory=640Mi, used: requests.memory=1Gi, limited: requests.memory=64Mi): free resources there or raise the quota`))
	})
})

var _ = Describe("syncReceivingFromPeer", func() {
	const ns = "app-x"

	ref := []metav1.OwnerReference{{Kind: intvirtv1alpha1.InternalVirtualizationVirtualMachineReceiverKind, Name: "web", UID: "receiver-uid"}}
	newClient := func(objs ...client.Object) client.Client {
		scheme := runtime.NewScheme()
		Expect(intvirtv1alpha1.AddToScheme(scheme)).To(Succeed())

		return fake.NewClientBuilder().WithScheme(scheme).WithObjects(objs...).Build()
	}
	request := func(peer string, phase intvirtv1alpha1.VirtualMachineReceiverPhase) *intvirtv1alpha1.InternalVirtualizationVirtualMachineReceiver {
		r := &intvirtv1alpha1.InternalVirtualizationVirtualMachineReceiver{
			ObjectMeta: metav1.ObjectMeta{Namespace: ns, Name: "web", UID: "receiver-uid"},
			Status:     intvirtv1alpha1.VirtualMachineReceiverStatus{Phase: phase},
		}
		if peer != "" {
			r.Spec.SourcePeer = peer
		}

		return r
	}
	newVM := func(refs []metav1.OwnerReference) *v1alpha2.VirtualMachine {
		return &v1alpha2.VirtualMachine{ObjectMeta: metav1.ObjectMeta{Namespace: ns, Name: "web", UID: "vm-uid", OwnerReferences: refs}}
	}
	markedKVVM := func() *virtv1.VirtualMachine {
		kvvm := newEmptyKVVM("web", ns)
		kvvm.UID = "kvvm-uid"
		kvvm.Spec.RunStrategy = ptr.To(virtv1.RunStrategyWaitAsReceiver)

		return kvvm
	}
	receiving := func(vm *v1alpha2.VirtualMachine) *metav1.Condition {
		for i := range vm.Status.Conditions {
			if vm.Status.Conditions[i].Type == vmcondition.TypeReceivingFromPeer.String() {
				return &vm.Status.Conditions[i]
			}
		}

		return nil
	}

	It("names the cluster the VM is received from", func() {
		vm := newVM(ref)
		Expect(syncReceivingFromPeer(context.Background(), newClient(request("cluster-a", "")), vm, nil, nil)).To(Succeed())
		c := receiving(vm)
		Expect(c).NotTo(BeNil())
		Expect(c.Status).To(Equal(metav1.ConditionTrue))
		Expect(c.Reason).To(Equal(vmcondition.ReasonReceivingFromPeerInProgress.String()))
		Expect(c.Message).To(Equal(`The virtual machine is being received from peer cluster "cluster-a".`))
		Expect(vm.Status.MigrationState.Source.ProjectPeerName).To(Equal("cluster-a"))
	})

	It("tells of a peer cluster when the request does not name it", func() {
		vm := newVM(ref)
		Expect(syncReceivingFromPeer(context.Background(), newClient(request("", "")), vm, nil, nil)).To(Succeed())
		Expect(receiving(vm).Message).To(Equal("The virtual machine is being received from a peer cluster."))
		Expect(vm.Status.MigrationState).To(BeNil())
	})

	// The request may go a moment before the receiving KVVM is unmarked: the receive goes on, and the peer
	// stays the one shown before.
	It("keeps the peer while the request is gone and the KVVM still receives", func() {
		vm := newVM(nil)
		prev := &v1alpha2.VirtualMachineMigrationState{Source: v1alpha2.VirtualMachineLocation{ProjectPeerName: "cluster-a"}}
		Expect(syncReceivingFromPeer(context.Background(), newClient(), vm, markedKVVM(), prev)).To(Succeed())
		Expect(receiving(vm).Message).To(Equal(`The virtual machine is being received from peer cluster "cluster-a".`))
		Expect(vm.Status.MigrationState.Source.ProjectPeerName).To(Equal("cluster-a"))
	})

	It("touches no condition of a VM whose request is not seen yet", func() {
		vm := newVM(ref)
		prev := &v1alpha2.VirtualMachineMigrationState{Source: v1alpha2.VirtualMachineLocation{ProjectPeerName: "cluster-a"}}
		Expect(syncReceivingFromPeer(context.Background(), newClient(), vm, nil, prev)).To(Succeed())
		Expect(receiving(vm)).To(BeNil())
		Expect(vm.Status.MigrationState.Source.ProjectPeerName).To(Equal("cluster-a"))
	})

	It("removes the condition once the VM runs here", func() {
		vm := newVM(nil)
		vm.Status.Conditions = []metav1.Condition{{Type: vmcondition.TypeReceivingFromPeer.String(), Status: metav1.ConditionTrue}}
		r := request("cluster-a", intvirtv1alpha1.ReceiverRunning)
		r.Status.VirtualMachineUID = "vm-uid"
		Expect(syncReceivingFromPeer(context.Background(), newClient(r), vm, nil, nil)).To(Succeed())
		Expect(receiving(vm)).To(BeNil())
		Expect(vm.Status.MigrationState).To(BeNil())
	})

	It("shows nothing for a regular VM", func() {
		vm := newVM(nil)
		Expect(syncReceivingFromPeer(context.Background(), newClient(), vm, nil, nil)).To(Succeed())
		Expect(receiving(vm)).To(BeNil())
		Expect(vm.Status.MigrationState).To(BeNil())
	})
})

var _ = Describe("syncMovedToPeer", func() {
	const ns = "app-x"

	newClient := func(objs ...client.Object) client.Client {
		cl, err := testutil.NewFakeClientWithObjects(objs...)
		Expect(err).NotTo(HaveOccurred())

		return cl
	}
	move := &v1alpha2.VirtualMachineOperation{
		ObjectMeta: metav1.ObjectMeta{Name: "move", Namespace: ns},
		Spec: v1alpha2.VirtualMachineOperationSpec{
			Type: v1alpha2.VMOPTypeMigrate, VirtualMachine: "web",
			Migrate: &v1alpha2.VirtualMachineOperationMigrateSpec{ProjectPeerName: "cluster-b"},
		},
	}
	markedKVVM := func() *virtv1.VirtualMachine {
		kvvm := newEmptyKVVM("web", ns)
		kvvm.Annotations = map[string]string{annotations.AnnPeerMigrationVMOP: "move"}

		return kvvm
	}
	newVM := func() *v1alpha2.VirtualMachine {
		return &v1alpha2.VirtualMachine{ObjectMeta: metav1.ObjectMeta{Namespace: ns, Name: "web"}}
	}
	moving := func(vm *v1alpha2.VirtualMachine) *metav1.Condition {
		return meta.FindStatusCondition(vm.Status.Conditions, vmcondition.TypeMovedToPeer.String())
	}

	It("names the cluster and the operation the VM moves by", func() {
		vm := newVM()
		Expect(syncMovedToPeer(context.Background(), newClient(move), vm, markedKVVM(), nil)).To(Succeed())
		c := moving(vm)
		Expect(c.Status).To(Equal(metav1.ConditionTrue))
		Expect(c.Reason).To(Equal(vmcondition.ReasonMovedToPeerInProgress.String()))
		Expect(c.Message).To(Equal(`The virtual machine is being moved to peer cluster "cluster-b" by VirtualMachineOperation "move".`))
		Expect(vm.Status.MigrationState.Target.ProjectPeerName).To(Equal("cluster-b"))
	})

	It("tells of a peer cluster when the operation is not seen", func() {
		vm := newVM()
		Expect(syncMovedToPeer(context.Background(), newClient(), vm, markedKVVM(), nil)).To(Succeed())
		Expect(moving(vm).Message).To(Equal(`The virtual machine is being moved to a peer cluster by VirtualMachineOperation "move".`))
		Expect(vm.Status.MigrationState).To(BeNil())
	})

	// The condition stands for the mark while the KubeVirt VM is not seen: it stays as it was.
	It("keeps the condition of a VM whose KubeVirt VM is not seen", func() {
		vm := newVM()
		vm.Status.Conditions = []metav1.Condition{{Type: vmcondition.TypeMovedToPeer.String(), Status: metav1.ConditionTrue, Message: "was"}}
		prev := &v1alpha2.VirtualMachineMigrationState{Target: v1alpha2.VirtualMachineLocation{ProjectPeerName: "cluster-b"}}
		Expect(syncMovedToPeer(context.Background(), newClient(move), vm, nil, prev)).To(Succeed())
		Expect(moving(vm).Message).To(Equal("was"))
		Expect(vm.Status.MigrationState.Target.ProjectPeerName).To(Equal("cluster-b"))
	})

	It("removes the condition once the mark goes", func() {
		vm := newVM()
		vm.Status.Conditions = []metav1.Condition{{Type: vmcondition.TypeMovedToPeer.String(), Status: metav1.ConditionTrue}}
		Expect(syncMovedToPeer(context.Background(), newClient(move), vm, newEmptyKVVM("web", ns), nil)).To(Succeed())
		Expect(moving(vm)).To(BeNil())
		Expect(vm.Status.MigrationState).To(BeNil())
	})
})

// A VM controller that took a moving VM for a regular one would create its KubeVirt VM and start a second
// guest here while the first one runs in the peer.
var _ = Describe("SyncKvvmHandler of a VM that moves to a peer", func() {
	It("creates no KubeVirt VM while the VM shows the move and its KubeVirt VM is not seen", func() {
		vm := &v1alpha2.VirtualMachine{
			ObjectMeta: metav1.ObjectMeta{Namespace: "app-x", Name: "web"},
			Spec:       v1alpha2.VirtualMachineSpec{RunPolicy: v1alpha2.AlwaysOnPolicy},
			Status: v1alpha2.VirtualMachineStatus{
				Phase:      v1alpha2.MachineRunning,
				Conditions: []metav1.Condition{{Type: vmcondition.TypeMovedToPeer.String(), Status: metav1.ConditionTrue}},
			},
		}
		fakeClient, _, vmState := setupEnvironment(vm)
		h := &SyncKvvmHandler{client: fakeClient}
		_, err := h.Handle(context.Background(), vmState)
		Expect(err).NotTo(HaveOccurred())
		Expect(k8serrors.IsNotFound(fakeClient.Get(context.Background(), client.ObjectKeyFromObject(vm), &virtv1.VirtualMachine{}))).To(BeTrue())
	})
})
