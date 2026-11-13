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
	"errors"
	"fmt"
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
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"
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
			// What the request has created here, if anything: the tests that need its objects create them
			// with these UIDs.
			Status: intvirtv1alpha1.VirtualMachineReceiverStatus{
				VirtualMachineUID: "vm-1",
				VirtualDiskUIDs:   map[string]types.UID{"web-root": "vd-1"},
			},
		}
	}
	// newReceiverNotCreated is a request that has created nothing here yet. newReceiver records the objects
	// of a request that has: a request whose recorded VM is gone fails the receive instead of creating it again.
	newReceiverNotCreated := func() *intvirtv1alpha1.InternalVirtualizationVirtualMachineReceiver {
		rcvr := newReceiver()
		rcvr.Status = intvirtv1alpha1.VirtualMachineReceiverStatus{}

		return rcvr
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

		created := 0
		cl = fake.NewClientBuilder().WithScheme(scheme).WithObjects(objs...).
			WithStatusSubresource(&intvirtv1alpha1.InternalVirtualizationVirtualMachineReceiver{}, &v1alpha2.VirtualDisk{}).
			// The API server gives a created object its UID and creation time; the request tells its objects by them.
			WithInterceptorFuncs(interceptor.Funcs{Create: func(ctx context.Context, c client.WithWatch, obj client.Object, opts ...client.CreateOption) error {
				created++
				obj.SetUID(types.UID(fmt.Sprintf("created-%d", created)))
				obj.SetCreationTimestamp(metav1.Now())

				return c.Create(ctx, obj, opts...)
			}}).Build()
		r = &ReceiverReconciler{client: cl, apiReader: cl, namespace: "d8-virtualization"}
	}

	It("publishes the address of this cluster and hands the pin of the source to the receiving migration", func() {
		rcvr := newReceiver()
		rcvr.Spec.SourceSynchronization.SyncPin = "sha256:A"
		receiving := &virtv1.VirtualMachineInstanceMigration{ObjectMeta: metav1.ObjectMeta{Namespace: ns, Name: "uid-1"}, Spec: virtv1.VirtualMachineInstanceMigrationSpec{Receive: &virtv1.VirtualMachineInstanceMigrationTarget{MigrationID: "uid-1"}}}
		setup(rcvr, receiving)

		kv := &virtv1.KubeVirt{}
		Expect(cl.Get(ctx, types.NamespacedName{Namespace: "d8-virtualization", Name: "config"}, kv)).To(Succeed())
		kv.Status.SynchronizationAddresses = []string{"172.17.9.10:9185"}
		Expect(cl.Update(ctx, kv)).To(Succeed())
		reconcileOnce()
		Expect(receiver().Status.Synchronization.SyncAddress).To(Equal("172.17.9.10:9185"))
		Expect(cl.Get(ctx, client.ObjectKeyFromObject(receiving), receiving)).To(Succeed())
		Expect(receiving.Annotations).To(HaveKeyWithValue(migrationPeerSyncPinAnnotation, "sha256:A"))
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

	It("creates the disks and the VM of the request owned by it and records them, labelling nothing", func() {
		setup(newReceiverNotCreated())
		reconcileOnce()
		Expect(receiver().Status.Phase).To(Equal(intvirtv1alpha1.ReceiverPending))

		var vd v1alpha2.VirtualDisk
		Expect(cl.Get(ctx, types.NamespacedName{Namespace: ns, Name: "web-root"}, &vd)).To(Succeed())
		// No label or annotation of the module: the reference to the request tells the disk, and the disk
		// keeps the serial of the source.
		Expect(vd.Labels).To(Equal(map[string]string{"tier": "db"}))
		Expect(vd.Annotations).To(BeEmpty())
		Expect(vd.Status.Serial).To(Equal("3f2a"))
		Expect(receiver().Status.VirtualDiskUIDs).To(HaveKeyWithValue("web-root", vd.UID))
		Expect(*vd.Spec.PersistentVolumeClaim.StorageClass).To(Equal("local"))
		Expect(vd.Spec.PersistentVolumeClaim.Size.String()).To(Equal("4Gi"))
		Expect(vd.OwnerReferences).To(Equal(receiverOwnerReferences(receiver())))

		var vm v1alpha2.VirtualMachine
		Expect(cl.Get(ctx, key, &vm)).To(Succeed())
		Expect(vm.Labels).To(Equal(map[string]string{"app": "web"}))
		Expect(vm.Annotations).To(Equal(map[string]string{"owner": "team-a"}))
		Expect(receiver().Status.VirtualMachineUID).To(Equal(vm.UID))
		Expect(vm.Spec.NodeSelector).To(Equal(map[string]string{"zone": "a"}))
		Expect(vm.Spec.PriorityClassName).To(Equal("high"))
		// The address object of the source means nothing here.
		Expect(vm.Spec.VirtualMachineIPAddress).To(BeEmpty())
		Expect(vm.OwnerReferences).To(Equal(receiverOwnerReferences(receiver())))
		// The reference does not block the deletion of the request: blocking needs the right to its finalizers.
		Expect(vm.OwnerReferences[0].BlockOwnerDeletion).To(BeNil())
		Expect(*vm.OwnerReferences[0].Controller).To(BeTrue())
		Expect(vm.OwnerReferences[0].UID).To(Equal(types.UID("uid-1")))
	})

	It("takes a free address object with the address of the source", func() {
		setup(newReceiverNotCreated(),
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

			setup(append(objs, newReceiverNotCreated())...)

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
			setup(obj, newReceiverNotCreated())
			Expect(cl.Delete(ctx, obj)).To(Succeed())
			reconcileOnce()
			Expect(receiver().Status.Phase).To(Equal(intvirtv1alpha1.ReceiverPending))
		},
		Entry("VM", &v1alpha2.VirtualMachine{ObjectMeta: metav1.ObjectMeta{Namespace: ns, Name: "web"}}),
		Entry("disk", &v1alpha2.VirtualDisk{ObjectMeta: metav1.ObjectMeta{Namespace: ns, Name: "web-root"}}),
	)

	DescribeTable("compares the volume mode of a disk with the one of the source",
		func(source string, phase intvirtv1alpha1.VirtualMachineReceiverPhase) {
			rcvr := newReceiverNotCreated()
			rcvr.Spec.InternalVirtualizationVirtualMachineInstance.Raw = []byte(`{"spec":{"domain":{"devices":{"disks":[{"name":"vd-web-root"}]}}},` +
				`"status":{"volumeStatus":[{"name":"vd-web-root","persistentVolumeClaimInfo":{"claimName":"pvc-a"` + source + `}}]}}`)
			setup(rcvr)
			// The StorageClass of this cluster gives filesystem volumes.
			var sc storagev1.StorageClass
			Expect(cl.Get(ctx, types.NamespacedName{Name: "local"}, &sc)).To(Succeed())
			sc.Annotations = map[string]string{annotations.AnnVirtualDiskVolumeMode: "Filesystem", annotations.AnnVirtualDiskAccessMode: "ReadWriteOnce"}
			Expect(cl.Update(ctx, &sc)).To(Succeed())
			reconcileOnce()
			Expect(receiver().Status.Phase).To(Equal(phase))

			if phase == intvirtv1alpha1.ReceiverFailed {
				Expect(receiver().Status.Reason).To(Equal(ReceiverReasonVolumeModeMismatch))
				Expect(receiver().Status.Message).To(ContainSubstring(`StorageClass "local" of the disk "web-root" gives Filesystem volumes there and Block volumes in the source cluster`))
				Expect(k8serrors.IsNotFound(cl.Get(ctx, types.NamespacedName{Namespace: ns, Name: "web-root"}, &v1alpha2.VirtualDisk{}))).To(BeTrue())
			}
		},
		Entry("refuses a block disk for a filesystem class", `,"volumeMode":"Block"`, intvirtv1alpha1.ReceiverFailed),
		Entry("takes the same mode", `,"volumeMode":"Filesystem"`, intvirtv1alpha1.ReceiverPending),
		Entry("takes a disk of a source that does not report the mode", ``, intvirtv1alpha1.ReceiverPending),
	)

	It("refuses a VM whose provisioning Secret is not in the project, and takes it once the Secret is there", func() {
		withSecret := func() *intvirtv1alpha1.InternalVirtualizationVirtualMachineReceiver {
			rcvr := newReceiverNotCreated()
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
		setup(idle, &v1alpha2.VirtualMachine{ObjectMeta: metav1.ObjectMeta{Namespace: ns, Name: "web", UID: "vm-1"}})
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
		setup(receiving, vmim, &v1alpha2.VirtualMachine{ObjectMeta: metav1.ObjectMeta{Namespace: ns, Name: "web", UID: "vm-1"}})
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
		setup(idle, vmim, vmi, &v1alpha2.VirtualMachine{ObjectMeta: metav1.ObjectMeta{Namespace: ns, Name: "web", UID: "vm-1"}})
		reconcileOnce()
		Expect(k8serrors.IsNotFound(cl.Get(ctx, key, &intvirtv1alpha1.InternalVirtualizationVirtualMachineReceiver{})) || !receiver().DeletionTimestamp.IsZero()).To(BeTrue())
	})

	DescribeTable("defers cleanup when arrival or activity cannot be ruled out",
		func(migration *virtv1.VirtualMachineInstanceMigration, vmi *virtv1.VirtualMachineInstance, arrived bool) {
			rcvr := newReceiver()
			rcvr.Finalizers = []string{FinalizerReceiverCleanup}
			vm := &v1alpha2.VirtualMachine{ObjectMeta: metav1.ObjectMeta{Namespace: ns, Name: "web", UID: "vm-1"}}
			disk := &v1alpha2.VirtualDisk{ObjectMeta: metav1.ObjectMeta{Namespace: ns, Name: "web-root", UID: "vd-1"}}

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
			&v1alpha2.VirtualMachine{ObjectMeta: metav1.ObjectMeta{Namespace: ns, Name: "web", UID: "vm-1"}},
			&v1alpha2.VirtualDisk{ObjectMeta: metav1.ObjectMeta{Namespace: ns, Name: "web-root", UID: "vd-1"}})

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
			&v1alpha2.VirtualMachine{ObjectMeta: metav1.ObjectMeta{Namespace: ns, Name: "web", UID: "vm-1"}})
		reconcileOnce()
		Expect(cl.Get(ctx, key, &v1alpha2.VirtualMachine{})).To(Succeed())
		Expect(cl.Get(ctx, key, &intvirtv1alpha1.InternalVirtualizationVirtualMachineReceiver{})).To(Succeed())
	})

	Describe("deleted after the guest arrived", func() {
		arrived := func() []client.Object {
			rcvr := newReceiver()
			rcvr.Finalizers = []string{FinalizerReceiverCleanup}
			rcvr.Status = intvirtv1alpha1.VirtualMachineReceiverStatus{
				Phase: intvirtv1alpha1.ReceiverReady, ReceiverStarted: true,
				VirtualMachineUID: "vm-1", VirtualDiskUIDs: map[string]types.UID{"web-root": "vd-1"},
			}
			refs := receiverOwnerReferences(rcvr)

			return []client.Object{
				rcvr,
				&v1alpha2.VirtualMachine{ObjectMeta: metav1.ObjectMeta{Namespace: ns, Name: "web", UID: "vm-1", OwnerReferences: refs}},
				&v1alpha2.VirtualDisk{ObjectMeta: metav1.ObjectMeta{Namespace: ns, Name: "web-root", UID: "vd-1", OwnerReferences: refs}},
			}
		}

		// The source deletes the request as soon as the guest arrived, before the VM controller here released
		// the objects: the garbage collector would follow a reference left once the request is gone.
		It("releases its objects before it lets its finalizer go", func() {
			setup(arrived()...)
			Expect(cl.Delete(ctx, receiver())).To(Succeed())
			reconcileOnce()

			Expect(k8serrors.IsNotFound(cl.Get(ctx, key, &intvirtv1alpha1.InternalVirtualizationVirtualMachineReceiver{}))).To(BeTrue())

			var vm v1alpha2.VirtualMachine
			Expect(cl.Get(ctx, key, &vm)).To(Succeed(), "the received VM stays")
			Expect(vm.OwnerReferences).To(BeEmpty())

			var vd v1alpha2.VirtualDisk
			Expect(cl.Get(ctx, types.NamespacedName{Namespace: ns, Name: "web-root"}, &vd)).To(Succeed(), "the received disk stays")
			Expect(vd.OwnerReferences).To(BeEmpty())
		})

		DescribeTable("keeps the receiver until the KVVM is finalized",
			func(recorded bool) {
				objs := arrived()

				rcvr := objs[0].(*intvirtv1alpha1.InternalVirtualizationVirtualMachineReceiver)
				if recorded {
					rcvr.Status.InternalVirtualMachineUID = "kvvm-1"
				}

				vm := objs[1].(*v1alpha2.VirtualMachine)
				strategy := virtv1.RunStrategyWaitAsReceiver
				kvvm := &virtv1.VirtualMachine{
					ObjectMeta: metav1.ObjectMeta{Namespace: ns, Name: "web", UID: "kvvm-1", OwnerReferences: []metav1.OwnerReference{*metav1.NewControllerRef(vm, v1alpha2.SchemeGroupVersion.WithKind(v1alpha2.VirtualMachineKind))}},
					Spec:       virtv1.VirtualMachineSpec{RunStrategy: &strategy},
				}
				setup(append(objs, kvvm)...)
				Expect(cl.Delete(ctx, receiver())).To(Succeed())
				reconcileOnce()
				Expect(receiver().Finalizers).To(ContainElement(FinalizerReceiverCleanup))

				var gotVM v1alpha2.VirtualMachine
				Expect(cl.Get(ctx, key, &gotVM)).To(Succeed())
				Expect(gotVM.OwnerReferences).NotTo(BeEmpty())

				rcvr = receiver()
				Expect(PatchReceiverStatus(ctx, cl, rcvr, func(st *intvirtv1alpha1.VirtualMachineReceiverStatus) { st.Phase = intvirtv1alpha1.ReceiverRunning })).To(Succeed())
				reconcileOnce()
				Expect(k8serrors.IsNotFound(cl.Get(ctx, key, &intvirtv1alpha1.InternalVirtualizationVirtualMachineReceiver{}))).To(BeTrue())
			},
			Entry("after recording its UID", true),
			Entry("after Create but before recording its UID", false),
		)

		It("keeps its finalizer while it cannot release an object", func() {
			setup(arrived()...)
			Expect(cl.Delete(ctx, receiver())).To(Succeed())
			r.client = interceptor.NewClient(cl.(client.WithWatch), interceptor.Funcs{
				Patch: func(ctx context.Context, c client.WithWatch, obj client.Object, patch client.Patch, opts ...client.PatchOption) error {
					if _, ok := obj.(*v1alpha2.VirtualMachine); ok {
						return errors.New("conflict")
					}

					return c.Patch(ctx, obj, patch, opts...)
				},
			})
			_, err := r.Reconcile(ctx, reconcile.Request{NamespacedName: key})
			Expect(err).To(HaveOccurred())
			Expect(receiver().Finalizers).To(ContainElement(FinalizerReceiverCleanup))
		})
	})

	DescribeTable("retires a retained arrival record after its VM leaves", func(replaced bool) {
		rcvr := newReceiver()
		rcvr.CreationTimestamp = metav1.Now()
		rcvr.Status.Phase, rcvr.Status.ReceiverStarted = intvirtv1alpha1.ReceiverRunning, true
		disk := &v1alpha2.VirtualDisk{ObjectMeta: metav1.ObjectMeta{
			Namespace: ns, Name: "web-root", UID: "vd-1", OwnerReferences: receiverOwnerReferences(rcvr),
		}}

		objs := []client.Object{rcvr, disk}
		if replaced {
			objs = append(objs, &v1alpha2.VirtualMachine{ObjectMeta: metav1.ObjectMeta{Namespace: ns, Name: "web", UID: "new-vm"}})
		}

		setup(objs...)
		reconcileOnce()
		reconcileOnce()
		Expect(k8serrors.IsNotFound(cl.Get(ctx, key, &intvirtv1alpha1.InternalVirtualizationVirtualMachineReceiver{}))).To(BeTrue())

		if replaced {
			var vm v1alpha2.VirtualMachine
			Expect(cl.Get(ctx, key, &vm)).To(Succeed())
			Expect(vm.UID).To(Equal(types.UID("new-vm")))
		}

		var keptDisk v1alpha2.VirtualDisk
		Expect(cl.Get(ctx, client.ObjectKeyFromObject(disk), &keptDisk)).To(Succeed())
		Expect(keptDisk.UID).To(Equal(disk.UID))
		Expect(keptDisk.OwnerReferences).To(BeEmpty(), "completed receive cleanup releases storage instead of deleting it")

		fresh := newReceiver()
		Expect(cl.Create(ctx, fresh)).To(Succeed(), "the VM name is available to another receive without waiting for the daily TTL")
	}, Entry("VM moved away", false), Entry("VM name was reused", true))

	It("keeps the arrival record when only the informer misses the received VM", func() {
		rcvr := newReceiver()
		rcvr.CreationTimestamp = metav1.Now()
		rcvr.Status.Phase, rcvr.Status.ReceiverStarted = intvirtv1alpha1.ReceiverRunning, true
		setup(rcvr)

		live := fake.NewClientBuilder().WithScheme(newScheme()).WithObjects(
			&v1alpha2.VirtualMachine{ObjectMeta: metav1.ObjectMeta{Namespace: ns, Name: "web", UID: "vm-1"}},
		).Build()
		r.apiReader = live
		res, err := r.Reconcile(ctx, reconcile.Request{NamespacedName: key})
		Expect(err).NotTo(HaveOccurred())
		Expect(receiver().DeletionTimestamp).To(BeNil())
		Expect(res.RequeueAfter).To(BeNumerically("<=", receiverExchangeResync))
	})

	It("keeps the arrival record when the live VM lookup fails", func() {
		rcvr := newReceiver()
		rcvr.Status.Phase, rcvr.Status.ReceiverStarted = intvirtv1alpha1.ReceiverRunning, true
		setup(rcvr)

		r.apiReader = interceptor.NewClient(cl.(client.WithWatch), interceptor.Funcs{Get: func(context.Context, client.WithWatch, client.ObjectKey, client.Object, ...client.GetOption) error {
			return fmt.Errorf("API unavailable")
		}})
		_, err := r.Reconcile(ctx, reconcile.Request{NamespacedName: key})
		Expect(err).To(MatchError("API unavailable"))
		Expect(receiver().DeletionTimestamp).To(BeNil())
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

	It("takes the VM it created before a restart of the controller could record it", func() {
		rcvr := newReceiver()
		rcvr.Status = intvirtv1alpha1.VirtualMachineReceiverStatus{Phase: intvirtv1alpha1.ReceiverPending}
		vm := &v1alpha2.VirtualMachine{
			ObjectMeta: metav1.ObjectMeta{Namespace: ns, Name: "web", UID: "vm-7", OwnerReferences: receiverOwnerReferences(rcvr)},
			Spec:       v1alpha2.VirtualMachineSpec{BlockDeviceRefs: []v1alpha2.BlockDeviceSpecRef{{Kind: v1alpha2.DiskDevice, Name: "web-root"}}},
		}
		setup(rcvr, vm)
		reconcileOnce()
		Expect(receiver().Status.Phase).To(Equal(intvirtv1alpha1.ReceiverPending))
		Expect(receiver().Status.VirtualMachineUID).To(Equal(types.UID("vm-7")))
	})

	It("fails when the VM it created is deleted before the guest arrives", func() {
		rcvr := newReceiver()
		rcvr.Status.Phase = intvirtv1alpha1.ReceiverReady
		now := metav1.Now()
		vm := &v1alpha2.VirtualMachine{ObjectMeta: metav1.ObjectMeta{
			Namespace: ns, Name: "web", UID: "vm-1", DeletionTimestamp: &now, Finalizers: []string{"test"},
		}}
		setup(rcvr, vm)
		reconcileOnce()
		Expect(receiver().Status.Phase).To(Equal(intvirtv1alpha1.ReceiverFailed))
		Expect(receiver().Status.Reason).To(Equal(ReceiverReasonVirtualMachineDeleted))
		Expect(receiver().Status.Message).To(Equal(`the receiving VirtualMachine "web" was deleted`))

		// Once the guest is here, the deletion is the user's own business with a regular VM.
		arrived := newReceiver()
		arrived.Status.Phase = intvirtv1alpha1.ReceiverReady
		arrived.Status.ReceiverStarted = true
		setup(arrived, vm.DeepCopy())
		reconcileOnce()
		Expect(receiver().Status.Phase).NotTo(Equal(intvirtv1alpha1.ReceiverFailed))
	})

	It("fails when the VM it created is gone before the guest arrives", func() {
		rcvr := newReceiver()
		rcvr.Status.Phase = intvirtv1alpha1.ReceiverReady
		setup(rcvr)
		reconcileOnce()
		Expect(receiver().Status.Phase).To(Equal(intvirtv1alpha1.ReceiverFailed))
		Expect(receiver().Status.Reason).To(Equal(ReceiverReasonVirtualMachineDeleted))
		Expect(receiver().Status.Message).To(Equal(`the receiving VirtualMachine "web" was deleted`))

		var vm v1alpha2.VirtualMachine
		Expect(k8serrors.IsNotFound(cl.Get(ctx, key, &vm))).To(BeTrue(), "the VM is not created again")
	})

	// The user may create a VM of the same name with the very disks the request has just created: without the
	// reference it is the user's, and the move fails instead of receiving the guest into it.
	It("refuses a VM of the user with the name and the disks of the request", func() {
		rcvr := newReceiver()
		rcvr.Status = intvirtv1alpha1.VirtualMachineReceiverStatus{Phase: intvirtv1alpha1.ReceiverPending}
		vm := &v1alpha2.VirtualMachine{
			ObjectMeta: metav1.ObjectMeta{Namespace: ns, Name: "web", UID: "vm-7", CreationTimestamp: metav1.Now()},
			Spec:       v1alpha2.VirtualMachineSpec{BlockDeviceRefs: []v1alpha2.BlockDeviceSpecRef{{Kind: v1alpha2.DiskDevice, Name: "web-root"}}},
		}
		setup(rcvr, vm)
		reconcileOnce()
		Expect(receiver().Status.Phase).To(Equal(intvirtv1alpha1.ReceiverFailed))
		Expect(receiver().Status.Reason).To(Equal(ReceiverReasonVirtualMachineExists))
		Expect(receiver().Status.VirtualMachineUID).To(BeEmpty())
	})

	It("does not delete a disk of the user that has the name of a disk of the request", func() {
		rcvr := newReceiver()
		rcvr.Finalizers = []string{FinalizerReceiverCleanup}
		user := &v1alpha2.VirtualDisk{ObjectMeta: metav1.ObjectMeta{Namespace: ns, Name: "web-root", UID: "user-disk"}}
		setup(rcvr, user)
		Expect(cl.Delete(ctx, receiver())).To(Succeed())
		reconcileOnce()
		Expect(cl.Get(ctx, client.ObjectKeyFromObject(user), &v1alpha2.VirtualDisk{})).To(Succeed())
		Expect(k8serrors.IsNotFound(cl.Get(ctx, key, &intvirtv1alpha1.InternalVirtualizationVirtualMachineReceiver{}))).To(BeTrue())
	})

	It("removes what it created when the VM has not run here, and keeps a VM that has", func() {
		setup(newReceiverNotCreated())
		reconcileOnce()
		Expect(cl.Delete(ctx, receiver())).To(Succeed())
		reconcileOnce()
		reconcileOnce()
		Expect(k8serrors.IsNotFound(cl.Get(ctx, key, &v1alpha2.VirtualMachine{}))).To(BeTrue())
		Expect(k8serrors.IsNotFound(cl.Get(ctx, types.NamespacedName{Namespace: ns, Name: "web-root"}, &v1alpha2.VirtualDisk{}))).To(BeTrue())
		Expect(k8serrors.IsNotFound(cl.Get(ctx, key, &intvirtv1alpha1.InternalVirtualizationVirtualMachineReceiver{}))).To(BeTrue())

		setup(newReceiverNotCreated())
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

	It("persists completion time once and refreshes it when a failed receive later arrives", func() {
		setup(newReceiver())

		request := receiver()
		Expect(SetReceiverFailed(ctx, cl, request, ReceiverReasonMigrationFailed, "failed")).To(Succeed())

		first := receiver().Status.FinishedAt
		Expect(first).NotTo(BeNil())

		request = receiver()
		Expect(SetReceiverFailed(ctx, cl, request, ReceiverReasonMigrationFailed, "another cause")).To(Succeed())
		Expect(receiver().Status.FinishedAt).To(Equal(first))
		request = receiver()
		old := metav1.NewTime(time.Now().Add(-time.Hour))
		request.Status.FinishedAt = &old
		Expect(cl.Status().Update(ctx, request)).To(Succeed())
		Expect(PatchReceiverStatus(ctx, cl, request, func(st *intvirtv1alpha1.VirtualMachineReceiverStatus) { st.Phase = intvirtv1alpha1.ReceiverRunning })).To(Succeed())
		Expect(receiver().Status.FinishedAt.Time.After(old.Time)).To(BeTrue())
	})

	It("counts a failed request from the phase, not from its creation", func() {
		old := newReceiver()
		old.CreationTimestamp = metav1.NewTime(time.Now().Add(-2 * receiverFailedTTL))
		old.Status.FinishedAt = &metav1.Time{Time: time.Now().Add(-time.Minute)}
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

var _ = Describe("AppendReceiverProblems", func() {
	It("keeps the problems in the order they were seen, each once", func() {
		h := AppendReceiverProblems("", "no device")
		h = AppendReceiverProblems(h, "no device", "")
		h = AppendReceiverProblems(h, "connection refused", "no device")
		Expect(h).To(Equal("no device; then connection refused"))
		Expect(LatestReceiverProblem(h)).To(Equal("connection refused"))
		Expect(LatestReceiverProblem("no device")).To(Equal("no device"))
	})

	It("keeps the first problem and the latest ones of a long chain", func() {
		h := AppendReceiverProblems("", "p1", "p2", "p3", "p4", "p5")
		Expect(h).To(Equal("p1; then p2; then p3; then p4; then p5"))
		h = AppendReceiverProblems(h, "p6")
		Expect(h).To(Equal("p1; then …; then p4; then p5; then p6"))
		h = AppendReceiverProblems(h, "p7")
		Expect(h).To(Equal("p1; then …; then p5; then p6; then p7"))
	})
})

var _ = Describe("CreatedVirtualMachine and CreatedVirtualDisk", func() {
	request := func(vmUID types.UID, vdUIDs map[string]types.UID) *intvirtv1alpha1.InternalVirtualizationVirtualMachineReceiver {
		return &intvirtv1alpha1.InternalVirtualizationVirtualMachineReceiver{
			ObjectMeta: metav1.ObjectMeta{Name: "web", UID: "uid-1"},
			Status:     intvirtv1alpha1.VirtualMachineReceiverStatus{VirtualMachineUID: vmUID, VirtualDiskUIDs: vdUIDs},
		}
	}
	vm := func(uid, owner types.UID) *v1alpha2.VirtualMachine {
		v := &v1alpha2.VirtualMachine{ObjectMeta: metav1.ObjectMeta{Name: "web", UID: uid}}
		if owner != "" {
			v.OwnerReferences = []metav1.OwnerReference{{Kind: intvirtv1alpha1.InternalVirtualizationVirtualMachineReceiverKind, Name: "web", UID: owner}}
		}

		return v
	}
	vd := func(uid, owner types.UID) *v1alpha2.VirtualDisk {
		v := &v1alpha2.VirtualDisk{ObjectMeta: metav1.ObjectMeta{Name: "web-root", UID: uid}}
		if owner != "" {
			v.OwnerReferences = []metav1.OwnerReference{{Kind: intvirtv1alpha1.InternalVirtualizationVirtualMachineReceiverKind, Name: "web", UID: owner}}
		}

		return v
	}

	It("tells its objects by the reference to it, before it records them", func() {
		r := request("", nil)
		Expect(CreatedVirtualMachine(r, vm("vm-1", "uid-1"))).To(BeTrue())
		Expect(CreatedVirtualDisk(r, vd("vd-1", "uid-1"))).To(BeTrue())
	})

	It("tells its objects by the UIDs it recorded once the reference is gone", func() {
		r := request("vm-1", map[string]types.UID{"web-root": "vd-1"})
		Expect(CreatedVirtualMachine(r, vm("vm-1", ""))).To(BeTrue())
		Expect(CreatedVirtualDisk(r, vd("vd-1", ""))).To(BeTrue())
	})

	It("leaves the objects of the user and of another request alone", func() {
		r := request("", nil)
		Expect(CreatedVirtualMachine(r, vm("vm-1", ""))).To(BeFalse(), "no reference, nothing recorded")
		Expect(CreatedVirtualDisk(r, vd("vd-1", ""))).To(BeFalse(), "no reference, nothing recorded")
		Expect(CreatedVirtualMachine(r, vm("vm-1", "uid-0"))).To(BeFalse(), "the reference to an earlier request of the same name")
		Expect(CreatedVirtualDisk(r, vd("vd-1", "uid-0"))).To(BeFalse(), "the reference to an earlier request of the same name")

		recorded := request("vm-1", map[string]types.UID{"web-root": "vd-1"})
		Expect(CreatedVirtualMachine(recorded, vm("vm-2", ""))).To(BeFalse(), "another VM of the same name")
		Expect(CreatedVirtualDisk(recorded, vd("vd-2", ""))).To(BeFalse(), "another disk of the same name")
	})
})
