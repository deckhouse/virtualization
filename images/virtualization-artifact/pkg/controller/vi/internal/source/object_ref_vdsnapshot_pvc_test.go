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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"

	"github.com/deckhouse/virtualization-controller/pkg/common/annotations"
	"github.com/deckhouse/virtualization-controller/pkg/controller/service"
	"github.com/deckhouse/virtualization-controller/pkg/controller/supplements"
	"github.com/deckhouse/virtualization-controller/pkg/dvcr"
	"github.com/deckhouse/virtualization-controller/pkg/eventrecord"
	"github.com/deckhouse/virtualization-controller/pkg/logger"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/vicondition"
)

var _ = Describe("ObjectRef VirtualImageSnapshot PersistentVolumeClaim", func() {
	var (
		ctx         context.Context
		scheme      *runtime.Scheme
		vi          *v1alpha2.VirtualImage
		vs          *vsv1.VolumeSnapshot
		sc          *storagev1.StorageClass
		vdSnapshot  *v1alpha2.VirtualDiskSnapshot
		pvc         *corev1.PersistentVolumeClaim
		settings    *dvcr.Settings
		recorder    eventrecord.EventRecorderLogger
		importer    *ImporterMock
		bounder     *BounderMock
		stat        *StatMock
		diskService *DiskMock
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

		importer = &ImporterMock{
			CleanUpSupplementsFunc: func(_ context.Context, _ supplements.Generator) (bool, error) {
				return false, nil
			},
		}
		bounder = &BounderMock{
			CleanUpSupplementsFunc: func(_ context.Context, _ supplements.Generator) (bool, error) {
				return false, nil
			},
		}
		stat = &StatMock{
			GetDVCRImageNameFunc: func(_ *corev1.Pod) string {
				return "image"
			},
			CheckPodFunc: func(_ *corev1.Pod) error {
				return nil
			},
			GetSizeFunc: func(_ *corev1.Pod) v1alpha2.ImageStatusSize {
				return v1alpha2.ImageStatusSize{}
			},
			GetCDROMFunc: func(_ *corev1.Pod) bool {
				return false
			},
			GetFormatFunc: func(_ *corev1.Pod) string {
				return "iso"
			},
			GetProgressFunc: func(_ types.UID, _ *corev1.Pod, _ string, _ ...service.GetProgressOption) string {
				return "N%"
			},
		}
		diskService = &DiskMock{
			GetPersistentVolumeClaimFunc: func(ctx context.Context, sup supplements.Generator) (*corev1.PersistentVolumeClaim, error) {
				// Return the test PVC if it exists
				if pvc != nil && pvc.Name != "" {
					return pvc, nil
				}
				return nil, nil
			},
		}
		settings = &dvcr.Settings{}

		sc = &storagev1.StorageClass{
			ObjectMeta: metav1.ObjectMeta{
				Name: "sc",
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

		vi = &v1alpha2.VirtualImage{
			ObjectMeta: metav1.ObjectMeta{
				Name:       "vi",
				Generation: 1,
				UID:        "22222222-2222-2222-2222-222222222222",
			},
			Spec: v1alpha2.VirtualImageSpec{
				Storage: v1alpha2.StoragePersistentVolumeClaim,
				DataSource: v1alpha2.VirtualImageDataSource{
					Type: v1alpha2.DataSourceTypeObjectRef,
					ObjectRef: &v1alpha2.VirtualImageObjectRef{
						Kind: v1alpha2.VirtualImageObjectRefKindVirtualDiskSnapshot,
						Name: vdSnapshot.Name,
					},
				},
			},
		}

		supgen := supplements.NewGenerator(annotations.VIShortName, vi.Name, vi.Namespace, vi.UID)

		pvc = &corev1.PersistentVolumeClaim{
			ObjectMeta: metav1.ObjectMeta{
				Name: supgen.PersistentVolumeClaim().Name,
			},
			Spec: corev1.PersistentVolumeClaimSpec{
				StorageClassName: &sc.Name,
			},
			Status: corev1.PersistentVolumeClaimStatus{
				Phase: corev1.ClaimBound,
			},
		}
	})

	Context("VirtualImage has just been created", func() {
		It("must create PVC", func() {
			var pvcCreated bool

			vi.Status = v1alpha2.VirtualImageStatus{}
			client := fake.NewClientBuilder().WithScheme(scheme).WithObjects(vdSnapshot, vs).
				WithInterceptorFuncs(interceptor.Funcs{
					Create: func(_ context.Context, _ client.WithWatch, obj client.Object, _ ...client.CreateOption) error {
						switch obj.(type) {
						case *corev1.PersistentVolumeClaim:
							pvcCreated = true
						}

						return nil
					},
				}).Build()

			diskService.GetPersistentVolumeClaimFunc = func(_ context.Context, _ supplements.Generator) (*corev1.PersistentVolumeClaim, error) {
				return nil, nil
			}

			syncer := NewObjectRefVirtualDiskSnapshotPVC(importer, stat, bounder, diskService, client, settings, recorder)

			res, err := syncer.Sync(ctx, vi)
			Expect(err).ToNot(HaveOccurred())
			Expect(res.IsZero()).To(BeTrue())

			Expect(pvcCreated).To(BeTrue())

			ExpectCondition(vi, metav1.ConditionFalse, vicondition.Provisioning, true)
			Expect(vi.Status.SourceUID).ToNot(BeNil())
			Expect(*vi.Status.SourceUID).ToNot(BeEmpty())
			Expect(vi.Status.Phase).To(Equal(v1alpha2.ImageProvisioning))
			Expect(vi.Status.Target.PersistentVolumeClaim).NotTo(BeEmpty())
		})
	})

	Context("VirtualImage is ready", func() {
		It("has PVC in Bound phase", func() {
			pvc.Status.Phase = corev1.ClaimBound
			client := fake.NewClientBuilder().WithScheme(scheme).WithObjects(pvc).Build()

			syncer := NewObjectRefVirtualDiskSnapshotPVC(importer, stat, bounder, diskService, client, nil, recorder)

			res, err := syncer.Sync(ctx, vi)
			Expect(err).ToNot(HaveOccurred())
			Expect(res.IsZero()).To(BeTrue())

			ExpectCondition(vi, metav1.ConditionTrue, vicondition.Ready, false)
			Expect(vi.Status.Phase).To(Equal(v1alpha2.ImageReady))
		})
	})

	Context("VirtualImage is lost", func() {
		It("is lost when PVC is not found", func() {
			vi.Status.Target.PersistentVolumeClaim = pvc.Name
			vi.Status.Conditions = []metav1.Condition{
				{
					Type:   vicondition.ReadyType.String(),
					Reason: vicondition.Ready.String(),
					Status: metav1.ConditionTrue,
				},
			}
			client := fake.NewClientBuilder().WithScheme(scheme).WithObjects().Build()

			diskService.GetPersistentVolumeClaimFunc = func(_ context.Context, _ supplements.Generator) (*corev1.PersistentVolumeClaim, error) {
				return nil, nil
			}

			syncer := NewObjectRefVirtualDiskSnapshotPVC(importer, stat, bounder, diskService, client, nil, recorder)

			res, err := syncer.Sync(ctx, vi)
			Expect(err).ToNot(HaveOccurred())
			Expect(res.IsZero()).To(BeTrue())

			ExpectCondition(vi, metav1.ConditionFalse, vicondition.Lost, true)
			Expect(vi.Status.Phase).To(Equal(v1alpha2.ImageLost))
			Expect(vi.Status.Target.PersistentVolumeClaim).NotTo(BeEmpty())
		})

		It("is lost when PVC is lost as well", func() {
			pvc.Status.Phase = corev1.ClaimLost
			vi.Status.Target.PersistentVolumeClaim = pvc.Name
			client := fake.NewClientBuilder().WithScheme(scheme).WithObjects(pvc).Build()

			syncer := NewObjectRefVirtualDiskSnapshotPVC(importer, stat, bounder, diskService, client, nil, recorder)

			res, err := syncer.Sync(ctx, vi)
			Expect(err).ToNot(HaveOccurred())
			Expect(res.IsZero()).To(BeTrue())

			ExpectCondition(vi, metav1.ConditionFalse, vicondition.Lost, true)
			Expect(vi.Status.Phase).To(Equal(v1alpha2.ImageLost))
			Expect(vi.Status.Target.PersistentVolumeClaim).NotTo(BeEmpty())
		})
	})
})
