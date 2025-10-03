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
	"testing"

	vsv1 "github.com/kubernetes-csi/external-snapshotter/client/v6/apis/volumesnapshot/v1"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	storagev1 "k8s.io/api/storage/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/utils/ptr"
	cdiv1 "kubevirt.io/containerized-data-importer-api/pkg/apis/core/v1beta1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"

	"github.com/deckhouse/virtualization-controller/pkg/controller/conditions"
	"github.com/deckhouse/virtualization-controller/pkg/controller/supplements"
	"github.com/deckhouse/virtualization-controller/pkg/eventrecord"
	"github.com/deckhouse/virtualization-controller/pkg/logger"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/vdcondition"
)

func TestHandlers(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Sources")
}

var _ = Describe("ObjectRef VirtualDiskSnapshot", func() {
	var (
		ctx        context.Context
		scheme     *runtime.Scheme
		vd         *v1alpha2.VirtualDisk
		vs         *vsv1.VolumeSnapshot
		sc         *storagev1.StorageClass
		vdSnapshot *v1alpha2.VirtualDiskSnapshot
		pvc        *corev1.PersistentVolumeClaim
		recorder   eventrecord.EventRecorderLogger
		svc        *ObjectRefVirtualDiskSnapshotDiskServiceMock
	)

	BeforeEach(func() {
		ctx = logger.ToContext(context.TODO(), slog.Default())

		scheme = runtime.NewScheme()
		Expect(v1alpha2.AddToScheme(scheme)).To(Succeed())
		Expect(corev1.AddToScheme(scheme)).To(Succeed())
		Expect(vsv1.AddToScheme(scheme)).To(Succeed())
		Expect(storagev1.AddToScheme(scheme)).To(Succeed())

		recorder = &eventrecord.EventRecorderLoggerMock{
			EventFunc: func(_ client.Object, _, _, _ string) {},
		}

		svc = &ObjectRefVirtualDiskSnapshotDiskServiceMock{
			GetCapacityFunc: func(_ *corev1.PersistentVolumeClaim) string {
				return "1Mi"
			},
			CleanUpSupplementsFunc: func(_ context.Context, _ supplements.Generator) (bool, error) {
				return false, nil
			},
			ProtectFunc: func(_ context.Context, _ supplements.Generator, _ client.Object, _ *cdiv1.DataVolume, _ *corev1.PersistentVolumeClaim) error {
				return nil
			},
		}

		sc = &storagev1.StorageClass{
			ObjectMeta: metav1.ObjectMeta{
				Name: "sc",
			},
		}

		pvc = &corev1.PersistentVolumeClaim{
			ObjectMeta: metav1.ObjectMeta{
				Name: "test-pvc",
			},
			Spec: corev1.PersistentVolumeClaimSpec{
				StorageClassName: &sc.Name,
			},
			Status: corev1.PersistentVolumeClaimStatus{
				Phase: corev1.ClaimBound,
			},
		}

		vs = &vsv1.VolumeSnapshot{
			ObjectMeta: metav1.ObjectMeta{
				Name: "vs",
			},
			Status: &vsv1.VolumeSnapshotStatus{
				ReadyToUse: ptr.To(true),
			},
		}

		vdSnapshot = &v1alpha2.VirtualDiskSnapshot{
			ObjectMeta: metav1.ObjectMeta{
				Name: "vd-snapshot",
				UID:  "11111111-1111-1111-1111-111111111111",
			},
			Spec: v1alpha2.VirtualDiskSnapshotSpec{},
			Status: v1alpha2.VirtualDiskSnapshotStatus{
				Phase:              v1alpha2.VirtualDiskSnapshotPhaseReady,
				VolumeSnapshotName: vs.Name,
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
						Kind: v1alpha2.VirtualDiskObjectRefKindVirtualDiskSnapshot,
						Name: vdSnapshot.Name,
					},
				},
			},
			Status: v1alpha2.VirtualDiskStatus{
				Target: v1alpha2.DiskTarget{
					PersistentVolumeClaim: "test-pvc",
				},
			},
		}
	})

	Context("VirtualDisk has just been created", func() {
		It("must create PVC", func() {
			var pvcCreated bool
			vd.Status = v1alpha2.VirtualDiskStatus{
				Target: v1alpha2.DiskTarget{
					PersistentVolumeClaim: "test-pvc",
				},
			}
			client := fake.NewClientBuilder().WithScheme(scheme).WithObjects(vdSnapshot, vs).
				WithInterceptorFuncs(interceptor.Funcs{
					Create: func(_ context.Context, _ client.WithWatch, obj client.Object, _ ...client.CreateOption) error {
						_, ok := obj.(*corev1.PersistentVolumeClaim)
						Expect(ok).To(BeTrue())
						pvcCreated = true
						return nil
					},
				}).Build()

			syncer := NewObjectRefVirtualDiskSnapshot(recorder, svc, client)

			res, err := syncer.Sync(ctx, vd)
			Expect(err).ToNot(HaveOccurred())
			Expect(res.IsZero()).To(BeTrue())

			Expect(pvcCreated).To(BeTrue())

			ExpectCondition(vd, metav1.ConditionFalse, vdcondition.Provisioning, true)
			Expect(vd.Status.SourceUID).ToNot(BeNil())
			Expect(*vd.Status.SourceUID).ToNot(BeEmpty())
			Expect(vd.Status.Phase).To(Equal(v1alpha2.DiskProvisioning))
			Expect(vd.Status.Target.PersistentVolumeClaim).NotTo(BeEmpty())
		})
	})

	Context("VirtualDisk waits for the PVC to be Bound", func() {
		It("waits for the first consumer", func() {
			pvc.Status.Phase = corev1.ClaimPending
			sc.VolumeBindingMode = ptr.To(storagev1.VolumeBindingWaitForFirstConsumer)
			client := fake.NewClientBuilder().WithScheme(scheme).WithObjects(pvc, sc).Build()

			syncer := NewObjectRefVirtualDiskSnapshot(recorder, svc, client)

			res, err := syncer.Sync(ctx, vd)
			Expect(err).ToNot(HaveOccurred())
			Expect(res.IsZero()).To(BeTrue())

			ExpectCondition(vd, metav1.ConditionFalse, vdcondition.WaitingForFirstConsumer, true)
			Expect(vd.Status.Phase).To(Equal(v1alpha2.DiskWaitForFirstConsumer))
		})

		It("is in provisioning", func() {
			pvc.Status.Phase = corev1.ClaimPending
			sc.VolumeBindingMode = ptr.To(storagev1.VolumeBindingImmediate)
			client := fake.NewClientBuilder().WithScheme(scheme).WithObjects(pvc, sc).Build()

			syncer := NewObjectRefVirtualDiskSnapshot(recorder, svc, client)

			res, err := syncer.Sync(ctx, vd)
			Expect(err).ToNot(HaveOccurred())
			Expect(res.IsZero()).To(BeTrue())

			ExpectCondition(vd, metav1.ConditionFalse, vdcondition.Provisioning, true)
			Expect(vd.Status.Phase).To(Equal(v1alpha2.DiskProvisioning))
		})
	})

	Context("VirtualDisk is ready", func() {
		It("checks that the VirtualDisk is ready", func() {
			pvc.Status.Phase = corev1.ClaimBound
			client := fake.NewClientBuilder().WithScheme(scheme).WithObjects(pvc).Build()

			syncer := NewObjectRefVirtualDiskSnapshot(recorder, svc, client)

			res, err := syncer.Sync(ctx, vd)
			Expect(err).ToNot(HaveOccurred())
			Expect(res.IsZero()).To(BeTrue())

			ExpectCondition(vd, metav1.ConditionTrue, vdcondition.Ready, false)
			ExpectStats(vd)
			Expect(vd.Status.Phase).To(Equal(v1alpha2.DiskReady))
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

			syncer := NewObjectRefVirtualDiskSnapshot(recorder, svc, client)

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

			syncer := NewObjectRefVirtualDiskSnapshot(recorder, svc, client)

			res, err := syncer.Sync(ctx, vd)
			Expect(err).ToNot(HaveOccurred())
			Expect(res.IsZero()).To(BeTrue())

			ExpectCondition(vd, metav1.ConditionFalse, vdcondition.Lost, true)
			Expect(vd.Status.Phase).To(Equal(v1alpha2.DiskLost))
			Expect(vd.Status.Target.PersistentVolumeClaim).NotTo(BeEmpty())
		})
	})
})

func ExpectStats(vd *v1alpha2.VirtualDisk) {
	Expect(vd.Status.Target.PersistentVolumeClaim).ToNot(BeEmpty())
	Expect(vd.Status.Capacity).ToNot(BeEmpty())
	Expect(vd.Status.Progress).ToNot(BeEmpty())
	Expect(vd.Status.Phase).ToNot(BeEmpty())
}

func ExpectCondition(vd *v1alpha2.VirtualDisk, status metav1.ConditionStatus, reason vdcondition.ReadyReason, msgExists bool) {
	ready, _ := conditions.GetCondition(vdcondition.Ready, vd.Status.Conditions)
	Expect(ready.Status).To(Equal(status))
	Expect(ready.Reason).To(Equal(reason.String()))
	Expect(ready.ObservedGeneration).To(Equal(vd.Generation))

	if msgExists {
		Expect(ready.Message).ToNot(BeEmpty())
	} else {
		Expect(ready.Message).To(BeEmpty())
	}
}
