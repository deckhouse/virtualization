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

	vsv1 "github.com/kubernetes-csi/external-snapshotter/client/v6/apis/volumesnapshot/v1"
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
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"

	"github.com/deckhouse/virtualization-controller/pkg/common/annotations"
	"github.com/deckhouse/virtualization-controller/pkg/controller/supplements"
	"github.com/deckhouse/virtualization-controller/pkg/eventrecord"
	"github.com/deckhouse/virtualization-controller/pkg/logger"
	virtv2 "github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/vdcondition"
)

var _ = Describe("Blank", func() {
	var (
		ctx      context.Context
		scheme   *runtime.Scheme
		vd       *virtv2.VirtualDisk
		sc       *storagev1.StorageClass
		pvc      *corev1.PersistentVolumeClaim
		recorder eventrecord.EventRecorderLogger
		svc      *BlankDataSourceDiskServiceMock
	)

	BeforeEach(func() {
		ctx = logger.ToContext(context.TODO(), slog.Default())

		scheme = runtime.NewScheme()
		Expect(virtv2.AddToScheme(scheme)).To(Succeed())
		Expect(corev1.AddToScheme(scheme)).To(Succeed())
		Expect(vsv1.AddToScheme(scheme)).To(Succeed())
		Expect(storagev1.AddToScheme(scheme)).To(Succeed())

		recorder = &eventrecord.EventRecorderLoggerMock{
			EventFunc: func(_ client.Object, _, _, _ string) {},
		}

		svc = &BlankDataSourceDiskServiceMock{
			GetVolumeAndAccessModesFunc: func(_ context.Context, _ *storagev1.StorageClass) (corev1.PersistentVolumeMode, corev1.PersistentVolumeAccessMode, error) {
				return *pvc.Spec.VolumeMode, pvc.Spec.AccessModes[0], nil
			},
			GetCapacityFunc: func(_ *corev1.PersistentVolumeClaim) string {
				return vd.Spec.PersistentVolumeClaim.Size.String()
			},
		}

		sc = &storagev1.StorageClass{
			ObjectMeta: metav1.ObjectMeta{
				Name: "sc",
			},
		}

		vd = &virtv2.VirtualDisk{
			ObjectMeta: metav1.ObjectMeta{
				Name:       "vd",
				Generation: 1,
				UID:        "11111111-1111-1111-1111-111111111111",
			},
			Spec: virtv2.VirtualDiskSpec{
				PersistentVolumeClaim: virtv2.VirtualDiskPersistentVolumeClaim{
					Size: ptr.To(resource.MustParse("10Mi")),
				},
			},
			Status: virtv2.VirtualDiskStatus{
				StorageClassName: sc.Name,
			},
		}

		supgen := supplements.NewGenerator(annotations.VDShortName, vd.Name, vd.Namespace, vd.UID)

		pvc = &corev1.PersistentVolumeClaim{
			ObjectMeta: metav1.ObjectMeta{
				Name: supgen.PersistentVolumeClaim().Name,
			},
			Spec: corev1.PersistentVolumeClaimSpec{
				StorageClassName: &sc.Name,
				VolumeMode:       ptr.To(corev1.PersistentVolumeBlock),
				AccessModes:      []corev1.PersistentVolumeAccessMode{corev1.ReadWriteMany},
				Resources: corev1.VolumeResourceRequirements{
					Requests: corev1.ResourceList{
						corev1.ResourceStorage: *vd.Spec.PersistentVolumeClaim.Size,
					},
				},
			},
		}
	})

	Context("VirtualDisk has just been created", func() {
		It("must create PVC", func() {
			var pvcCreated bool
			client := fake.NewClientBuilder().WithScheme(scheme).WithObjects(sc).
				WithInterceptorFuncs(interceptor.Funcs{
					Create: func(_ context.Context, _ client.WithWatch, obj client.Object, _ ...client.CreateOption) error {
						_, ok := obj.(*corev1.PersistentVolumeClaim)
						Expect(ok).To(BeTrue())
						pvcCreated = true
						return nil
					},
				}).Build()

			syncer := NewBlankDataSource(recorder, svc, client)

			res, err := syncer.Sync(ctx, vd)
			Expect(err).ToNot(HaveOccurred())
			Expect(res.IsZero()).To(BeTrue())

			Expect(pvcCreated).To(BeTrue())

			ExpectCondition(vd, metav1.ConditionFalse, vdcondition.Provisioning, true)
			Expect(vd.Status.Phase).To(Equal(virtv2.DiskProvisioning))
			Expect(vd.Status.Progress).NotTo(BeEmpty())
			Expect(vd.Status.Target.PersistentVolumeClaim).NotTo(BeEmpty())
		})

		It("checks size in spec", func() {
			client := fake.NewClientBuilder().WithScheme(scheme).Build()
			syncer := NewBlankDataSource(nil, nil, client)

			res, err := syncer.Sync(ctx, vd)
			Expect(err).To(HaveOccurred())
			Expect(res.IsZero()).To(BeTrue())
		})

		It("checks storage class is set in status", func() {
			client := fake.NewClientBuilder().WithScheme(scheme).Build()
			syncer := NewBlankDataSource(nil, nil, client)

			res, err := syncer.Sync(ctx, vd)
			Expect(err).To(HaveOccurred())
			Expect(res.IsZero()).To(BeTrue())
		})
	})

	Context("VirtualDisk waits for the PVC to be Bound", func() {
		It("waits for the first consumer", func() {
			pvc.Status.Phase = corev1.ClaimPending
			sc.VolumeBindingMode = ptr.To(storagev1.VolumeBindingWaitForFirstConsumer)
			client := fake.NewClientBuilder().WithScheme(scheme).WithObjects(pvc, sc).Build()

			syncer := NewBlankDataSource(nil, nil, client)

			res, err := syncer.Sync(ctx, vd)
			Expect(err).ToNot(HaveOccurred())
			Expect(res.IsZero()).To(BeTrue())

			ExpectCondition(vd, metav1.ConditionFalse, vdcondition.WaitingForFirstConsumer, true)
			Expect(vd.Status.Phase).To(Equal(virtv2.DiskWaitForFirstConsumer))
		})

		It("is in provisioning", func() {
			pvc.Status.Phase = corev1.ClaimPending
			sc.VolumeBindingMode = ptr.To(storagev1.VolumeBindingImmediate)
			client := fake.NewClientBuilder().WithScheme(scheme).WithObjects(pvc, sc).Build()

			syncer := NewBlankDataSource(nil, nil, client)

			res, err := syncer.Sync(ctx, vd)
			Expect(err).ToNot(HaveOccurred())
			Expect(res.IsZero()).To(BeTrue())

			ExpectCondition(vd, metav1.ConditionFalse, vdcondition.Provisioning, true)
			Expect(vd.Status.Phase).To(Equal(virtv2.DiskProvisioning))
		})
	})

	Context("VirtualDisk is ready", func() {
		It("checks that the VirtualDisk is ready", func() {
			pvc.Status.Phase = corev1.ClaimBound
			client := fake.NewClientBuilder().WithScheme(scheme).WithObjects(pvc).Build()

			syncer := NewBlankDataSource(nil, svc, client)

			res, err := syncer.Sync(ctx, vd)
			Expect(err).ToNot(HaveOccurred())
			Expect(res.IsZero()).To(BeTrue())

			ExpectCondition(vd, metav1.ConditionTrue, vdcondition.Ready, false)
			ExpectStats(vd)
			Expect(vd.Status.Phase).To(Equal(virtv2.DiskReady))
		})
	})

	Context("VirtualDisk is lost", func() {
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

			syncer := NewBlankDataSource(nil, svc, client)

			res, err := syncer.Sync(ctx, vd)
			Expect(err).ToNot(HaveOccurred())
			Expect(res.IsZero()).To(BeTrue())

			ExpectCondition(vd, metav1.ConditionFalse, vdcondition.Lost, true)
			Expect(vd.Status.Phase).To(Equal(virtv2.DiskLost))
			Expect(vd.Status.Target.PersistentVolumeClaim).NotTo(BeEmpty())
		})

		It("is lost when PVC is lost as well", func() {
			pvc.Status.Phase = corev1.ClaimLost
			vd.Status.Target.PersistentVolumeClaim = pvc.Name
			client := fake.NewClientBuilder().WithScheme(scheme).WithObjects(pvc).Build()

			syncer := NewBlankDataSource(nil, svc, client)

			res, err := syncer.Sync(ctx, vd)
			Expect(err).ToNot(HaveOccurred())
			Expect(res.IsZero()).To(BeTrue())

			ExpectCondition(vd, metav1.ConditionFalse, vdcondition.Lost, true)
			Expect(vd.Status.Phase).To(Equal(virtv2.DiskLost))
			Expect(vd.Status.Target.PersistentVolumeClaim).NotTo(BeEmpty())
		})
	})
})
