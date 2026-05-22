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
	"log/slog"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	storagev1 "k8s.io/api/storage/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"
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

var _ = Describe("ObjectRef VirtualImage", func() {
	var (
		ctx    context.Context
		scheme *runtime.Scheme
		vi     *v1alpha2.VirtualImage
		vd     *v1alpha2.VirtualDisk
		sc     *storagev1.StorageClass
		pvc    *corev1.PersistentVolumeClaim
		svc    *ObjectRefVirtualImageDiskServiceMock
	)

	BeforeEach(func() {
		ctx = logger.ToContext(context.TODO(), slog.Default())

		scheme = runtime.NewScheme()
		Expect(v1alpha2.AddToScheme(scheme)).To(Succeed())
		Expect(corev1.AddToScheme(scheme)).To(Succeed())
		Expect(storagev1.AddToScheme(scheme)).To(Succeed())

		svc = &ObjectRefVirtualImageDiskServiceMock{
			GetCapacityFunc: func(_ *corev1.PersistentVolumeClaim) string {
				return "100Mi"
			},
			CleanUpSupplementsFunc: func(_ context.Context, _ supplements.Generator) (bool, error) {
				return false, nil
			},
			ProtectFunc: func(_ context.Context, _ supplements.Generator, _ client.Object, _ *corev1.PersistentVolumeClaim) error {
				return nil
			},
			EnsurePVCImportFunc: func(_ context.Context, _ *corev1.PersistentVolumeClaim, _ *service.PVCImportSource, _ *v1alpha2.VirtualDisk, _ *provisioner.NodePlacement) (corev1.PodPhase, error) {
				return corev1.PodRunning, nil
			},
		}

		sc = &storagev1.StorageClass{
			ObjectMeta: metav1.ObjectMeta{
				Name: "sc",
			},
		}

		vi = &v1alpha2.VirtualImage{
			ObjectMeta: metav1.ObjectMeta{
				Name:       "vi",
				Generation: 1,
				UID:        "11111111-1111-1111-1111-111111111111",
			},
			Status: v1alpha2.VirtualImageStatus{
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
						Kind: v1alpha2.VirtualDiskObjectRefKindVirtualImage,
						Name: vi.Name,
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
					corev1.ResourceStorage: resource.MustParse(vi.Status.Size.UnpackedBytes),
				},
			},
		}

	})

	Context("VirtualDisk has just been created", func() {
		It("must start PVC import", func() {
			var importStarted bool
			vd.Status = v1alpha2.VirtualDiskStatus{
				Target: v1alpha2.DiskTarget{
					PersistentVolumeClaim: "test-pvc",
				},
			}
			fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(vi, sc).Build()
			svc.StartPVCImportFunc = func(_ context.Context, _ resource.Quantity, _ *storagev1.StorageClass, _ *service.PVCImportSource, _ *v1alpha2.VirtualDisk, _ *provisioner.NodePlacement) error {
				importStarted = true
				return nil
			}

			syncer := NewObjectRefVirtualImage(svc, fakeClient)

			res, err := syncer.Sync(ctx, vd)
			Expect(err).ToNot(HaveOccurred())
			Expect(res.IsZero()).To(BeTrue())

			Expect(importStarted).To(BeTrue())

			ExpectCondition(vd, metav1.ConditionFalse, vdcondition.Provisioning, true)
			Expect(vd.Status.Phase).To(Equal(v1alpha2.DiskProvisioning))
			Expect(vd.Status.Progress).ToNot(BeEmpty())
			Expect(vd.Status.Target.PersistentVolumeClaim).ToNot(BeEmpty())
		})
	})

	Context("VirtualDisk waits for the PVC to be Bound", func() {
		It("waits for the first consumer", func() {
			pvc.Status.Phase = corev1.ClaimPending
			sc.VolumeBindingMode = ptr.To(storagev1.VolumeBindingWaitForFirstConsumer)
			client := fake.NewClientBuilder().WithScheme(scheme).WithObjects(pvc, sc).Build()

			syncer := NewObjectRefVirtualImage(svc, client)

			res, err := syncer.Sync(ctx, vd)
			Expect(err).ToNot(HaveOccurred())
			Expect(res.IsZero()).To(BeTrue())

			ExpectCondition(vd, metav1.ConditionFalse, vdcondition.WaitingForFirstConsumer, true)
			Expect(vd.Status.Phase).To(Equal(v1alpha2.DiskWaitForFirstConsumer))
			Expect(vd.Status.Progress).ToNot(BeEmpty())
			Expect(vd.Status.Target.PersistentVolumeClaim).ToNot(BeEmpty())
		})

		It("is in provisioning", func() {
			pvc.Status.Phase = corev1.ClaimPending
			sc.VolumeBindingMode = ptr.To(storagev1.VolumeBindingImmediate)
			client := fake.NewClientBuilder().WithScheme(scheme).WithObjects(pvc, sc).Build()

			syncer := NewObjectRefVirtualImage(svc, client)

			res, err := syncer.Sync(ctx, vd)
			Expect(err).ToNot(HaveOccurred())
			Expect(res.IsZero()).To(BeTrue())

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

			syncer := NewObjectRefVirtualImage(svc, client)

			res, err := syncer.Sync(ctx, vd)
			Expect(err).ToNot(HaveOccurred())
			Expect(res.IsZero()).To(BeTrue())

			ExpectCondition(vd, metav1.ConditionTrue, vdcondition.Ready, false)
			Expect(vd.Status.Phase).To(Equal(v1alpha2.DiskReady))
			ExpectStats(vd)
		})

		It("requeues when the import has just completed", func() {
			pvc.Status.Phase = corev1.ClaimBound
			pvc.Annotations = map[string]string{
				annotations.AnnPVCImportPhase: string(corev1.PodRunning),
			}
			client := fake.NewClientBuilder().WithScheme(scheme).WithObjects(pvc, vi).Build()
			svc.EnsurePVCImportFunc = func(_ context.Context, _ *corev1.PersistentVolumeClaim, _ *service.PVCImportSource, _ *v1alpha2.VirtualDisk, _ *provisioner.NodePlacement) (corev1.PodPhase, error) {
				return corev1.PodSucceeded, nil
			}

			syncer := NewObjectRefVirtualImage(svc, client)

			res, err := syncer.Sync(ctx, vd)
			Expect(err).ToNot(HaveOccurred())
			Expect(res.Requeue).To(BeTrue())
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

			syncer := NewObjectRefVirtualImage(svc, client)

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

			syncer := NewObjectRefVirtualImage(svc, client)

			res, err := syncer.Sync(ctx, vd)
			Expect(err).ToNot(HaveOccurred())
			Expect(res.IsZero()).To(BeTrue())

			ExpectCondition(vd, metav1.ConditionFalse, vdcondition.Lost, true)
			Expect(vd.Status.Phase).To(Equal(v1alpha2.DiskLost))
			Expect(vd.Status.Target.PersistentVolumeClaim).NotTo(BeEmpty())
		})
	})
})
