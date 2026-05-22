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

package source

import (
	"context"
	"errors"
	"log/slog"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	netv1 "k8s.io/api/networking/v1"
	storagev1 "k8s.io/api/storage/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/deckhouse/virtualization-controller/pkg/common/annotations"
	"github.com/deckhouse/virtualization-controller/pkg/common/datasource"
	"github.com/deckhouse/virtualization-controller/pkg/common/provisioner"
	"github.com/deckhouse/virtualization-controller/pkg/controller/service"
	"github.com/deckhouse/virtualization-controller/pkg/controller/supplements"
	"github.com/deckhouse/virtualization-controller/pkg/controller/uploader"
	vdsupplements "github.com/deckhouse/virtualization-controller/pkg/controller/vd/internal/supplements"
	"github.com/deckhouse/virtualization-controller/pkg/dvcr"
	"github.com/deckhouse/virtualization-controller/pkg/eventrecord"
	"github.com/deckhouse/virtualization-controller/pkg/logger"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/vdcondition"
)

var _ = Describe("UploadDataSource", func() {
	var (
		ctx          context.Context
		scheme       *runtime.Scheme
		vd           *v1alpha2.VirtualDisk
		sc           *storagev1.StorageClass
		pvc          *corev1.PersistentVolumeClaim
		disk         *UploadDataSourceDiskServiceMock
		uploaderSvc  *UploadDataSourceUploaderServiceMock
		stat         *UploadDataSourceStatServiceMock
		recorder     eventrecord.EventRecorderLogger
		dvcrSettings *dvcr.Settings
	)

	BeforeEach(func() {
		ctx = logger.ToContext(context.TODO(), slog.Default())

		scheme = runtime.NewScheme()
		Expect(v1alpha2.AddToScheme(scheme)).To(Succeed())
		Expect(corev1.AddToScheme(scheme)).To(Succeed())
		Expect(storagev1.AddToScheme(scheme)).To(Succeed())
		Expect(netv1.AddToScheme(scheme)).To(Succeed())

		recorder = &eventrecord.EventRecorderLoggerMock{
			EventFunc: func(_ client.Object, _, _, _ string) {},
		}

		dvcrSettings = &dvcr.Settings{
			RegistryURL: "dvcr.example.com",
		}

		sc = &storagev1.StorageClass{
			ObjectMeta: metav1.ObjectMeta{Name: "sc"},
		}

		vd = &v1alpha2.VirtualDisk{
			ObjectMeta: metav1.ObjectMeta{
				Name:       "vd",
				Generation: 1,
				UID:        "44444444-4444-4444-4444-444444444444",
			},
			Spec: v1alpha2.VirtualDiskSpec{
				DataSource: &v1alpha2.VirtualDiskDataSource{
					Type: v1alpha2.DataSourceTypeUpload,
				},
			},
			Status: v1alpha2.VirtualDiskStatus{
				StorageClassName: sc.Name,
				Target:           v1alpha2.DiskTarget{PersistentVolumeClaim: "test-pvc"},
			},
		}

		supgen := vdsupplements.NewGenerator(vd)
		pvc = &corev1.PersistentVolumeClaim{
			ObjectMeta: metav1.ObjectMeta{
				Name:      supgen.PersistentVolumeClaim().Name,
				Namespace: vd.Namespace,
			},
			Spec: corev1.PersistentVolumeClaimSpec{StorageClassName: ptr.To(sc.Name)},
			Status: corev1.PersistentVolumeClaimStatus{
				Capacity: corev1.ResourceList{corev1.ResourceStorage: resource.MustParse("1Gi")},
			},
		}

		disk = &UploadDataSourceDiskServiceMock{
			GetCapacityFunc: func(_ *corev1.PersistentVolumeClaim) string { return "1Gi" },
			GetPersistentVolumeClaimFunc: func(_ context.Context, _ supplements.Generator) (*corev1.PersistentVolumeClaim, error) {
				return pvc, nil
			},
			CleanUpFunc:            func(_ context.Context, _ supplements.Generator) (bool, error) { return false, nil },
			CleanUpSupplementsFunc: func(_ context.Context, _ supplements.Generator) (bool, error) { return false, nil },
		}

		uploaderSvc = &UploadDataSourceUploaderServiceMock{
			GetPodFunc:     func(_ context.Context, _ supplements.Generator) (*corev1.Pod, error) { return nil, nil },
			GetServiceFunc: func(_ context.Context, _ supplements.Generator) (*corev1.Service, error) { return nil, nil },
			GetIngressFunc: func(_ context.Context, _ supplements.Generator) (*netv1.Ingress, error) { return nil, nil },
			CleanUpFunc:    func(_ context.Context, _ supplements.Generator) (bool, error) { return false, nil },
			ProtectFunc: func(_ context.Context, _ supplements.Generator, _ *corev1.Pod, _ *corev1.Service, _ *netv1.Ingress) error {
				return nil
			},
			GetExternalURLFunc:  func(_ context.Context, _ *netv1.Ingress) string { return "https://upload.example.com" },
			GetInClusterURLFunc: func(_ context.Context, _ *corev1.Service) string { return "http://upload.svc/upload" },
		}

		stat = &UploadDataSourceStatServiceMock{
			GetDVCRImageNameFunc: func(_ *corev1.Pod) string { return "dvcr.example.com/cvi/vd:1" },
			GetSizeFunc: func(_ *corev1.Pod) v1alpha2.ImageStatusSize {
				return v1alpha2.ImageStatusSize{UnpackedBytes: "500Mi"}
			},
			GetFormatFunc:        func(_ *corev1.Pod) string { return "qcow2" },
			GetDownloadSpeedFunc: func(_ types.UID, _ *corev1.Pod) *v1alpha2.StatusSpeed { return nil },
			GetProgressFunc: func(_ types.UID, _ *corev1.Pod, prev string, _ ...service.GetProgressOption) string {
				if prev == "" {
					return "10%"
				}
				return prev
			},
			CheckPodFunc:        func(_ *corev1.Pod) error { return nil },
			IsUploadStartedFunc: func(_ types.UID, _ *corev1.Pod) bool { return false },
			IsUploaderReadyFunc: func(_ *corev1.Pod, _ *corev1.Service, _ *netv1.Ingress, _ *corev1.Secret) (bool, error) {
				return false, nil
			},
		}
	})

	newSyncer := func(c client.Client) *UploadDataSource {
		return NewUploadDataSource(recorder, stat, uploaderSvc, disk, dvcrSettings, c)
	}

	Context("VirtualDisk has just been created (no uploader supplements yet)", func() {
		BeforeEach(func() {
			disk.GetPersistentVolumeClaimFunc = func(_ context.Context, _ supplements.Generator) (*corev1.PersistentVolumeClaim, error) {
				return nil, nil
			}
		})

		It("creates the uploader supplements and sets DiskProvisioning", func() {
			var started bool
			uploaderSvc.StartFunc = func(_ context.Context, _ *uploader.Settings, _ client.Object, _ supplements.Generator, _ *datasource.CABundle, _ ...service.Option) error {
				started = true
				return nil
			}

			cl := fake.NewClientBuilder().WithScheme(scheme).Build()
			res, err := newSyncer(cl).Sync(ctx, vd)
			Expect(err).ToNot(HaveOccurred())
			Expect(res.RequeueAfter).ToNot(BeZero())

			Expect(started).To(BeTrue())
			Expect(vd.Status.Phase).To(Equal(v1alpha2.DiskProvisioning))
			ExpectCondition(vd, metav1.ConditionFalse, vdcondition.Provisioning, true)
			Expect(vd.Status.Progress).To(Equal("0%"))
		})

		It("propagates QuotaExceeded as DiskFailed/QuotaExceeded", func() {
			uploaderSvc.StartFunc = func(_ context.Context, _ *uploader.Settings, _ client.Object, _ supplements.Generator, _ *datasource.CABundle, _ ...service.Option) error {
				return errors.New("exceeded quota: storage requested but limit reached")
			}

			cl := fake.NewClientBuilder().WithScheme(scheme).Build()
			res, err := newSyncer(cl).Sync(ctx, vd)
			Expect(err).ToNot(HaveOccurred())
			Expect(res.IsZero()).To(BeTrue())

			Expect(vd.Status.Phase).To(Equal(v1alpha2.DiskFailed))
			ExpectCondition(vd, metav1.ConditionFalse, vdcondition.QuotaExceeded, true)
		})
	})

	Context("Uploader supplements exist, user has not uploaded yet", func() {
		var (
			pod *corev1.Pod
			svc *corev1.Service
			ing *netv1.Ingress
		)

		BeforeEach(func() {
			disk.GetPersistentVolumeClaimFunc = func(_ context.Context, _ supplements.Generator) (*corev1.PersistentVolumeClaim, error) {
				return nil, nil
			}
			pod = &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{Name: "uploader", Namespace: vd.Namespace},
				Status:     corev1.PodStatus{Phase: corev1.PodRunning},
			}
			svc = &corev1.Service{ObjectMeta: metav1.ObjectMeta{Name: "uploader-svc", Namespace: vd.Namespace}}
			ing = &netv1.Ingress{ObjectMeta: metav1.ObjectMeta{Name: "uploader-ing", Namespace: vd.Namespace}}

			uploaderSvc.GetPodFunc = func(_ context.Context, _ supplements.Generator) (*corev1.Pod, error) { return pod, nil }
			uploaderSvc.GetServiceFunc = func(_ context.Context, _ supplements.Generator) (*corev1.Service, error) { return svc, nil }
			uploaderSvc.GetIngressFunc = func(_ context.Context, _ supplements.Generator) (*netv1.Ingress, error) { return ing, nil }
		})

		It("reports WaitForUserUpload with ImageUploadURLs when the uploader is ready", func() {
			stat.IsUploaderReadyFunc = func(_ *corev1.Pod, _ *corev1.Service, _ *netv1.Ingress, _ *corev1.Secret) (bool, error) {
				return true, nil
			}

			cl := fake.NewClientBuilder().WithScheme(scheme).Build()
			res, err := newSyncer(cl).Sync(ctx, vd)
			Expect(err).ToNot(HaveOccurred())
			Expect(res.RequeueAfter).ToNot(BeZero())

			Expect(vd.Status.Phase).To(Equal(v1alpha2.DiskWaitForUserUpload))
			ExpectCondition(vd, metav1.ConditionFalse, vdcondition.WaitForUserUpload, true)
			Expect(vd.Status.ImageUploadURLs).ToNot(BeNil())
			Expect(vd.Status.ImageUploadURLs.External).ToNot(BeEmpty())
			Expect(vd.Status.ImageUploadURLs.InCluster).ToNot(BeEmpty())
		})

		It("reports Provisioning while the uploader is not yet ready", func() {
			stat.IsUploaderReadyFunc = func(_ *corev1.Pod, _ *corev1.Service, _ *netv1.Ingress, _ *corev1.Secret) (bool, error) {
				return false, nil
			}

			cl := fake.NewClientBuilder().WithScheme(scheme).Build()
			res, err := newSyncer(cl).Sync(ctx, vd)
			Expect(err).ToNot(HaveOccurred())
			Expect(res.RequeueAfter).ToNot(BeZero())

			Expect(vd.Status.Phase).To(Equal(v1alpha2.DiskProvisioning))
			ExpectCondition(vd, metav1.ConditionFalse, vdcondition.Provisioning, true)
		})
	})

	Context("User upload is in progress", func() {
		BeforeEach(func() {
			disk.GetPersistentVolumeClaimFunc = func(_ context.Context, _ supplements.Generator) (*corev1.PersistentVolumeClaim, error) {
				return nil, nil
			}
			pod := &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{Name: "uploader", Namespace: vd.Namespace},
				Status:     corev1.PodStatus{Phase: corev1.PodRunning},
			}
			uploaderSvc.GetPodFunc = func(_ context.Context, _ supplements.Generator) (*corev1.Pod, error) { return pod, nil }
			uploaderSvc.GetServiceFunc = func(_ context.Context, _ supplements.Generator) (*corev1.Service, error) {
				return &corev1.Service{ObjectMeta: metav1.ObjectMeta{Name: "uploader-svc", Namespace: vd.Namespace}}, nil
			}
			uploaderSvc.GetIngressFunc = func(_ context.Context, _ supplements.Generator) (*netv1.Ingress, error) {
				return &netv1.Ingress{ObjectMeta: metav1.ObjectMeta{Name: "uploader-ing", Namespace: vd.Namespace}}, nil
			}
			stat.IsUploadStartedFunc = func(_ types.UID, _ *corev1.Pod) bool { return true }
		})

		It("reports DiskProvisioning while uploading to DVCR and protects the uploader", func() {
			cl := fake.NewClientBuilder().WithScheme(scheme).Build()
			res, err := newSyncer(cl).Sync(ctx, vd)
			Expect(err).ToNot(HaveOccurred())
			Expect(res.RequeueAfter).ToNot(BeZero())

			Expect(vd.Status.Phase).To(Equal(v1alpha2.DiskProvisioning))
			ExpectCondition(vd, metav1.ConditionFalse, vdcondition.Provisioning, true)
			Expect(uploaderSvc.ProtectCalls()).To(HaveLen(1))
		})

		It("keeps DiskProvisioning on transient uploader pod errors after upload has started", func() {
			stat.CheckPodFunc = func(_ *corev1.Pod) error { return service.ErrNotScheduled }

			cl := fake.NewClientBuilder().WithScheme(scheme).Build()
			res, err := newSyncer(cl).Sync(ctx, vd)
			Expect(err).ToNot(HaveOccurred())
			Expect(res.RequeueAfter).ToNot(BeZero())

			Expect(vd.Status.Phase).To(Equal(v1alpha2.DiskProvisioning))
			ExpectCondition(vd, metav1.ConditionFalse, vdcondition.Provisioning, true)
		})
	})

	Context("Uploader pod has completed, no PVC yet", func() {
		BeforeEach(func() {
			disk.GetPersistentVolumeClaimFunc = func(_ context.Context, _ supplements.Generator) (*corev1.PersistentVolumeClaim, error) {
				return nil, nil
			}
			pod := &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{Name: "uploader", Namespace: vd.Namespace},
				Status:     corev1.PodStatus{Phase: corev1.PodSucceeded},
			}
			uploaderSvc.GetPodFunc = func(_ context.Context, _ supplements.Generator) (*corev1.Pod, error) { return pod, nil }
			uploaderSvc.GetServiceFunc = func(_ context.Context, _ supplements.Generator) (*corev1.Service, error) {
				return &corev1.Service{ObjectMeta: metav1.ObjectMeta{Name: "uploader-svc", Namespace: vd.Namespace}}, nil
			}
			uploaderSvc.GetIngressFunc = func(_ context.Context, _ supplements.Generator) (*netv1.Ingress, error) {
				return &netv1.Ingress{ObjectMeta: metav1.ObjectMeta{Name: "uploader-ing", Namespace: vd.Namespace}}, nil
			}
		})

		It("starts the PVC import using a registry source", func() {
			var started bool
			disk.StartPVCImportFunc = func(_ context.Context, _ resource.Quantity, _ *storagev1.StorageClass, source *service.PVCImportSource, _ *v1alpha2.VirtualDisk, _ *provisioner.NodePlacement) error {
				started = true
				Expect(source).ToNot(BeNil())
				Expect(source.Registry).ToNot(BeNil())
				return nil
			}

			cl := fake.NewClientBuilder().WithScheme(scheme).WithObjects(sc).Build()
			res, err := newSyncer(cl).Sync(ctx, vd)
			Expect(err).ToNot(HaveOccurred())
			Expect(res.IsZero()).To(BeTrue())

			Expect(started).To(BeTrue())
			Expect(vd.Status.Phase).To(Equal(v1alpha2.DiskProvisioning))
			ExpectCondition(vd, metav1.ConditionFalse, vdcondition.Provisioning, true)
		})

		It("fails the disk when the uploaded source is an ISO", func() {
			stat.GetFormatFunc = func(_ *corev1.Pod) string { return "iso" }

			cl := fake.NewClientBuilder().WithScheme(scheme).WithObjects(sc).Build()
			res, err := newSyncer(cl).Sync(ctx, vd)
			Expect(err).ToNot(HaveOccurred())
			Expect(res.IsZero()).To(BeTrue())

			Expect(vd.Status.Phase).To(Equal(v1alpha2.DiskFailed))
			ExpectCondition(vd, metav1.ConditionFalse, vdcondition.ProvisioningFailed, true)
		})
	})

	Context("PVC is Bound and the import is complete", func() {
		BeforeEach(func() {
			pvc.Status.Phase = corev1.ClaimBound
			pvc.Annotations = map[string]string{annotations.AnnPVCImportPhase: string(corev1.PodSucceeded)}
		})

		It("marks DiskReady and cleans up the uploader once the condition is finished", func() {
			vd.Status.Conditions = []metav1.Condition{{
				Type:   vdcondition.ReadyType.String(),
				Reason: vdcondition.Ready.String(),
				Status: metav1.ConditionTrue,
			}}
			pod := &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{Name: "uploader", Namespace: vd.Namespace},
				Status:     corev1.PodStatus{Phase: corev1.PodSucceeded},
			}
			uploaderSvc.GetPodFunc = func(_ context.Context, _ supplements.Generator) (*corev1.Pod, error) { return pod, nil }
			uploaderSvc.GetServiceFunc = func(_ context.Context, _ supplements.Generator) (*corev1.Service, error) {
				return &corev1.Service{ObjectMeta: metav1.ObjectMeta{Name: "uploader-svc", Namespace: vd.Namespace}}, nil
			}
			uploaderSvc.GetIngressFunc = func(_ context.Context, _ supplements.Generator) (*netv1.Ingress, error) {
				return &netv1.Ingress{ObjectMeta: metav1.ObjectMeta{Name: "uploader-ing", Namespace: vd.Namespace}}, nil
			}
			var cleaned bool
			uploaderSvc.CleanUpFunc = func(_ context.Context, _ supplements.Generator) (bool, error) {
				cleaned = true
				return true, nil
			}

			cl := fake.NewClientBuilder().WithScheme(scheme).WithObjects(pvc).Build()
			res, err := newSyncer(cl).Sync(ctx, vd)
			Expect(err).ToNot(HaveOccurred())
			Expect(res.IsZero()).To(BeTrue())

			Expect(cleaned).To(BeTrue())
			Expect(vd.Status.Phase).To(Equal(v1alpha2.DiskReady))
			ExpectCondition(vd, metav1.ConditionTrue, vdcondition.Ready, false)
			ExpectStats(vd)
		})
	})

	Context("CleanUp", func() {
		It("delegates to both uploader and disk services", func() {
			var uploaderCleaned, diskCleaned bool
			uploaderSvc.CleanUpFunc = func(_ context.Context, _ supplements.Generator) (bool, error) {
				uploaderCleaned = true
				return false, nil
			}
			disk.CleanUpFunc = func(_ context.Context, _ supplements.Generator) (bool, error) {
				diskCleaned = true
				return true, nil
			}

			cl := fake.NewClientBuilder().WithScheme(scheme).Build()
			requeue, err := newSyncer(cl).CleanUp(ctx, vd)
			Expect(err).ToNot(HaveOccurred())
			Expect(requeue).To(BeTrue())
			Expect(uploaderCleaned).To(BeTrue())
			Expect(diskCleaned).To(BeTrue())
		})
	})

	Context("Validate", func() {
		It("is a no-op", func() {
			cl := fake.NewClientBuilder().WithScheme(scheme).Build()
			Expect(newSyncer(cl).Validate(ctx, vd)).To(Succeed())
		})
	})
})
