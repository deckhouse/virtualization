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
	"github.com/deckhouse/virtualization-controller/pkg/controller/importer"
	"github.com/deckhouse/virtualization-controller/pkg/controller/service"
	"github.com/deckhouse/virtualization-controller/pkg/controller/supplements"
	vdsupplements "github.com/deckhouse/virtualization-controller/pkg/controller/vd/internal/supplements"
	"github.com/deckhouse/virtualization-controller/pkg/dvcr"
	"github.com/deckhouse/virtualization-controller/pkg/eventrecord"
	"github.com/deckhouse/virtualization-controller/pkg/logger"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/vdcondition"
)

var _ = Describe("HTTPDataSource", func() {
	var (
		ctx          context.Context
		scheme       *runtime.Scheme
		vd           *v1alpha2.VirtualDisk
		sc           *storagev1.StorageClass
		pvc          *corev1.PersistentVolumeClaim
		disk         *HTTPDataSourceDiskServiceMock
		importerSvc  *HTTPDataSourceImporterServiceMock
		stat         *HTTPDataSourceStatServiceMock
		recorder     eventrecord.EventRecorderLogger
		dvcrSettings *dvcr.Settings
	)

	BeforeEach(func() {
		ctx = logger.ToContext(context.TODO(), slog.Default())

		scheme = runtime.NewScheme()
		Expect(v1alpha2.AddToScheme(scheme)).To(Succeed())
		Expect(corev1.AddToScheme(scheme)).To(Succeed())
		Expect(storagev1.AddToScheme(scheme)).To(Succeed())

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
				UID:        "22222222-2222-2222-2222-222222222222",
			},
			Spec: v1alpha2.VirtualDiskSpec{
				DataSource: &v1alpha2.VirtualDiskDataSource{
					Type: v1alpha2.DataSourceTypeHTTP,
					HTTP: &v1alpha2.DataSourceHTTP{URL: "https://example.com/image.qcow2"},
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

		disk = &HTTPDataSourceDiskServiceMock{
			GetCapacityFunc: func(_ *corev1.PersistentVolumeClaim) string { return "1Gi" },
			GetPersistentVolumeClaimFunc: func(_ context.Context, _ supplements.Generator) (*corev1.PersistentVolumeClaim, error) {
				return pvc, nil
			},
			CleanUpFunc:            func(_ context.Context, _ supplements.Generator) (bool, error) { return false, nil },
			CleanUpSupplementsFunc: func(_ context.Context, _ supplements.Generator) (bool, error) { return false, nil },
			ProtectFunc: func(_ context.Context, _ supplements.Generator, _ client.Object, _ *corev1.PersistentVolumeClaim) error {
				return nil
			},
		}

		importerSvc = &HTTPDataSourceImporterServiceMock{
			GetPodFunc:  func(_ context.Context, _ supplements.Generator) (*corev1.Pod, error) { return nil, nil },
			CleanUpFunc: func(_ context.Context, _ supplements.Generator) (bool, error) { return false, nil },
			ProtectFunc: func(_ context.Context, _ *corev1.Pod, _ supplements.Generator) error { return nil },
		}

		stat = &HTTPDataSourceStatServiceMock{
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
			CheckPodFunc: func(_ *corev1.Pod) error { return nil },
		}
	})

	newSyncer := func(c client.Client) *HTTPDataSource {
		return NewHTTPDataSource(recorder, stat, importerSvc, disk, dvcrSettings, c)
	}

	Context("VirtualDisk has just been created (no importer pod yet)", func() {
		It("starts the importer pod and sets DiskProvisioning", func() {
			disk.GetPersistentVolumeClaimFunc = func(_ context.Context, _ supplements.Generator) (*corev1.PersistentVolumeClaim, error) {
				return nil, nil
			}
			var started bool
			var gotSettings *importer.Settings
			importerSvc.StartFunc = func(_ context.Context, settings *importer.Settings, _ client.Object, _ supplements.Generator, _ *datasource.CABundle, _ ...service.Option) error {
				started = true
				gotSettings = settings
				return nil
			}

			cl := fake.NewClientBuilder().WithScheme(scheme).Build()
			res, err := newSyncer(cl).Sync(ctx, vd)
			Expect(err).ToNot(HaveOccurred())
			Expect(res.Requeue).To(BeTrue())

			Expect(started).To(BeTrue())
			Expect(gotSettings).ToNot(BeNil())

			ExpectCondition(vd, metav1.ConditionFalse, vdcondition.Provisioning, true)
			Expect(vd.Status.Phase).To(Equal(v1alpha2.DiskProvisioning))
			Expect(vd.Status.Progress).To(Equal("0%"))
		})

		It("propagates QuotaExceeded as DiskFailed/QuotaExceeded", func() {
			disk.GetPersistentVolumeClaimFunc = func(_ context.Context, _ supplements.Generator) (*corev1.PersistentVolumeClaim, error) {
				return nil, nil
			}
			importerSvc.StartFunc = func(_ context.Context, _ *importer.Settings, _ client.Object, _ supplements.Generator, _ *datasource.CABundle, _ ...service.Option) error {
				return errors.New("exceeded quota: cpu requested but limit reached")
			}

			cl := fake.NewClientBuilder().WithScheme(scheme).Build()
			res, err := newSyncer(cl).Sync(ctx, vd)
			Expect(err).ToNot(HaveOccurred())
			Expect(res.IsZero()).To(BeTrue())

			Expect(vd.Status.Phase).To(Equal(v1alpha2.DiskFailed))
			ExpectCondition(vd, metav1.ConditionFalse, vdcondition.QuotaExceeded, true)
		})
	})

	Context("Importer pod is running", func() {
		var pod *corev1.Pod

		BeforeEach(func() {
			disk.GetPersistentVolumeClaimFunc = func(_ context.Context, _ supplements.Generator) (*corev1.PersistentVolumeClaim, error) {
				return nil, nil
			}
			pod = &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{Name: "importer", Namespace: vd.Namespace},
				Status:     corev1.PodStatus{Phase: corev1.PodRunning},
			}
			importerSvc.GetPodFunc = func(_ context.Context, _ supplements.Generator) (*corev1.Pod, error) { return pod, nil }
		})

		It("marks Provisioning and updates progress/download speed", func() {
			cl := fake.NewClientBuilder().WithScheme(scheme).Build()
			res, err := newSyncer(cl).Sync(ctx, vd)
			Expect(err).ToNot(HaveOccurred())
			Expect(res.RequeueAfter).ToNot(BeZero())

			Expect(vd.Status.Phase).To(Equal(v1alpha2.DiskProvisioning))
			ExpectCondition(vd, metav1.ConditionFalse, vdcondition.Provisioning, true)
			Expect(vd.Status.Progress).ToNot(BeEmpty())
			Expect(importerSvc.ProtectCalls()).To(HaveLen(1))
		})

		It("surfaces ProvisioningFailed when CheckPod returns ErrProvisioningFailed", func() {
			stat.CheckPodFunc = func(_ *corev1.Pod) error { return service.ErrProvisioningFailed }

			cl := fake.NewClientBuilder().WithScheme(scheme).Build()
			res, err := newSyncer(cl).Sync(ctx, vd)
			Expect(err).ToNot(HaveOccurred())
			Expect(res.IsZero()).To(BeTrue())

			Expect(vd.Status.Phase).To(Equal(v1alpha2.DiskFailed))
			ExpectCondition(vd, metav1.ConditionFalse, vdcondition.ProvisioningFailed, true)
		})
	})

	Context("Importer pod completed, no PVC yet", func() {
		var pod *corev1.Pod

		BeforeEach(func() {
			disk.GetPersistentVolumeClaimFunc = func(_ context.Context, _ supplements.Generator) (*corev1.PersistentVolumeClaim, error) {
				return nil, nil
			}
			pod = &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{Name: "importer", Namespace: vd.Namespace},
				Status:     corev1.PodStatus{Phase: corev1.PodSucceeded},
			}
			importerSvc.GetPodFunc = func(_ context.Context, _ supplements.Generator) (*corev1.Pod, error) { return pod, nil }
		})

		It("kicks off the PVC import", func() {
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
			ExpectCondition(vd, metav1.ConditionFalse, vdcondition.Provisioning, true)
			Expect(vd.Status.Phase).To(Equal(v1alpha2.DiskProvisioning))
		})

		It("fails the disk when the source is ISO", func() {
			stat.GetFormatFunc = func(_ *corev1.Pod) string { return "iso" }

			cl := fake.NewClientBuilder().WithScheme(scheme).WithObjects(sc).Build()
			res, err := newSyncer(cl).Sync(ctx, vd)
			Expect(err).ToNot(HaveOccurred())
			Expect(res.IsZero()).To(BeTrue())

			Expect(vd.Status.Phase).To(Equal(v1alpha2.DiskFailed))
			ExpectCondition(vd, metav1.ConditionFalse, vdcondition.ProvisioningFailed, true)
		})
	})

	Context("PVC is created but not yet Bound", func() {
		var pod *corev1.Pod

		BeforeEach(func() {
			pod = &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{Name: "importer", Namespace: vd.Namespace},
				Status:     corev1.PodStatus{Phase: corev1.PodSucceeded},
			}
			importerSvc.GetPodFunc = func(_ context.Context, _ supplements.Generator) (*corev1.Pod, error) { return pod, nil }
		})

		It("reports WaitForFirstConsumer for WFFC storage class", func() {
			pvc.Status.Phase = corev1.ClaimPending
			sc.VolumeBindingMode = ptr.To(storagev1.VolumeBindingWaitForFirstConsumer)

			cl := fake.NewClientBuilder().WithScheme(scheme).WithObjects(pvc, sc).Build()
			res, err := newSyncer(cl).Sync(ctx, vd)
			Expect(err).ToNot(HaveOccurred())
			Expect(res.IsZero()).To(BeTrue())

			Expect(vd.Status.Phase).To(Equal(v1alpha2.DiskWaitForFirstConsumer))
			ExpectCondition(vd, metav1.ConditionFalse, vdcondition.WaitingForFirstConsumer, true)
		})

		It("reports Provisioning while waiting for Bound", func() {
			pvc.Status.Phase = corev1.ClaimPending
			sc.VolumeBindingMode = ptr.To(storagev1.VolumeBindingImmediate)

			cl := fake.NewClientBuilder().WithScheme(scheme).WithObjects(pvc, sc).Build()
			res, err := newSyncer(cl).Sync(ctx, vd)
			Expect(err).ToNot(HaveOccurred())
			Expect(res.IsZero()).To(BeTrue())

			Expect(vd.Status.Phase).To(Equal(v1alpha2.DiskProvisioning))
			ExpectCondition(vd, metav1.ConditionFalse, vdcondition.Provisioning, true)
		})
	})

	Context("PVC is Bound and the import is still in flight", func() {
		BeforeEach(func() {
			pvc.Status.Phase = corev1.ClaimBound
			pvc.Annotations = map[string]string{annotations.AnnPVCImportPhase: string(corev1.PodRunning)}
			pod := &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{Name: "importer", Namespace: vd.Namespace},
				Status:     corev1.PodStatus{Phase: corev1.PodSucceeded},
			}
			importerSvc.GetPodFunc = func(_ context.Context, _ supplements.Generator) (*corev1.Pod, error) { return pod, nil }
		})

		It("drives EnsurePVCImport with a registry source built from the importer pod", func() {
			disk.EnsurePVCImportFunc = func(_ context.Context, _ *corev1.PersistentVolumeClaim, source *service.PVCImportSource, _ *v1alpha2.VirtualDisk, _ *provisioner.NodePlacement) (corev1.PodPhase, error) {
				Expect(source).ToNot(BeNil())
				Expect(source.Registry).ToNot(BeNil())
				return corev1.PodRunning, nil
			}

			cl := fake.NewClientBuilder().WithScheme(scheme).WithObjects(pvc).Build()
			res, err := newSyncer(cl).Sync(ctx, vd)
			Expect(err).ToNot(HaveOccurred())
			Expect(res.IsZero()).To(BeTrue())

			Expect(vd.Status.Phase).To(Equal(v1alpha2.DiskProvisioning))
			ExpectCondition(vd, metav1.ConditionFalse, vdcondition.Provisioning, true)
		})

		It("requeues when EnsurePVCImport reports Succeeded", func() {
			disk.EnsurePVCImportFunc = func(_ context.Context, _ *corev1.PersistentVolumeClaim, _ *service.PVCImportSource, _ *v1alpha2.VirtualDisk, _ *provisioner.NodePlacement) (corev1.PodPhase, error) {
				return corev1.PodSucceeded, nil
			}

			cl := fake.NewClientBuilder().WithScheme(scheme).WithObjects(pvc).Build()
			res, err := newSyncer(cl).Sync(ctx, vd)
			Expect(err).ToNot(HaveOccurred())
			Expect(res.Requeue).To(BeTrue())
		})
	})

	Context("PVC is Bound and the import is complete", func() {
		BeforeEach(func() {
			pvc.Status.Phase = corev1.ClaimBound
			pvc.Annotations = map[string]string{annotations.AnnPVCImportPhase: string(corev1.PodSucceeded)}
		})

		It("marks DiskReady and cleans up the importer once the condition is finished", func() {
			vd.Status.Conditions = []metav1.Condition{{
				Type:   vdcondition.ReadyType.String(),
				Reason: vdcondition.Ready.String(),
				Status: metav1.ConditionTrue,
			}}
			pod := &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{Name: "importer", Namespace: vd.Namespace},
				Status:     corev1.PodStatus{Phase: corev1.PodSucceeded},
			}
			importerSvc.GetPodFunc = func(_ context.Context, _ supplements.Generator) (*corev1.Pod, error) { return pod, nil }
			var cleaned bool
			importerSvc.CleanUpFunc = func(_ context.Context, _ supplements.Generator) (bool, error) {
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
		It("delegates to both importer and disk services", func() {
			var importerCleaned, diskCleaned bool
			importerSvc.CleanUpFunc = func(_ context.Context, _ supplements.Generator) (bool, error) {
				importerCleaned = true
				return true, nil
			}
			disk.CleanUpFunc = func(_ context.Context, _ supplements.Generator) (bool, error) {
				diskCleaned = true
				return false, nil
			}

			cl := fake.NewClientBuilder().WithScheme(scheme).Build()
			requeue, err := newSyncer(cl).CleanUp(ctx, vd)
			Expect(err).ToNot(HaveOccurred())
			Expect(requeue).To(BeTrue())
			Expect(importerCleaned).To(BeTrue())
			Expect(diskCleaned).To(BeTrue())
		})
	})
})
