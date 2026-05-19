/*
Copyright 2025 Flant JSC

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

package state

import (
	"context"
	"log/slog"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	storagev1 "k8s.io/api/storage/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	apiruntime "k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	virtv1 "kubevirt.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/deckhouse/virtualization-controller/pkg/controller/indexer"
	"github.com/deckhouse/virtualization-controller/pkg/controller/reconciler"
	"github.com/deckhouse/virtualization-controller/pkg/logger"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/vdcondition"
)

func TestState(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "State Test Suite")
}

type StateTestArgs struct {
	specRefs   []v1alpha2.BlockDeviceSpecRef
	statusRefs []v1alpha2.BlockDeviceStatusRef
	uniqueRefs int
}

var _ = Describe("State fill check", func() {
	scheme := apiruntime.NewScheme()
	for _, f := range []func(*apiruntime.Scheme) error{
		v1alpha2.AddToScheme,
		virtv1.AddToScheme,
		corev1.AddToScheme,
	} {
		err := f(scheme)
		Expect(err).NotTo(HaveOccurred(), "failed to add scheme: %s", err)
	}

	namespacedName := types.NamespacedName{
		Namespace: "ns",
		Name:      "vm",
	}

	var ctx context.Context
	var vm *v1alpha2.VirtualMachine

	BeforeEach(func() {
		ctx = logger.ToContext(context.TODO(), slog.Default())

		vm = &v1alpha2.VirtualMachine{
			ObjectMeta: metav1.ObjectMeta{
				Name:      namespacedName.Name,
				Namespace: namespacedName.Namespace,
			},
			Spec: v1alpha2.VirtualMachineSpec{
				BlockDeviceRefs: []v1alpha2.BlockDeviceSpecRef{},
			},
			Status: v1alpha2.VirtualMachineStatus{
				Phase:           v1alpha2.MachinePending,
				BlockDeviceRefs: []v1alpha2.BlockDeviceStatusRef{},
			},
		}
	})

	DescribeTable("Checking Forbid events",
		func(args StateTestArgs) {
			vm.Spec.BlockDeviceRefs = args.specRefs
			vm.Status.BlockDeviceRefs = args.statusRefs

			fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(vm).Build()
			vmResource := reconciler.NewResource(namespacedName, fakeClient, vmFactoryByVM(vm), vmStatusGetter)

			err := vmResource.Fetch(ctx)
			Expect(err).NotTo(HaveOccurred())

			state := &state{client: fakeClient, vm: vmResource}

			state.fill()

			Expect(state.bdRefs).To(HaveLen(args.uniqueRefs))
		},
		Entry("Should has 3 refs; all non unique", StateTestArgs{
			uniqueRefs: 3,
			specRefs: []v1alpha2.BlockDeviceSpecRef{
				{Kind: v1alpha2.DiskDevice, Name: "vd1"},
				{Kind: v1alpha2.DiskDevice, Name: "vd2"},
				{Kind: v1alpha2.DiskDevice, Name: "vd3"},
			},
			statusRefs: []v1alpha2.BlockDeviceStatusRef{
				{Kind: v1alpha2.DiskDevice, Name: "vd1"},
				{Kind: v1alpha2.DiskDevice, Name: "vd2"},
				{Kind: v1alpha2.DiskDevice, Name: "vd3"},
			},
		}),
		Entry("Should has 3 refs; some of them are unique", StateTestArgs{
			uniqueRefs: 3,
			specRefs: []v1alpha2.BlockDeviceSpecRef{
				{Kind: v1alpha2.DiskDevice, Name: "vd2"},
				{Kind: v1alpha2.DiskDevice, Name: "vd3"},
			},
			statusRefs: []v1alpha2.BlockDeviceStatusRef{
				{Kind: v1alpha2.DiskDevice, Name: "vd1"},
				{Kind: v1alpha2.DiskDevice, Name: "vd2"},
			},
		}),
		Entry("Should has 5 refs; some of them have the different kind", StateTestArgs{
			uniqueRefs: 5,
			specRefs: []v1alpha2.BlockDeviceSpecRef{
				{Kind: v1alpha2.DiskDevice, Name: "vd2"},
				{Kind: v1alpha2.DiskDevice, Name: "vd3"},
				{Kind: v1alpha2.ImageDevice, Name: "vd3"},
			},
			statusRefs: []v1alpha2.BlockDeviceStatusRef{
				{Kind: v1alpha2.DiskDevice, Name: "vd1"},
				{Kind: v1alpha2.ClusterImageDevice, Name: "vd2"},
			},
		}),
	)
})

var _ = Describe("PVNodeAffinityTerms", func() {
	scheme := apiruntime.NewScheme()
	for _, f := range []func(*apiruntime.Scheme) error{
		v1alpha2.AddToScheme,
		virtv1.AddToScheme,
		corev1.AddToScheme,
		storagev1.AddToScheme,
	} {
		err := f(scheme)
		Expect(err).NotTo(HaveOccurred())
	}

	const (
		ns    = "test-ns"
		vmNm  = "test-vm"
		node1 = "node-1"
		node2 = "node-2"
		node3 = "node-3"
	)

	nodeAffinityTerm := func(nodes ...string) corev1.NodeSelectorTerm {
		return corev1.NodeSelectorTerm{
			MatchExpressions: []corev1.NodeSelectorRequirement{{
				Key:      "topology.kubernetes.io/node",
				Operator: corev1.NodeSelectorOpIn,
				Values:   nodes,
			}},
		}
	}

	makePV := func(name string, terms ...corev1.NodeSelectorTerm) *corev1.PersistentVolume {
		pv := &corev1.PersistentVolume{
			ObjectMeta: metav1.ObjectMeta{Name: name},
		}
		if len(terms) > 0 {
			pv.Spec.NodeAffinity = &corev1.VolumeNodeAffinity{
				Required: &corev1.NodeSelector{NodeSelectorTerms: terms},
			}
		}
		return pv
	}

	makePVC := func(name, pvName string) *corev1.PersistentVolumeClaim {
		return &corev1.PersistentVolumeClaim{
			ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: ns},
			Spec:       corev1.PersistentVolumeClaimSpec{VolumeName: pvName},
		}
	}

	makeVD := func(name, pvcName string) *v1alpha2.VirtualDisk {
		return &v1alpha2.VirtualDisk{
			ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: ns},
			Status:     v1alpha2.VirtualDiskStatus{Target: v1alpha2.DiskTarget{PersistentVolumeClaim: pvcName}},
		}
	}

	makeVI := func(name, pvcName string, storage v1alpha2.StorageType) *v1alpha2.VirtualImage {
		return &v1alpha2.VirtualImage{
			ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: ns},
			Spec:       v1alpha2.VirtualImageSpec{Storage: storage},
			Status:     v1alpha2.VirtualImageStatus{Target: v1alpha2.VirtualImageStatusTarget{PersistentVolumeClaim: pvcName}},
		}
	}

	buildState := func(vm *v1alpha2.VirtualMachine, objs ...client.Object) *state {
		allObjs := []client.Object{vm}
		allObjs = append(allObjs, objs...)
		vmbdaIndexObj, vmbdaIndexField, vmbdaIndexFn := indexer.IndexVMBDAByVM()
		fakeClient := fake.NewClientBuilder().
			WithScheme(scheme).
			WithObjects(allObjs...).
			WithIndex(vmbdaIndexObj, vmbdaIndexField, vmbdaIndexFn).
			Build()
		namespacedName := types.NamespacedName{Name: vm.Name, Namespace: vm.Namespace}
		vmResource := reconciler.NewResource(namespacedName, fakeClient, vmFactoryByVM(vm), vmStatusGetter)
		ctx := logger.ToContext(context.TODO(), slog.Default())
		err := vmResource.Fetch(ctx)
		Expect(err).NotTo(HaveOccurred())
		s := &state{client: fakeClient, vm: vmResource}
		s.fill()
		return s
	}

	makeVM := func(refs ...v1alpha2.BlockDeviceSpecRef) *v1alpha2.VirtualMachine {
		return &v1alpha2.VirtualMachine{
			ObjectMeta: metav1.ObjectMeta{Name: vmNm, Namespace: ns},
			Spec:       v1alpha2.VirtualMachineSpec{BlockDeviceRefs: refs},
			Status: v1alpha2.VirtualMachineStatus{
				Phase: v1alpha2.MachinePending,
			},
		}
	}

	It("should return nil when no block devices have PV nodeAffinity (network storage)", func() {
		vm := makeVM(v1alpha2.BlockDeviceSpecRef{Kind: v1alpha2.DiskDevice, Name: "net-disk"})
		vd := makeVD("net-disk", "pvc-net")
		pvc := makePVC("pvc-net", "pv-net")
		pv := makePV("pv-net") // no nodeAffinity

		s := buildState(vm, vd, pvc, pv)
		ctx := logger.ToContext(context.TODO(), slog.Default())
		terms, err := s.PVNodeAffinityTerms(ctx)
		Expect(err).NotTo(HaveOccurred())
		Expect(terms).To(BeNil())
	})

	It("should return PV nodeAffinity for a single local disk", func() {
		vm := makeVM(v1alpha2.BlockDeviceSpecRef{Kind: v1alpha2.DiskDevice, Name: "local-disk"})
		vd := makeVD("local-disk", "pvc-local")
		pvc := makePVC("pvc-local", "pv-local")
		pv := makePV("pv-local", nodeAffinityTerm(node1))

		s := buildState(vm, vd, pvc, pv)
		ctx := logger.ToContext(context.TODO(), slog.Default())
		terms, err := s.PVNodeAffinityTerms(ctx)
		Expect(err).NotTo(HaveOccurred())
		Expect(terms).To(HaveLen(1))
		Expect(terms[0].MatchExpressions).To(HaveLen(1))
		Expect(terms[0].MatchExpressions[0].Values).To(ConsistOf(node1))
	})

	It("should return intersection for multiple local disks on compatible nodes", func() {
		vm := makeVM(
			v1alpha2.BlockDeviceSpecRef{Kind: v1alpha2.DiskDevice, Name: "disk-a"},
			v1alpha2.BlockDeviceSpecRef{Kind: v1alpha2.DiskDevice, Name: "disk-b"},
		)
		vdA := makeVD("disk-a", "pvc-a")
		pvcA := makePVC("pvc-a", "pv-a")
		pvA := makePV("pv-a", nodeAffinityTerm(node1, node2))

		vdB := makeVD("disk-b", "pvc-b")
		pvcB := makePVC("pvc-b", "pv-b")
		pvB := makePV("pv-b", nodeAffinityTerm(node2, node3))

		s := buildState(vm, vdA, pvcA, pvA, vdB, pvcB, pvB)
		ctx := logger.ToContext(context.TODO(), slog.Default())
		terms, err := s.PVNodeAffinityTerms(ctx)
		Expect(err).NotTo(HaveOccurred())
		Expect(terms).To(HaveLen(1))
		Expect(terms[0].MatchExpressions).To(HaveLen(2))
	})

	It("should return only local PV terms when mixing local and network disks", func() {
		vm := makeVM(
			v1alpha2.BlockDeviceSpecRef{Kind: v1alpha2.DiskDevice, Name: "net-disk"},
			v1alpha2.BlockDeviceSpecRef{Kind: v1alpha2.DiskDevice, Name: "local-disk"},
		)
		vdNet := makeVD("net-disk", "pvc-net")
		pvcNet := makePVC("pvc-net", "pv-net")
		pvNet := makePV("pv-net") // no nodeAffinity

		vdLocal := makeVD("local-disk", "pvc-local")
		pvcLocal := makePVC("pvc-local", "pv-local")
		pvLocal := makePV("pv-local", nodeAffinityTerm(node2))

		s := buildState(vm, vdNet, pvcNet, pvNet, vdLocal, pvcLocal, pvLocal)
		ctx := logger.ToContext(context.TODO(), slog.Default())
		terms, err := s.PVNodeAffinityTerms(ctx)
		Expect(err).NotTo(HaveOccurred())
		Expect(terms).To(HaveLen(1))
		Expect(terms[0].MatchExpressions).To(HaveLen(1))
		Expect(terms[0].MatchExpressions[0].Values).To(ConsistOf(node2))
	})

	It("should skip unbound PVC when no available PVs match and SC is not local CSI", func() {
		vm := makeVM(
			v1alpha2.BlockDeviceSpecRef{Kind: v1alpha2.DiskDevice, Name: "bound-disk"},
			v1alpha2.BlockDeviceSpecRef{Kind: v1alpha2.DiskDevice, Name: "pending-disk"},
		)
		vdBound := makeVD("bound-disk", "pvc-bound")
		pvcBound := makePVC("pvc-bound", "pv-bound")
		pvBound := makePV("pv-bound", nodeAffinityTerm(node1))

		vdPending := makeVD("pending-disk", "pvc-pending")
		vdPending.Status.StorageClassName = "some-other-storage"
		pvcPending := &corev1.PersistentVolumeClaim{
			ObjectMeta: metav1.ObjectMeta{Name: "pvc-pending", Namespace: ns},
		}
		otherSC := &storagev1.StorageClass{
			ObjectMeta:  metav1.ObjectMeta{Name: "some-other-storage"},
			Provisioner: "some-other-provisioner",
		}

		s := buildState(vm, vdBound, pvcBound, pvBound, vdPending, pvcPending, otherSC)
		ctx := logger.ToContext(context.TODO(), slog.Default())
		terms, err := s.PVNodeAffinityTerms(ctx)
		Expect(err).NotTo(HaveOccurred())
		Expect(terms).To(HaveLen(1))
		Expect(terms[0].MatchExpressions[0].Values).To(ConsistOf(node1))
	})

	It("should derive node affinity from LVMVolumeGroup for local CSI dynamic provisioning", func() {
		vm := makeVM(v1alpha2.BlockDeviceSpecRef{Kind: v1alpha2.DiskDevice, Name: "local-disk"})
		vd := makeVD("local-disk", "pvc-local")
		vd.Status.StorageClassName = "local-storage-class-thin"
		pvcLocal := &corev1.PersistentVolumeClaim{
			ObjectMeta: metav1.ObjectMeta{Name: "pvc-local", Namespace: ns},
		}
		localSC := &storagev1.StorageClass{
			ObjectMeta:  metav1.ObjectMeta{Name: "local-storage-class-thin"},
			Provisioner: localCSIProvisioner,
			Parameters: map[string]string{
				lvmVolumeGroupsParam: "- name: vg-data-node-1\n  thin:\n    poolName: thin-data\n",
			},
		}
		lvg := &unstructured.Unstructured{}
		lvg.SetGroupVersionKind(schema.GroupVersionKind{
			Group: "storage.deckhouse.io", Version: "v1alpha1", Kind: "LVMVolumeGroup",
		})
		lvg.SetName("vg-data-node-1")
		_ = unstructured.SetNestedField(lvg.Object, node1, "spec", "local", "nodeName")

		s := buildState(vm, vd, pvcLocal, localSC, lvg)
		ctx := logger.ToContext(context.TODO(), slog.Default())
		terms, err := s.PVNodeAffinityTerms(ctx)
		Expect(err).NotTo(HaveOccurred())
		Expect(terms).To(HaveLen(1))
		Expect(terms[0].MatchExpressions).To(HaveLen(1))
		Expect(terms[0].MatchExpressions[0].Key).To(Equal(corev1.LabelHostname))
		Expect(terms[0].MatchExpressions[0].Values).To(ConsistOf(node1))
	})

	It("should intersect bound PV terms with LVMVolumeGroup topology", func() {
		vm := makeVM(
			v1alpha2.BlockDeviceSpecRef{Kind: v1alpha2.DiskDevice, Name: "bound-disk"},
			v1alpha2.BlockDeviceSpecRef{Kind: v1alpha2.DiskDevice, Name: "local-disk"},
		)
		vdBound := makeVD("bound-disk", "pvc-bound")
		pvcBound := makePVC("pvc-bound", "pv-bound")
		pvBound := makePV("pv-bound", nodeAffinityTerm(node1, node2))

		vdLocal := makeVD("local-disk", "pvc-local")
		vdLocal.Status.StorageClassName = "local-storage-class-thin"
		pvcLocal := &corev1.PersistentVolumeClaim{
			ObjectMeta: metav1.ObjectMeta{Name: "pvc-local", Namespace: ns},
		}
		localSC := &storagev1.StorageClass{
			ObjectMeta:  metav1.ObjectMeta{Name: "local-storage-class-thin"},
			Provisioner: localCSIProvisioner,
			Parameters: map[string]string{
				lvmVolumeGroupsParam: "- name: vg-data-node-1\n  thin:\n    poolName: thin-data\n",
			},
		}
		lvg := &unstructured.Unstructured{}
		lvg.SetGroupVersionKind(schema.GroupVersionKind{
			Group: "storage.deckhouse.io", Version: "v1alpha1", Kind: "LVMVolumeGroup",
		})
		lvg.SetName("vg-data-node-1")
		_ = unstructured.SetNestedField(lvg.Object, node1, "spec", "local", "nodeName")

		s := buildState(vm, vdBound, pvcBound, pvBound, vdLocal, pvcLocal, localSC, lvg)
		ctx := logger.ToContext(context.TODO(), slog.Default())
		terms, err := s.PVNodeAffinityTerms(ctx)
		Expect(err).NotTo(HaveOccurred())
		Expect(terms).NotTo(BeEmpty())
	})

	It("should collect node affinity from available PVs for unbound WFFC PVC", func() {
		vm := makeVM(v1alpha2.BlockDeviceSpecRef{Kind: v1alpha2.DiskDevice, Name: "wffc-disk"})
		vd := makeVD("wffc-disk", "pvc-wffc")
		vd.Status.StorageClassName = "local-storage"
		pvcWFFC := &corev1.PersistentVolumeClaim{
			ObjectMeta: metav1.ObjectMeta{Name: "pvc-wffc", Namespace: ns},
		}

		pvAvail1 := makePV("pv-avail-1", nodeAffinityTerm(node1))
		pvAvail1.Spec.StorageClassName = "local-storage"
		pvAvail1.Status.Phase = corev1.VolumeAvailable

		pvAvail2 := makePV("pv-avail-2", nodeAffinityTerm(node2))
		pvAvail2.Spec.StorageClassName = "local-storage"
		pvAvail2.Status.Phase = corev1.VolumeAvailable

		pvBound := makePV("pv-bound-other", nodeAffinityTerm(node3))
		pvBound.Spec.StorageClassName = "local-storage"
		pvBound.Status.Phase = corev1.VolumeBound

		s := buildState(vm, vd, pvcWFFC, pvAvail1, pvAvail2, pvBound)
		ctx := logger.ToContext(context.TODO(), slog.Default())
		terms, err := s.PVNodeAffinityTerms(ctx)
		Expect(err).NotTo(HaveOccurred())
		Expect(terms).To(HaveLen(2), "should have terms from 2 available PVs (not the bound one)")
	})

	It("should use VirtualDisk status storage class when PVC storage class is empty", func() {
		vm := makeVM(v1alpha2.BlockDeviceSpecRef{Kind: v1alpha2.DiskDevice, Name: "wffc-disk"})
		vd := makeVD("wffc-disk", "pvc-wffc")
		vd.Status.StorageClassName = "local-storage"
		pvcWFFC := &corev1.PersistentVolumeClaim{
			ObjectMeta: metav1.ObjectMeta{Name: "pvc-wffc", Namespace: ns},
		}

		pvAvail := makePV("pv-avail-1", nodeAffinityTerm(node2))
		pvAvail.Spec.StorageClassName = "local-storage"
		pvAvail.Status.Phase = corev1.VolumeAvailable

		s := buildState(vm, vd, pvcWFFC, pvAvail)
		ctx := logger.ToContext(context.TODO(), slog.Default())
		terms, err := s.PVNodeAffinityTerms(ctx)
		Expect(err).NotTo(HaveOccurred())
		Expect(terms).To(HaveLen(1))
		Expect(terms[0].MatchExpressions).To(HaveLen(1))
		Expect(terms[0].MatchExpressions[0].Values).To(ConsistOf(node2))
	})

	It("should use VirtualDisk spec storage class when PVC and status storage classes are empty", func() {
		vm := makeVM(v1alpha2.BlockDeviceSpecRef{Kind: v1alpha2.DiskDevice, Name: "wffc-disk"})
		vd := makeVD("wffc-disk", "pvc-wffc")
		sc := "local-storage"
		vd.Spec.PersistentVolumeClaim.StorageClass = &sc
		pvcWFFC := &corev1.PersistentVolumeClaim{
			ObjectMeta: metav1.ObjectMeta{Name: "pvc-wffc", Namespace: ns},
		}

		pvAvail := makePV("pv-avail-1", nodeAffinityTerm(node3))
		pvAvail.Spec.StorageClassName = "local-storage"
		pvAvail.Status.Phase = corev1.VolumeAvailable

		s := buildState(vm, vd, pvcWFFC, pvAvail)
		ctx := logger.ToContext(context.TODO(), slog.Default())
		terms, err := s.PVNodeAffinityTerms(ctx)
		Expect(err).NotTo(HaveOccurred())
		Expect(terms).To(HaveLen(1))
		Expect(terms[0].MatchExpressions).To(HaveLen(1))
		Expect(terms[0].MatchExpressions[0].Values).To(ConsistOf(node3))
	})

	It("should intersect available PV terms with bound disk terms", func() {
		vm := makeVM(
			v1alpha2.BlockDeviceSpecRef{Kind: v1alpha2.DiskDevice, Name: "bound-disk"},
			v1alpha2.BlockDeviceSpecRef{Kind: v1alpha2.DiskDevice, Name: "wffc-disk"},
		)

		vdBound := makeVD("bound-disk", "pvc-bound")
		pvcBound := makePVC("pvc-bound", "pv-bound")
		pvBound := makePV("pv-bound", nodeAffinityTerm(node1, node2))

		vdWFFC := makeVD("wffc-disk", "pvc-wffc")
		vdWFFC.Status.StorageClassName = "local-storage"
		pvcWFFC := &corev1.PersistentVolumeClaim{
			ObjectMeta: metav1.ObjectMeta{Name: "pvc-wffc", Namespace: ns},
		}
		pvAvail1 := makePV("pv-avail-1", nodeAffinityTerm(node2))
		pvAvail1.Spec.StorageClassName = "local-storage"
		pvAvail1.Status.Phase = corev1.VolumeAvailable

		pvAvail2 := makePV("pv-avail-2", nodeAffinityTerm(node3))
		pvAvail2.Spec.StorageClassName = "local-storage"
		pvAvail2.Status.Phase = corev1.VolumeAvailable

		s := buildState(vm, vdBound, pvcBound, pvBound, vdWFFC, pvcWFFC, pvAvail1, pvAvail2)
		ctx := logger.ToContext(context.TODO(), slog.Default())
		terms, err := s.PVNodeAffinityTerms(ctx)
		Expect(err).NotTo(HaveOccurred())
		// bound-disk allows node1,node2; wffc-disk allows node2,node3
		// intersection (cross-product) should yield terms matching node2
		Expect(terms).NotTo(BeEmpty())
	})

	It("should collect PV nodeAffinity from VirtualImage with PVC storage", func() {
		vm := makeVM(v1alpha2.BlockDeviceSpecRef{Kind: v1alpha2.ImageDevice, Name: "pvc-image"})
		vi := makeVI("pvc-image", "pvc-vi", v1alpha2.StoragePersistentVolumeClaim)
		pvc := makePVC("pvc-vi", "pv-vi")
		pv := makePV("pv-vi", nodeAffinityTerm(node3))

		s := buildState(vm, vi, pvc, pv)
		ctx := logger.ToContext(context.TODO(), slog.Default())
		terms, err := s.PVNodeAffinityTerms(ctx)
		Expect(err).NotTo(HaveOccurred())
		Expect(terms).To(HaveLen(1))
		Expect(terms[0].MatchExpressions[0].Values).To(ConsistOf(node3))
	})

	It("should skip VirtualImage with ContainerRegistry storage", func() {
		vm := makeVM(v1alpha2.BlockDeviceSpecRef{Kind: v1alpha2.ImageDevice, Name: "cr-image"})
		vi := makeVI("cr-image", "", v1alpha2.StorageContainerRegistry)

		s := buildState(vm, vi)
		ctx := logger.ToContext(context.TODO(), slog.Default())
		terms, err := s.PVNodeAffinityTerms(ctx)
		Expect(err).NotTo(HaveOccurred())
		Expect(terms).To(BeNil())
	})

	It("should skip ClusterVirtualImage block devices", func() {
		vm := makeVM(v1alpha2.BlockDeviceSpecRef{Kind: v1alpha2.ClusterImageDevice, Name: "cvi"})

		s := buildState(vm)
		ctx := logger.ToContext(context.TODO(), slog.Default())
		terms, err := s.PVNodeAffinityTerms(ctx)
		Expect(err).NotTo(HaveOccurred())
		Expect(terms).To(BeNil())
	})

	It("should use target PVC's PV node affinity during in-progress storage migration", func() {
		vm := makeVM(v1alpha2.BlockDeviceSpecRef{Kind: v1alpha2.DiskDevice, Name: "local-disk"})

		vd := makeVD("local-disk", "pvc-source")
		vd.Generation = 1
		vd.Status.Conditions = []metav1.Condition{{
			Type:               vdcondition.MigratingType.String(),
			Status:             metav1.ConditionTrue,
			ObservedGeneration: 1,
			Reason:             "Migrating",
		}}
		vd.Status.MigrationState = v1alpha2.VirtualDiskMigrationState{
			SourcePVC: "pvc-source",
			TargetPVC: "pvc-target",
		}

		pvcSource := makePVC("pvc-source", "pv-source")
		pvSource := makePV("pv-source", nodeAffinityTerm(node1))
		pvcTarget := makePVC("pvc-target", "pv-target")
		pvTarget := makePV("pv-target", nodeAffinityTerm(node2))

		s := buildState(vm, vd, pvcSource, pvSource, pvcTarget, pvTarget)
		ctx := logger.ToContext(context.TODO(), slog.Default())
		terms, err := s.PVNodeAffinityTerms(ctx)
		Expect(err).NotTo(HaveOccurred())
		Expect(terms).To(HaveLen(1))
		Expect(terms[0].MatchExpressions[0].Values).To(ConsistOf(node2),
			"affinity should follow the migration target PVC's PV (node-2), not the source PVC's PV (node-1)")
	})

	It("should use target PVC storage class for unbound target PVC during migration", func() {
		vm := makeVM(v1alpha2.BlockDeviceSpecRef{Kind: v1alpha2.DiskDevice, Name: "local-disk"})

		vd := makeVD("local-disk", "pvc-source")
		vd.Generation = 1
		vd.Status.StorageClassName = "source-sc"
		vd.Status.Conditions = []metav1.Condition{{
			Type:               vdcondition.MigratingType.String(),
			Status:             metav1.ConditionTrue,
			ObservedGeneration: 1,
			Reason:             "Migrating",
		}}
		vd.Status.MigrationState = v1alpha2.VirtualDiskMigrationState{
			SourcePVC: "pvc-source",
			TargetPVC: "pvc-target",
		}

		pvcSource := makePVC("pvc-source", "pv-source")
		pvSource := makePV("pv-source", nodeAffinityTerm(node1))
		pvSource.Spec.StorageClassName = "source-sc"
		pvSource.Status.Phase = corev1.VolumeBound

		targetSC := "target-sc"
		pvcTarget := &corev1.PersistentVolumeClaim{
			ObjectMeta: metav1.ObjectMeta{Name: "pvc-target", Namespace: ns},
			Spec: corev1.PersistentVolumeClaimSpec{
				StorageClassName: &targetSC,
			},
		}
		pvAvailTarget := makePV("pv-target-avail", nodeAffinityTerm(node2))
		pvAvailTarget.Spec.StorageClassName = "target-sc"
		pvAvailTarget.Status.Phase = corev1.VolumeAvailable

		s := buildState(vm, vd, pvcSource, pvSource, pvcTarget, pvAvailTarget)
		ctx := logger.ToContext(context.TODO(), slog.Default())
		terms, err := s.PVNodeAffinityTerms(ctx)
		Expect(err).NotTo(HaveOccurred())
		Expect(terms).To(HaveLen(1))
		Expect(terms[0].MatchExpressions[0].Values).To(ConsistOf(node2),
			"affinity for unbound target PVC should use target storage class, not source VD status class")
	})

	It("should fall back to source PVC when migration condition is False (e.g. reverted)", func() {
		vm := makeVM(v1alpha2.BlockDeviceSpecRef{Kind: v1alpha2.DiskDevice, Name: "local-disk"})

		vd := makeVD("local-disk", "pvc-source")
		vd.Generation = 1
		vd.Status.Conditions = []metav1.Condition{{
			Type:               vdcondition.MigratingType.String(),
			Status:             metav1.ConditionFalse,
			ObservedGeneration: 1,
			Reason:             "MigrationReverted",
		}}
		vd.Status.MigrationState = v1alpha2.VirtualDiskMigrationState{
			SourcePVC: "pvc-source",
			TargetPVC: "pvc-target",
		}

		pvcSource := makePVC("pvc-source", "pv-source")
		pvSource := makePV("pv-source", nodeAffinityTerm(node1))
		pvcTarget := makePVC("pvc-target", "pv-target")
		pvTarget := makePV("pv-target", nodeAffinityTerm(node2))

		s := buildState(vm, vd, pvcSource, pvSource, pvcTarget, pvTarget)
		ctx := logger.ToContext(context.TODO(), slog.Default())
		terms, err := s.PVNodeAffinityTerms(ctx)
		Expect(err).NotTo(HaveOccurred())
		Expect(terms).To(HaveLen(1))
		Expect(terms[0].MatchExpressions[0].Values).To(ConsistOf(node1),
			"affinity should fall back to source when migration is not in progress")
	})

	It("should fall back to source PVC when Migrating condition is stale (older generation)", func() {
		vm := makeVM(v1alpha2.BlockDeviceSpecRef{Kind: v1alpha2.DiskDevice, Name: "local-disk"})

		vd := makeVD("local-disk", "pvc-source")
		vd.Generation = 2
		vd.Status.Conditions = []metav1.Condition{{
			Type:               vdcondition.MigratingType.String(),
			Status:             metav1.ConditionTrue,
			ObservedGeneration: 1, // stale
			Reason:             "Migrating",
		}}
		vd.Status.MigrationState = v1alpha2.VirtualDiskMigrationState{
			SourcePVC: "pvc-source",
			TargetPVC: "pvc-target",
		}

		pvcSource := makePVC("pvc-source", "pv-source")
		pvSource := makePV("pv-source", nodeAffinityTerm(node1))
		pvcTarget := makePVC("pvc-target", "pv-target")
		pvTarget := makePV("pv-target", nodeAffinityTerm(node2))

		s := buildState(vm, vd, pvcSource, pvSource, pvcTarget, pvTarget)
		ctx := logger.ToContext(context.TODO(), slog.Default())
		terms, err := s.PVNodeAffinityTerms(ctx)
		Expect(err).NotTo(HaveOccurred())
		Expect(terms).To(HaveLen(1))
		Expect(terms[0].MatchExpressions[0].Values).To(ConsistOf(node1),
			"affinity should fall back to source when Migrating condition is not last-updated")
	})
})

func vmFactoryByVM(vm *v1alpha2.VirtualMachine) func() *v1alpha2.VirtualMachine {
	return func() *v1alpha2.VirtualMachine {
		return vm
	}
}

func vmStatusGetter(obj *v1alpha2.VirtualMachine) v1alpha2.VirtualMachineStatus {
	return obj.Status
}
