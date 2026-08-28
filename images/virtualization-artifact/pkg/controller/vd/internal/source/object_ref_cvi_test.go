/*
Copyright 2024 Flant JSC

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

package source

import (
	"context"
	"errors"
	"log/slog"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	storagev1 "k8s.io/api/storage/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/ptr"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/deckhouse/virtualization-controller/pkg/common/annotations"
	"github.com/deckhouse/virtualization-controller/pkg/common/provisioner"
	"github.com/deckhouse/virtualization-controller/pkg/controller/service"
	"github.com/deckhouse/virtualization-controller/pkg/controller/supplements"
	vdsupplements "github.com/deckhouse/virtualization-controller/pkg/controller/vd/internal/supplements"
	"github.com/deckhouse/virtualization-controller/pkg/logger"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/vdcondition"
)

var _ = Describe("ObjectRef ClusterVirtualImage", func() {
	var (
		ctx    context.Context
		scheme *runtime.Scheme
		cvi    *v1alpha2.ClusterVirtualImage
		vd     *v1alpha2.VirtualDisk
		sc     *storagev1.StorageClass
		pvc    *corev1.PersistentVolumeClaim
		svc    *ObjectRefVirtualImageDiskServiceMock
		pvcSvc *DataSourcePVCServiceMock
		stat   *ObjectRefClusterVirtualImageStatServiceMock
	)

	BeforeEach(func() {
		ctx = logger.ToContext(context.TODO(), slog.Default())

		scheme = runtime.NewScheme()
		Expect(v1alpha2.AddToScheme(scheme)).To(Succeed())
		Expect(corev1.AddToScheme(scheme)).To(Succeed())
		Expect(storagev1.AddToScheme(scheme)).To(Succeed())

		stat = &ObjectRefClusterVirtualImageStatServiceMock{
			GetProgressFunc: func(_ types.UID, _ *corev1.Pod, prev string, _ ...service.GetProgressOption) string {
				return prev
			},
		}

		svc = &ObjectRefVirtualImageDiskServiceMock{
			GetCapacityFunc: func(_ *corev1.PersistentVolumeClaim) string {
				return "100Mi"
			},
			CleanUpSupplementsFunc: func(_ context.Context, _ supplements.Generator) (bool, error) {
				return false, nil
			},
			GetVolumeAndAccessModesFunc: func(_ context.Context, _ kclient.Object, _ *storagev1.StorageClass) (corev1.PersistentVolumeMode, corev1.PersistentVolumeAccessMode, error) {
				return corev1.PersistentVolumeFilesystem, corev1.ReadWriteOnce, nil
			},
		}

		pvcSvc = &DataSourcePVCServiceMock{
			FinalizersFunc: func() []string { return nil },
			ImportFunc: func(_ context.Context, _ *corev1.PersistentVolumeClaim, _ *service.PVCImportSource, _ kclient.Object, _ supplements.Generator, _ *provisioner.NodePlacement) error {
				return nil
			},
			WaitForImportFunc: func(_ context.Context, _ *corev1.PersistentVolumeClaim, _ *service.PVCImportSource, _ kclient.Object, _ supplements.Generator, _ *provisioner.NodePlacement) (corev1.PodPhase, error) {
				return corev1.PodRunning, nil
			},
		}

		sc = &storagev1.StorageClass{
			ObjectMeta: metav1.ObjectMeta{
				Name: "sc",
			},
		}

		cvi = &v1alpha2.ClusterVirtualImage{
			ObjectMeta: metav1.ObjectMeta{
				Name:       "vi",
				Generation: 1,
				UID:        "11111111-1111-1111-1111-111111111111",
			},
			Status: v1alpha2.ClusterVirtualImageStatus{
				Size: v1alpha2.ImageStatusSize{
					UnpackedBytes: "100Mi",
				},
			},
		}

		vd = &v1alpha2.VirtualDisk{
			ObjectMeta: metav1.ObjectMeta{
				Name:       "vd",
				Generation: 1,
				UID:        "22222222-2222-2222-2222-222222222222",
			},
			Spec: v1alpha2.VirtualDiskSpec{
				DataSource: &v1alpha2.VirtualDiskDataSource{
					Type: v1alpha2.DataSourceTypeObjectRef,
					ObjectRef: &v1alpha2.VirtualDiskObjectRef{
						Kind: v1alpha2.VirtualDiskObjectRefKindClusterVirtualImage,
						Name: cvi.Name,
					},
				},
			},
			Status: v1alpha2.VirtualDiskStatus{
				StorageClassName: sc.Name,
				Target: v1alpha2.DiskTarget{
					PersistentVolumeClaim: "test-pvc",
				},
			},
		}

		supgen := vdsupplements.NewGenerator(vd)

		pvc = &corev1.PersistentVolumeClaim{
			ObjectMeta: metav1.ObjectMeta{
				Name:      supgen.PersistentVolumeClaim().Name,
				Namespace: vd.Namespace,
			},
			Spec: corev1.PersistentVolumeClaimSpec{
				StorageClassName: ptr.To(sc.Name),
			},
			Status: corev1.PersistentVolumeClaimStatus{
				Capacity: corev1.ResourceList{
					corev1.ResourceStorage: resource.MustParse(cvi.Status.Size.UnpackedBytes),
				},
			},
		}
	})

	Context("VirtualDisk has just been created", func() {
		It("must start PVC import", func() {
			var importStarted bool
			vd.Status = v1alpha2.VirtualDiskStatus{
				StorageClassName: sc.Name,
				Target: v1alpha2.DiskTarget{
					PersistentVolumeClaim: "test-pvc",
				},
			}
			fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(cvi, sc).Build()
			pvcSvc.CreateTargetFromDVCRFunc = func(_ context.Context, _ types.NamespacedName, _ string, _ *resource.Quantity, _ kclient.Object, _ *service.PVCImportSourceRegistry, _ service.VolumeAndAccessModesGetter, _ *provisioner.NodePlacement) (corev1.PersistentVolumeClaim, error) {
				importStarted = true
				return corev1.PersistentVolumeClaim{}, nil
			}

			syncer := NewObjectRefClusterVirtualImage(svc, pvcSvc, stat, fakeClient)

			res, err := syncer.Sync(ctx, vd)
			Expect(err).ToNot(HaveOccurred())
			Expect(res.IsZero()).To(BeTrue())

			Expect(importStarted).To(BeTrue())

			ExpectCondition(vd, metav1.ConditionFalse, vdcondition.Provisioning, true)
			Expect(vd.Status.Phase).To(Equal(v1alpha2.DiskProvisioning))
			Expect(vd.Status.Progress).ToNot(BeEmpty())
			Expect(vd.Status.Target.PersistentVolumeClaim).ToNot(BeEmpty())
		})

		It("does not create the PVC while no VM consumes the disk on a WFFC storage class", func() {
			var pvcCreated bool
			sc.VolumeBindingMode = ptr.To(storagev1.VolumeBindingWaitForFirstConsumer)
			fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(cvi, sc).Build()
			pvcSvc.CreateTargetFromDVCRFunc = func(_ context.Context, _ types.NamespacedName, _ string, _ *resource.Quantity, _ kclient.Object, _ *service.PVCImportSourceRegistry, _ service.VolumeAndAccessModesGetter, _ *provisioner.NodePlacement) (corev1.PersistentVolumeClaim, error) {
				pvcCreated = true
				return corev1.PersistentVolumeClaim{}, nil
			}

			syncer := NewObjectRefClusterVirtualImage(svc, pvcSvc, stat, fakeClient)

			res, err := syncer.Sync(ctx, vd)
			Expect(err).ToNot(HaveOccurred())
			Expect(res.IsZero()).To(BeTrue())

			Expect(pvcCreated).To(BeFalse())
			ExpectCondition(vd, metav1.ConditionFalse, vdcondition.WaitingForFirstConsumer, true)
			Expect(vd.Status.Phase).To(Equal(v1alpha2.DiskWaitForFirstConsumer))
		})

		It("creates the PVC before the VM node is known once a VM consumes the disk on a WFFC storage class", func() {
			var pvcCreated bool
			sc.VolumeBindingMode = ptr.To(storagev1.VolumeBindingWaitForFirstConsumer)
			vm := &v1alpha2.VirtualMachine{
				ObjectMeta: metav1.ObjectMeta{Name: "vm", Namespace: vd.Namespace},
			}
			vd.Status.AttachedToVirtualMachines = []v1alpha2.AttachedVirtualMachine{{Name: vm.Name}}
			fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(cvi, sc, vm).Build()
			pvcSvc.CreateTargetFromDVCRFunc = func(_ context.Context, _ types.NamespacedName, _ string, _ *resource.Quantity, _ kclient.Object, _ *service.PVCImportSourceRegistry, _ service.VolumeAndAccessModesGetter, _ *provisioner.NodePlacement) (corev1.PersistentVolumeClaim, error) {
				pvcCreated = true
				return corev1.PersistentVolumeClaim{}, nil
			}

			syncer := NewObjectRefClusterVirtualImage(svc, pvcSvc, stat, fakeClient)

			res, err := syncer.Sync(ctx, vd)
			Expect(err).ToNot(HaveOccurred())
			Expect(res.IsZero()).To(BeTrue())

			Expect(pvcCreated).To(BeTrue())
		})

		It("propagates a target PVC quota rejection as Pending/QuotaExceeded instead of an error", func() {
			vd.Status = v1alpha2.VirtualDiskStatus{
				StorageClassName: sc.Name,
				Target: v1alpha2.DiskTarget{
					PersistentVolumeClaim: "test-pvc",
				},
			}
			fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(cvi, sc).Build()
			pvcSvc.CreateTargetFromDVCRFunc = func(_ context.Context, _ types.NamespacedName, _ string, _ *resource.Quantity, _ kclient.Object, _ *service.PVCImportSourceRegistry, _ service.VolumeAndAccessModesGetter, _ *provisioner.NodePlacement) (corev1.PersistentVolumeClaim, error) {
				return corev1.PersistentVolumeClaim{}, errors.New(`persistentvolumeclaims "d8v-vd-test" is forbidden: exceeded quota: block-pods-and-pvcs, requested: count/persistentvolumeclaims=1, used: count/persistentvolumeclaims=1, limited: count/persistentvolumeclaims=0`)
			}

			syncer := NewObjectRefClusterVirtualImage(svc, pvcSvc, stat, fakeClient)

			res, err := syncer.Sync(ctx, vd)
			// The quota rejection must not surface as a reconciler error.
			Expect(err).ToNot(HaveOccurred())
			Expect(res.IsZero()).To(BeTrue())

			Expect(vd.Status.Phase).To(Equal(v1alpha2.DiskPending))
			ExpectCondition(vd, metav1.ConditionFalse, vdcondition.QuotaExceeded, true)
		})
	})

	Context("VirtualDisk is provisioning while its PVC is not yet Bound", func() {
		// Until a consumer schedules onto the WFFC target (the scheduler stamps its
		// selected-node annotation) the populator defers the import, so the disk
		// keeps reporting WaitForFirstConsumer: the VirtualMachine controller only
		// starts the consuming VM while the disk is in this phase.
		It("reports WaitForFirstConsumer for WFFC storage class until a consumer is scheduled", func() {
			pvc.Status.Phase = corev1.ClaimPending
			sc.VolumeBindingMode = ptr.To(storagev1.VolumeBindingWaitForFirstConsumer)
			client := fake.NewClientBuilder().WithScheme(scheme).WithObjects(pvc, sc).Build()

			syncer := NewObjectRefClusterVirtualImage(svc, pvcSvc, stat, client)

			res, err := syncer.Sync(ctx, vd)
			Expect(err).ToNot(HaveOccurred())
			Expect(res.IsZero()).To(BeTrue())

			ExpectCondition(vd, metav1.ConditionFalse, vdcondition.WaitingForFirstConsumer, true)
			Expect(vd.Status.Phase).To(Equal(v1alpha2.DiskWaitForFirstConsumer))
			Expect(vd.Status.Target.PersistentVolumeClaim).ToNot(BeEmpty())
		})

		It("reports Provisioning for WFFC storage class once the consumer node is selected", func() {
			pvc.Status.Phase = corev1.ClaimPending
			pvc.Annotations = map[string]string{service.SelectedNodeAnnotation: "node-a"}
			sc.VolumeBindingMode = ptr.To(storagev1.VolumeBindingWaitForFirstConsumer)
			client := fake.NewClientBuilder().WithScheme(scheme).WithObjects(pvc, sc).Build()

			syncer := NewObjectRefClusterVirtualImage(svc, pvcSvc, stat, client)

			res, err := syncer.Sync(ctx, vd)
			Expect(err).ToNot(HaveOccurred())
			Expect(res.RequeueAfter).ToNot(BeZero())

			ExpectCondition(vd, metav1.ConditionFalse, vdcondition.Provisioning, true)
			Expect(vd.Status.Phase).To(Equal(v1alpha2.DiskProvisioning))
			Expect(vd.Status.Progress).ToNot(BeEmpty())
			Expect(vd.Status.Target.PersistentVolumeClaim).ToNot(BeEmpty())
		})

		It("reports Provisioning for Immediate storage class", func() {
			pvc.Status.Phase = corev1.ClaimPending
			sc.VolumeBindingMode = ptr.To(storagev1.VolumeBindingImmediate)
			client := fake.NewClientBuilder().WithScheme(scheme).WithObjects(pvc, sc).Build()

			syncer := NewObjectRefClusterVirtualImage(svc, pvcSvc, stat, client)

			res, err := syncer.Sync(ctx, vd)
			Expect(err).ToNot(HaveOccurred())
			Expect(res.RequeueAfter).ToNot(BeZero())

			ExpectCondition(vd, metav1.ConditionFalse, vdcondition.Provisioning, true)
			Expect(vd.Status.Phase).To(Equal(v1alpha2.DiskProvisioning))
			Expect(vd.Status.Progress).ToNot(BeEmpty())
			Expect(vd.Status.Target.PersistentVolumeClaim).ToNot(BeEmpty())
		})
	})

	Context("VirtualDisk is ready", func() {
		It("checks that the VirtualDisk is ready", func() {
			pvc.Status.Phase = corev1.ClaimBound
			client := fake.NewClientBuilder().WithScheme(scheme).WithObjects(pvc).Build()

			syncer := NewObjectRefClusterVirtualImage(svc, pvcSvc, stat, client)

			res, err := syncer.Sync(ctx, vd)
			Expect(err).ToNot(HaveOccurred())
			Expect(res.IsZero()).To(BeTrue())

			ExpectCondition(vd, metav1.ConditionTrue, vdcondition.Ready, false)
			Expect(vd.Status.Phase).To(Equal(v1alpha2.DiskReady))
			ExpectStats(vd)
		})

		It("requeues when the import has just completed", func() {
			pvc.Status.Phase = corev1.ClaimBound
			pvc.Annotations = map[string]string{annotations.AnnPVCPopulationStrategy: service.PopulationStrategyDVCR}
			client := fake.NewClientBuilder().WithScheme(scheme).WithObjects(pvc, cvi).Build()
			pvcSvc.WaitForImportFunc = func(_ context.Context, _ *corev1.PersistentVolumeClaim, _ *service.PVCImportSource, _ kclient.Object, _ supplements.Generator, _ *provisioner.NodePlacement) (corev1.PodPhase, error) {
				return corev1.PodSucceeded, nil
			}

			syncer := NewObjectRefClusterVirtualImage(svc, pvcSvc, stat, client)

			res, err := syncer.Sync(ctx, vd)
			Expect(err).ToNot(HaveOccurred())
			Expect(res.RequeueAfter).ToNot(BeZero())
		})

		It("waits for populator when the target PVC already exists", func() {
			pvc.Status.Phase = corev1.ClaimBound
			pvc.Annotations = map[string]string{annotations.AnnPVCPopulationStrategy: service.PopulationStrategyDVCR}
			cvi.Status.Target.RegistryURL = "registry.example/cvi/source"
			var imported bool
			pvcSvc.ImportFunc = func(_ context.Context, target *corev1.PersistentVolumeClaim, source *service.PVCImportSource, _ kclient.Object, _ supplements.Generator, _ *provisioner.NodePlacement) error {
				imported = true
				Expect(target.Name).To(Equal(pvc.Name))
				Expect(source).ToNot(BeNil())
				Expect(source.Registry).ToNot(BeNil())
				return nil
			}
			pvcSvc.WaitForImportFunc = func(_ context.Context, _ *corev1.PersistentVolumeClaim, source *service.PVCImportSource, _ kclient.Object, _ supplements.Generator, _ *provisioner.NodePlacement) (corev1.PodPhase, error) {
				Expect(imported).To(BeTrue())
				Expect(source).ToNot(BeNil())
				Expect(source.Registry).ToNot(BeNil())
				return corev1.PodPending, nil
			}
			client := fake.NewClientBuilder().WithScheme(scheme).WithObjects(pvc, cvi).Build()

			syncer := NewObjectRefClusterVirtualImage(svc, pvcSvc, stat, client)

			res, err := syncer.Sync(ctx, vd)
			Expect(err).ToNot(HaveOccurred())
			Expect(res.RequeueAfter).ToNot(BeZero())
			Expect(imported).To(BeFalse())
			Expect(vd.Status.Phase).To(Equal(v1alpha2.DiskProvisioning))
			ExpectCondition(vd, metav1.ConditionFalse, vdcondition.Provisioning, true)
		})
	})

	Context("VirtualDisk is lost", func() {
		BeforeEach(func() {
			vd.Status.Progress = "100%"
		})

		It("is lost when PVC is not found", func() {
			vd.Status.Target.PersistentVolumeClaim = pvc.Name
			vd.Status.Conditions = []metav1.Condition{
				{
					Type:   vdcondition.ReadyType.String(),
					Reason: vdcondition.Ready.String(),
					Status: metav1.ConditionTrue,
				},
			}
			client := fake.NewClientBuilder().WithScheme(scheme).WithObjects().Build()

			syncer := NewObjectRefClusterVirtualImage(svc, pvcSvc, stat, client)

			res, err := syncer.Sync(ctx, vd)
			Expect(err).ToNot(HaveOccurred())
			Expect(res.IsZero()).To(BeTrue())

			ExpectCondition(vd, metav1.ConditionFalse, vdcondition.Lost, true)
			Expect(vd.Status.Phase).To(Equal(v1alpha2.DiskLost))
			Expect(vd.Status.Target.PersistentVolumeClaim).NotTo(BeEmpty())
		})

		It("is lost when PVC is lost as well", func() {
			pvc.Status.Phase = corev1.ClaimLost
			vd.Status.Target.PersistentVolumeClaim = pvc.Name
			client := fake.NewClientBuilder().WithScheme(scheme).WithObjects(pvc).Build()

			syncer := NewObjectRefClusterVirtualImage(svc, pvcSvc, stat, client)

			res, err := syncer.Sync(ctx, vd)
			Expect(err).ToNot(HaveOccurred())
			Expect(res.IsZero()).To(BeTrue())

			ExpectCondition(vd, metav1.ConditionFalse, vdcondition.Lost, true)
			Expect(vd.Status.Phase).To(Equal(v1alpha2.DiskLost))
			Expect(vd.Status.Target.PersistentVolumeClaim).NotTo(BeEmpty())
		})
	})
})
