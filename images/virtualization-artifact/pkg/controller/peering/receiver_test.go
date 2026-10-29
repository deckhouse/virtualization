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
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	schedulingv1 "k8s.io/api/scheduling/v1"
	storagev1 "k8s.io/api/storage/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	virtv1 "kubevirt.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/deckhouse/virtualization-controller/pkg/common/annotations"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
	intvirtv1alpha1 "github.com/deckhouse/virtualization/api/internalvirtualization/v1alpha1"
)

func newScheme() *runtime.Scheme {
	scheme := runtime.NewScheme()
	Expect(corev1.AddToScheme(scheme)).To(Succeed())
	Expect(v1alpha2.AddToScheme(scheme)).To(Succeed())
	Expect(intvirtv1alpha1.AddToScheme(scheme)).To(Succeed())
	return scheme
}

var _ = Describe("ReceiverReconciler", func() {
	const ns = "app-x"
	ctx := context.Background()
	key := types.NamespacedName{Namespace: ns, Name: "web"}

	var (
		cl client.Client
		r  *ReceiverReconciler
	)
	const vmSpec = `{"metadata":{"labels":{"app":"web"},"annotations":{"owner":"team-a"}},"spec":{"virtualMachineClassName":"generic",` +
		`"priorityClassName":"high","nodeSelector":{"zone":"a"},"virtualMachineIPAddressName":"web-ip-a","runPolicy":"AlwaysOn"}}`
	newReceiver := func() *intvirtv1alpha1.InternalVirtualizationVirtualMachineReceiver {
		return &intvirtv1alpha1.InternalVirtualizationVirtualMachineReceiver{
			ObjectMeta: metav1.ObjectMeta{Namespace: ns, Name: "web", UID: "uid-1"},
			Spec: intvirtv1alpha1.VirtualMachineReceiverSpec{
				VirtualMachineTemplate: runtime.RawExtension{Raw: []byte(vmSpec)},
				VirtualDiskTemplates: []runtime.RawExtension{{Raw: []byte(
					`{"metadata":{"name":"web-root","labels":{"tier":"db"}},"spec":{"persistentVolumeClaim":{"storageClassName":"local","size":"4Gi"}}}`,
				)}},
				InternalVirtualizationVirtualMachineInstance: runtime.RawExtension{Raw: []byte(
					`{"spec":{"domain":{"devices":{"disks":[{"name":"vd-web-root","serial":"3f2a"}]}}}}`,
				)},
				IPAddress: "10.66.10.5",
			},
		}
	}
	reconcileOnce := func() {
		GinkgoHelper()
		_, err := r.Reconcile(ctx, reconcile.Request{NamespacedName: key})
		Expect(err).NotTo(HaveOccurred())
	}
	receiver := func() *intvirtv1alpha1.InternalVirtualizationVirtualMachineReceiver {
		GinkgoHelper()
		var rcvr intvirtv1alpha1.InternalVirtualizationVirtualMachineReceiver
		Expect(cl.Get(ctx, key, &rcvr)).To(Succeed())
		return &rcvr
	}
	setup := func(objs ...client.Object) {
		objs = append(objs,
			&v1alpha2.VirtualMachineClass{ObjectMeta: metav1.ObjectMeta{Name: "generic"}},
			&schedulingv1.PriorityClass{ObjectMeta: metav1.ObjectMeta{Name: "high"}},
			&storagev1.StorageClass{ObjectMeta: metav1.ObjectMeta{Name: "local"}},
			&virtv1.KubeVirt{ObjectMeta: metav1.ObjectMeta{Namespace: "d8-virtualization", Name: "config"}, Status: virtv1.KubeVirtStatus{Phase: virtv1.KubeVirtPhaseDeployed}})
		scheme := newScheme()
		Expect(schedulingv1.AddToScheme(scheme)).To(Succeed())
		Expect(storagev1.AddToScheme(scheme)).To(Succeed())
		Expect(virtv1.AddToScheme(scheme)).To(Succeed())
		cl = fake.NewClientBuilder().WithScheme(scheme).WithObjects(objs...).
			WithStatusSubresource(&intvirtv1alpha1.InternalVirtualizationVirtualMachineReceiver{}).Build()
		r = &ReceiverReconciler{client: cl, namespace: "d8-virtualization"}
	}

	It("publishes the address of this cluster and hands the pin of the source to the receiving migration", func() {
		rcvr := newReceiver()
		rcvr.Annotations = map[string]string{annotations.AnnReceiverSourceSyncPin: "sha256:A"}
		receiving := &virtv1.VirtualMachineInstanceMigration{ObjectMeta: metav1.ObjectMeta{Namespace: ns, Name: "uid-1"}}
		setup(rcvr, receiving)
		kv := &virtv1.KubeVirt{}
		Expect(cl.Get(ctx, types.NamespacedName{Namespace: "d8-virtualization", Name: "config"}, kv)).To(Succeed())
		kv.Status.SynchronizationAddresses = []string{"172.17.9.10:9185"}
		Expect(cl.Update(ctx, kv)).To(Succeed())
		reconcileOnce()
		Expect(receiver().Status.SynchronizationAddress).To(Equal("172.17.9.10:9185"))
		Expect(cl.Get(ctx, client.ObjectKeyFromObject(receiving), receiving)).To(Succeed())
		Expect(receiving.Annotations).To(HaveKeyWithValue(annotations.AnnMigrationPeerSyncPin, "sha256:A"))
	})

	It("confirms the arrival of the guest once the receiving migration succeeded", func() {
		receiving := &virtv1.VirtualMachineInstanceMigration{
			ObjectMeta: metav1.ObjectMeta{Namespace: ns, Name: "uid-1"},
			Spec:       virtv1.VirtualMachineInstanceMigrationSpec{VMIName: "web", Receive: &virtv1.VirtualMachineInstanceMigrationTarget{MigrationID: "uid-1"}},
			Status:     virtv1.VirtualMachineInstanceMigrationStatus{Phase: virtv1.MigrationSucceeded},
		}
		setup(newReceiver(), receiving)
		reconcileOnce()
		Expect(receiver().Status.ReceiverStarted).To(BeTrue())

		failed := newReceiver()
		failed.Status.Phase = intvirtv1alpha1.ReceiverFailed
		setup(failed, receiving)
		reconcileOnce()
		Expect(receiver().Status.ReceiverStarted).To(BeTrue(), "a request failed during the copy still reports the arrival")
	})

	It("creates the disks and the VM of the request, marked with it", func() {
		setup(newReceiver())
		reconcileOnce()
		Expect(receiver().Status.Phase).To(Equal(intvirtv1alpha1.ReceiverPending))

		var vd v1alpha2.VirtualDisk
		Expect(cl.Get(ctx, types.NamespacedName{Namespace: ns, Name: "web-root"}, &vd)).To(Succeed())
		Expect(vd.Labels).To(Equal(map[string]string{"tier": "db", annotations.LabelVMReceiver: "uid-1"}))
		Expect(vd.Annotations).To(HaveKeyWithValue(annotations.AnnVDSerial, "3f2a"))
		Expect(*vd.Spec.PersistentVolumeClaim.StorageClass).To(Equal("local"))
		Expect(vd.Spec.PersistentVolumeClaim.Size.String()).To(Equal("4Gi"))
		Expect(vd.OwnerReferences).To(BeEmpty())

		var vm v1alpha2.VirtualMachine
		Expect(cl.Get(ctx, key, &vm)).To(Succeed())
		Expect(vm.Labels).To(Equal(map[string]string{"app": "web", annotations.LabelVMReceiver: "uid-1"}))
		Expect(vm.Annotations).To(Equal(map[string]string{"owner": "team-a", annotations.AnnVMMigrationReceiver: "uid-1"}))
		Expect(vm.Spec.NodeSelector).To(Equal(map[string]string{"zone": "a"}))
		Expect(vm.Spec.PriorityClassName).To(Equal("high"))
		// The address object of the source means nothing here.
		Expect(vm.Spec.VirtualMachineIPAddress).To(BeEmpty())
		Expect(vm.OwnerReferences).To(BeEmpty())
	})

	It("takes a free address object with the address of the source", func() {
		setup(newReceiver(),
			&v1alpha2.VirtualMachineIPAddress{
				ObjectMeta: metav1.ObjectMeta{Namespace: ns, Name: "web-kept"},
				Status:     v1alpha2.VirtualMachineIPAddressStatus{Address: "10.66.10.5", Phase: v1alpha2.VirtualMachineIPAddressPhaseBound},
			})
		reconcileOnce()
		var vm v1alpha2.VirtualMachine
		Expect(cl.Get(ctx, key, &vm)).To(Succeed())
		Expect(vm.Spec.VirtualMachineIPAddress).To(Equal("web-kept"))
	})

	DescribeTable("fails the request before creating anything",
		func(extra, drop client.Object, reason, message string) {
			var objs []client.Object
			if extra != nil {
				objs = append(objs, extra)
			}
			setup(append(objs, newReceiver())...)
			if drop != nil {
				Expect(cl.Delete(ctx, drop)).To(Succeed())
			}
			reconcileOnce()
			Expect(receiver().Status.Phase).To(Equal(intvirtv1alpha1.ReceiverFailed))
			Expect(receiver().Status.Reason).To(Equal(reason))
			Expect(receiver().Status.Message).To(ContainSubstring(message))
			Expect(k8serrors.IsNotFound(cl.Get(ctx, types.NamespacedName{Namespace: ns, Name: "web-root"}, &v1alpha2.VirtualDisk{}))).To(BeTrue())
		},
		Entry("a VM of the same name", &v1alpha2.VirtualMachine{ObjectMeta: metav1.ObjectMeta{Namespace: ns, Name: "web"}}, nil,
			ReceiverReasonVirtualMachineExists, `"web"`),
		Entry("no class", nil, &v1alpha2.VirtualMachineClass{ObjectMeta: metav1.ObjectMeta{Name: "generic"}},
			ReceiverReasonNotFound, `VirtualMachineClass "generic"`),
		Entry("no PriorityClass", nil, &schedulingv1.PriorityClass{ObjectMeta: metav1.ObjectMeta{Name: "high"}},
			ReceiverReasonNotFound, `PriorityClass "high"`),
		Entry("no StorageClass", nil, &storagev1.StorageClass{ObjectMeta: metav1.ObjectMeta{Name: "local"}},
			ReceiverReasonNotFound, `StorageClass "local"`),
	)

	DescribeTable("waits for a VM or disk of the same name that is being deleted",
		func(obj client.Object) {
			obj.SetFinalizers([]string{"test"})
			setup(obj, newReceiver())
			Expect(cl.Delete(ctx, obj)).To(Succeed())
			reconcileOnce()
			Expect(receiver().Status.Phase).To(Equal(intvirtv1alpha1.ReceiverPending))
		},
		Entry("VM", &v1alpha2.VirtualMachine{ObjectMeta: metav1.ObjectMeta{Namespace: ns, Name: "web"}}),
		Entry("disk", &v1alpha2.VirtualDisk{ObjectMeta: metav1.ObjectMeta{Namespace: ns, Name: "web-root"}}),
	)

	It("refuses a VM whose provisioning Secret is not in the project, and takes it once the Secret is there", func() {
		withSecret := func() *intvirtv1alpha1.InternalVirtualizationVirtualMachineReceiver {
			rcvr := newReceiver()
			rcvr.Spec.VirtualMachineTemplate.Raw = []byte(`{"spec":{"virtualMachineClassName":"generic",` +
				`"provisioning":{"type":"UserDataRef","userDataRef":{"kind":"Secret","name":"web-ud"}}}}`)
			return rcvr
		}
		setup(withSecret())
		reconcileOnce()
		Expect(receiver().Status.Reason).To(Equal(ReceiverReasonNotFound))
		Expect(receiver().Status.Message).To(ContainSubstring(`Secret "web-ud"`))

		setup(withSecret(), &corev1.Secret{ObjectMeta: metav1.ObjectMeta{Namespace: ns, Name: "web-ud"}})
		reconcileOnce()
		Expect(receiver().Status.Phase).To(Equal(intvirtv1alpha1.ReceiverPending))
	})

	It("removes a request whose move never started, and keeps one that is receiving", func() {
		idle := newReceiver()
		idle.CreationTimestamp = metav1.NewTime(time.Now().Add(-receiverIdleTTL - time.Minute))
		idle.Status.Phase = intvirtv1alpha1.ReceiverReady
		setup(idle, &v1alpha2.VirtualMachine{ObjectMeta: metav1.ObjectMeta{Namespace: ns, Name: "web", Labels: map[string]string{annotations.LabelVMReceiver: "uid-1"}}})
		reconcileOnce() // deletes the request
		reconcileOnce() // removes what it created
		reconcileOnce() // lets its finalizer go
		Expect(k8serrors.IsNotFound(cl.Get(ctx, key, &intvirtv1alpha1.InternalVirtualizationVirtualMachineReceiver{}))).To(BeTrue())
		Expect(k8serrors.IsNotFound(cl.Get(ctx, key, &v1alpha2.VirtualMachine{}))).To(BeTrue(), "the VM with blank disks goes with the request")

		receiving := newReceiver()
		receiving.CreationTimestamp = metav1.NewTime(time.Now().Add(-receiverIdleTTL - time.Minute))
		receiving.Status.Phase = intvirtv1alpha1.ReceiverReady
		now := metav1.Now()
		vmim := &virtv1.VirtualMachineInstanceMigration{
			ObjectMeta: metav1.ObjectMeta{Namespace: ns, Name: "uid-1"},
			Spec:       virtv1.VirtualMachineInstanceMigrationSpec{VMIName: "web", Receive: &virtv1.VirtualMachineInstanceMigrationTarget{MigrationID: "uid-1"}},
			Status:     virtv1.VirtualMachineInstanceMigrationStatus{Phase: virtv1.MigrationRunning, MigrationState: &virtv1.VirtualMachineInstanceMigrationState{StartTimestamp: &now}},
		}
		setup(receiving, vmim, &v1alpha2.VirtualMachine{ObjectMeta: metav1.ObjectMeta{Namespace: ns, Name: "web", Labels: map[string]string{annotations.LabelVMReceiver: "uid-1"}}})
		reconcileOnce()
		Expect(cl.Get(ctx, key, &intvirtv1alpha1.InternalVirtualizationVirtualMachineReceiver{})).To(Succeed(), "a request whose migration runs stays")
	})

	It("removes a ready request whose receiving migration waited for the source all along", func() {
		idle := newReceiver()
		idle.CreationTimestamp = metav1.NewTime(time.Now().Add(-receiverIdleTTL - time.Minute))
		idle.Status.Phase = intvirtv1alpha1.ReceiverReady
		vmim := &virtv1.VirtualMachineInstanceMigration{
			ObjectMeta: metav1.ObjectMeta{Namespace: ns, Name: "uid-1"},
			Spec:       virtv1.VirtualMachineInstanceMigrationSpec{VMIName: "web", Receive: &virtv1.VirtualMachineInstanceMigrationTarget{MigrationID: "uid-1"}},
			Status:     virtv1.VirtualMachineInstanceMigrationStatus{Phase: virtv1.MigrationWaitingForSync},
		}
		vmi := &virtv1.VirtualMachineInstance{ObjectMeta: metav1.ObjectMeta{Namespace: ns, Name: "web"}, Status: virtv1.VirtualMachineInstanceStatus{Phase: virtv1.Scheduled}}
		setup(idle, vmim, vmi, &v1alpha2.VirtualMachine{ObjectMeta: metav1.ObjectMeta{Namespace: ns, Name: "web", Labels: map[string]string{annotations.LabelVMReceiver: "uid-1"}}})
		reconcileOnce()
		Expect(k8serrors.IsNotFound(cl.Get(ctx, key, &intvirtv1alpha1.InternalVirtualizationVirtualMachineReceiver{})) || !receiver().DeletionTimestamp.IsZero()).To(BeTrue())
	})

	DescribeTable("defers cleanup when arrival or activity cannot be ruled out",
		func(migration *virtv1.VirtualMachineInstanceMigration, vmi *virtv1.VirtualMachineInstance, arrived bool) {
			rcvr := newReceiver()
			rcvr.Finalizers = []string{FinalizerReceiverCleanup}
			vm := &v1alpha2.VirtualMachine{ObjectMeta: metav1.ObjectMeta{Namespace: ns, Name: "web", Labels: map[string]string{annotations.LabelVMReceiver: "uid-1"}}}
			disk := &v1alpha2.VirtualDisk{ObjectMeta: metav1.ObjectMeta{Namespace: ns, Name: "web-root", Labels: map[string]string{annotations.LabelVMReceiver: "uid-1"}}}
			var objs []client.Object
			objs = append(objs, rcvr, vm, disk)
			if migration != nil {
				objs = append(objs, migration)
			}
			if vmi != nil {
				objs = append(objs, vmi)
			}
			setup(objs...)
			Expect(cl.Delete(ctx, receiver())).To(Succeed())
			for range 2 {
				res, err := r.Reconcile(ctx, reconcile.Request{NamespacedName: key})
				Expect(err).NotTo(HaveOccurred())
				if !arrived {
					Expect(res.RequeueAfter).To(BeNumerically(">", 0))
					Expect(receiver().Finalizers).To(ContainElement(FinalizerReceiverCleanup))
				}
				Expect(cl.Get(ctx, key, &v1alpha2.VirtualMachine{})).To(Succeed())
				Expect(cl.Get(ctx, client.ObjectKeyFromObject(disk), &v1alpha2.VirtualDisk{})).To(Succeed())
			}
		},
		Entry("matching migration succeeded", &virtv1.VirtualMachineInstanceMigration{
			ObjectMeta: metav1.ObjectMeta{Namespace: ns, Name: "uid-1", UID: "migration-uid"},
			Spec:       virtv1.VirtualMachineInstanceMigrationSpec{VMIName: "web", Receive: &virtv1.VirtualMachineInstanceMigrationTarget{MigrationID: "uid-1"}},
			Status:     virtv1.VirtualMachineInstanceMigrationStatus{Phase: virtv1.MigrationSucceeded},
		}, nil, true),
		Entry("matching migration is running", &virtv1.VirtualMachineInstanceMigration{
			ObjectMeta: metav1.ObjectMeta{Namespace: ns, Name: "uid-1"},
			Spec:       virtv1.VirtualMachineInstanceMigrationSpec{VMIName: "web", Receive: &virtv1.VirtualMachineInstanceMigrationTarget{MigrationID: "uid-1"}},
			Status:     virtv1.VirtualMachineInstanceMigrationStatus{Phase: virtv1.MigrationRunning},
		}, nil, false),
		Entry("transient completed migration state while still running", &virtv1.VirtualMachineInstanceMigration{
			ObjectMeta: metav1.ObjectMeta{Namespace: ns, Name: "uid-1"},
			Spec:       virtv1.VirtualMachineInstanceMigrationSpec{VMIName: "web", Receive: &virtv1.VirtualMachineInstanceMigrationTarget{MigrationID: "uid-1"}},
			Status:     virtv1.VirtualMachineInstanceMigrationStatus{Phase: virtv1.MigrationRunning, MigrationState: &virtv1.VirtualMachineInstanceMigrationState{Completed: true}},
		}, nil, false),
		Entry("migration identity is ambiguous", &virtv1.VirtualMachineInstanceMigration{
			ObjectMeta: metav1.ObjectMeta{Namespace: ns, Name: "uid-1"},
			Spec:       virtv1.VirtualMachineInstanceMigrationSpec{VMIName: "other", Receive: &virtv1.VirtualMachineInstanceMigrationTarget{MigrationID: "wrong"}},
		}, nil, false),
		Entry("unrelated succeeded migration", &virtv1.VirtualMachineInstanceMigration{
			ObjectMeta: metav1.ObjectMeta{Namespace: ns, Name: "uid-1"},
			Spec:       virtv1.VirtualMachineInstanceMigrationSpec{VMIName: "other", Receive: &virtv1.VirtualMachineInstanceMigrationTarget{MigrationID: "wrong"}},
			Status:     virtv1.VirtualMachineInstanceMigrationStatus{Phase: virtv1.MigrationSucceeded},
		}, nil, false),
		Entry("mismatched VMI migration UID", &virtv1.VirtualMachineInstanceMigration{
			ObjectMeta: metav1.ObjectMeta{Namespace: ns, Name: "uid-1", UID: "migration-uid"},
			Spec:       virtv1.VirtualMachineInstanceMigrationSpec{VMIName: "web", Receive: &virtv1.VirtualMachineInstanceMigrationTarget{MigrationID: "uid-1"}},
		}, &virtv1.VirtualMachineInstance{
			ObjectMeta: metav1.ObjectMeta{Namespace: ns, Name: "web"},
			Status:     virtv1.VirtualMachineInstanceStatus{MigrationState: &virtv1.VirtualMachineInstanceMigrationState{MigrationUID: "other-uid", Completed: true}},
		}, false),
		Entry("completed matching VMI before migration succeeds", &virtv1.VirtualMachineInstanceMigration{
			ObjectMeta: metav1.ObjectMeta{Namespace: ns, Name: "uid-1", UID: "migration-uid"},
			Spec:       virtv1.VirtualMachineInstanceMigrationSpec{VMIName: "web", Receive: &virtv1.VirtualMachineInstanceMigrationTarget{MigrationID: "uid-1"}},
		}, &virtv1.VirtualMachineInstance{
			ObjectMeta: metav1.ObjectMeta{Namespace: ns, Name: "web"},
			Status:     virtv1.VirtualMachineInstanceStatus{MigrationState: &virtv1.VirtualMachineInstanceMigrationState{MigrationUID: "migration-uid", Completed: true}},
		}, false),
		Entry("running VMI without migration", nil, &virtv1.VirtualMachineInstance{
			ObjectMeta: metav1.ObjectMeta{Namespace: ns, Name: "web"}, Status: virtv1.VirtualMachineInstanceStatus{Phase: virtv1.Running},
		}, false),
		Entry("completed VMI without migration identity", nil, &virtv1.VirtualMachineInstance{
			ObjectMeta: metav1.ObjectMeta{Namespace: ns, Name: "web"},
			Status:     virtv1.VirtualMachineInstanceStatus{MigrationState: &virtv1.VirtualMachineInstanceMigrationState{Completed: true}},
		}, false),
	)

	DescribeTable("cleans up a failed migration with a start timestamp", func(vmi *virtv1.VirtualMachineInstance) {
		now := metav1.Now()
		migration := &virtv1.VirtualMachineInstanceMigration{
			ObjectMeta: metav1.ObjectMeta{Namespace: ns, Name: "uid-1"},
			Spec:       virtv1.VirtualMachineInstanceMigrationSpec{VMIName: "web", Receive: &virtv1.VirtualMachineInstanceMigrationTarget{MigrationID: "uid-1"}},
			Status:     virtv1.VirtualMachineInstanceMigrationStatus{Phase: virtv1.MigrationFailed, MigrationState: &virtv1.VirtualMachineInstanceMigrationState{StartTimestamp: &now}},
		}
		rcvr := newReceiver()
		rcvr.Finalizers = []string{FinalizerReceiverCleanup}
		setup(rcvr, migration,
			&v1alpha2.VirtualMachine{ObjectMeta: metav1.ObjectMeta{Namespace: ns, Name: "web", Labels: map[string]string{annotations.LabelVMReceiver: "uid-1"}}},
			&v1alpha2.VirtualDisk{ObjectMeta: metav1.ObjectMeta{Namespace: ns, Name: "web-root", Labels: map[string]string{annotations.LabelVMReceiver: "uid-1"}}})
		if vmi != nil {
			Expect(cl.Create(ctx, vmi)).To(Succeed())
		}
		Expect(cl.Delete(ctx, receiver())).To(Succeed())
		for range 2 {
			reconcileOnce()
		}
		Expect(k8serrors.IsNotFound(cl.Get(ctx, key, &v1alpha2.VirtualMachine{}))).To(BeTrue())
		Expect(k8serrors.IsNotFound(cl.Get(ctx, types.NamespacedName{Namespace: ns, Name: "web-root"}, &v1alpha2.VirtualDisk{}))).To(BeTrue())
		Expect(k8serrors.IsNotFound(cl.Get(ctx, key, &intvirtv1alpha1.InternalVirtualizationVirtualMachineReceiver{}))).To(BeTrue())
	},
		Entry("and no VMI", nil),
		Entry("and a VMI the source marked failed but not completed", &virtv1.VirtualMachineInstance{
			ObjectMeta: metav1.ObjectMeta{Namespace: ns, Name: "web"},
			Status:     virtv1.VirtualMachineInstanceStatus{Phase: virtv1.Scheduled, MigrationState: &virtv1.VirtualMachineInstanceMigrationState{Failed: true}},
		}),
	)

	It("defers idle expiry when a VMI may be running", func() {
		idle := newReceiver()
		idle.CreationTimestamp = metav1.NewTime(time.Now().Add(-receiverIdleTTL - time.Minute))
		idle.Status.Phase = intvirtv1alpha1.ReceiverReady
		setup(idle, &virtv1.VirtualMachineInstance{ObjectMeta: metav1.ObjectMeta{Namespace: ns, Name: "web"}, Status: virtv1.VirtualMachineInstanceStatus{Phase: virtv1.Running}},
			&v1alpha2.VirtualMachine{ObjectMeta: metav1.ObjectMeta{Namespace: ns, Name: "web", Labels: map[string]string{annotations.LabelVMReceiver: "uid-1"}}})
		reconcileOnce()
		Expect(cl.Get(ctx, key, &v1alpha2.VirtualMachine{})).To(Succeed())
		Expect(cl.Get(ctx, key, &intvirtv1alpha1.InternalVirtualizationVirtualMachineReceiver{})).To(Succeed())
	})

	It("removes a request the source left behind, once its purpose is served", func() {
		old := newReceiver()
		old.CreationTimestamp = metav1.NewTime(time.Now().Add(-receiverRunningTTL - time.Minute))
		old.Status.Phase = intvirtv1alpha1.ReceiverRunning
		old.Status.ReceiverStarted = true
		setup(old, &v1alpha2.VirtualMachine{ObjectMeta: metav1.ObjectMeta{Namespace: ns, Name: "web"}})
		reconcileOnce() // deletes the request
		reconcileOnce() // lets its finalizer go
		Expect(k8serrors.IsNotFound(cl.Get(ctx, key, &intvirtv1alpha1.InternalVirtualizationVirtualMachineReceiver{}))).To(BeTrue())
		Expect(cl.Get(ctx, key, &v1alpha2.VirtualMachine{})).To(Succeed(), "the VM that ran here stays")

		fresh := newReceiver()
		fresh.CreationTimestamp = metav1.Now()
		fresh.Status.Phase = intvirtv1alpha1.ReceiverFailed
		setup(fresh)
		res, err := r.Reconcile(ctx, reconcile.Request{NamespacedName: key})
		Expect(err).NotTo(HaveOccurred())
		Expect(res.RequeueAfter).To(BeNumerically(">", 0), "a failed request waits for the source to read why")
		Expect(cl.Get(ctx, key, &intvirtv1alpha1.InternalVirtualizationVirtualMachineReceiver{})).To(Succeed())
	})

	It("removes what it created when the VM has not run here, and keeps a VM that has", func() {
		setup(newReceiver())
		reconcileOnce()
		Expect(cl.Delete(ctx, receiver())).To(Succeed())
		reconcileOnce()
		reconcileOnce()
		Expect(k8serrors.IsNotFound(cl.Get(ctx, key, &v1alpha2.VirtualMachine{}))).To(BeTrue())
		Expect(k8serrors.IsNotFound(cl.Get(ctx, types.NamespacedName{Namespace: ns, Name: "web-root"}, &v1alpha2.VirtualDisk{}))).To(BeTrue())
		Expect(k8serrors.IsNotFound(cl.Get(ctx, key, &intvirtv1alpha1.InternalVirtualizationVirtualMachineReceiver{}))).To(BeTrue())

		setup(newReceiver())
		reconcileOnce()
		rcvr := receiver()
		Expect(PatchReceiverStatus(ctx, cl, rcvr, func(st *intvirtv1alpha1.VirtualMachineReceiverStatus) {
			st.Phase, st.ReceiverStarted = intvirtv1alpha1.ReceiverRunning, true
		})).To(Succeed())
		Expect(cl.Delete(ctx, receiver())).To(Succeed())
		reconcileOnce()
		Expect(cl.Get(ctx, key, &v1alpha2.VirtualMachine{})).To(Succeed())
		Expect(k8serrors.IsNotFound(cl.Get(ctx, key, &intvirtv1alpha1.InternalVirtualizationVirtualMachineReceiver{}))).To(BeTrue())
	})

	It("removes a request refused for the name, and keeps the running VM of that name", func() {
		rcvr := newReceiver()
		rcvr.Finalizers = []string{FinalizerReceiverCleanup}
		rcvr.Status.Phase = intvirtv1alpha1.ReceiverFailed
		setup(rcvr,
			&v1alpha2.VirtualMachine{ObjectMeta: metav1.ObjectMeta{Namespace: ns, Name: "web"}},
			&virtv1.VirtualMachineInstance{ObjectMeta: metav1.ObjectMeta{Namespace: ns, Name: "web"}, Status: virtv1.VirtualMachineInstanceStatus{Phase: virtv1.Running}})
		Expect(cl.Delete(ctx, receiver())).To(Succeed())
		reconcileOnce()
		Expect(k8serrors.IsNotFound(cl.Get(ctx, key, &intvirtv1alpha1.InternalVirtualizationVirtualMachineReceiver{}))).To(BeTrue())
		Expect(cl.Get(ctx, key, &v1alpha2.VirtualMachine{})).To(Succeed())
	})

	It("counts a failed request from the phase, not from its creation", func() {
		old := newReceiver()
		old.CreationTimestamp = metav1.NewTime(time.Now().Add(-2 * receiverFailedTTL))
		old.Annotations = map[string]string{annotations.AnnReceiverPhaseSince: time.Now().Add(-time.Minute).UTC().Format(time.RFC3339)}
		old.Status.Phase = intvirtv1alpha1.ReceiverFailed
		setup(old)

		res, err := r.Reconcile(ctx, reconcile.Request{NamespacedName: key})
		Expect(err).NotTo(HaveOccurred())
		Expect(res.RequeueAfter).To(BeNumerically(">", 0), "the source still has time to read the reason")
		Expect(cl.Get(ctx, key, &intvirtv1alpha1.InternalVirtualizationVirtualMachineReceiver{})).To(Succeed())
	})
})

var _ = Describe("ReceiverIPAddress", func() {
	It("keeps the address of the source unless the request asks for one of this cluster", func() {
		r := &intvirtv1alpha1.InternalVirtualizationVirtualMachineReceiver{Spec: intvirtv1alpha1.VirtualMachineReceiverSpec{IPAddress: "10.66.10.5"}}
		Expect(ReceiverIPAddress(r)).To(Equal("10.66.10.5"))
		r.Annotations = map[string]string{annotations.AnnReceiverNewIPAddress: "true"}
		Expect(ReceiverIPAddress(r)).To(BeEmpty())
		Expect(ReceiverIPAddress(nil)).To(BeEmpty())
	})
})
