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
	cdiv1 "kubevirt.io/containerized-data-importer-api/pkg/apis/core/v1beta1"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/deckhouse/virtualization-controller/pkg/controller/service"
	"github.com/deckhouse/virtualization-controller/pkg/controller/supplements"
	"github.com/deckhouse/virtualization-controller/pkg/eventrecord"
	"github.com/deckhouse/virtualization-controller/pkg/logger"
	virtv2 "github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/vicondition"
)

var _ = Describe("ObjectRef VirtualDisk PersistentVolumeClaim", func() {
	var (
		ctx      context.Context
		scheme   *runtime.Scheme
		vi       *virtv2.VirtualImage
		vd       *virtv2.VirtualDisk
		pvc      *corev1.PersistentVolumeClaim
		recorder eventrecord.EventRecorderLogger
		bounder  *BounderMock
		disk     *DiskMock
	)

	BeforeEach(func() {
		ctx = logger.ToContext(context.TODO(), slog.Default())

		scheme = runtime.NewScheme()
		Expect(virtv2.AddToScheme(scheme)).To(Succeed())
		Expect(corev1.AddToScheme(scheme)).To(Succeed())
		Expect(cdiv1.AddToScheme(scheme)).To(Succeed())

		_, disk, _, recorder, bounder = getServiceMocks()

		vd = getVirtualDisk()
		vi = getVirtualImage(virtv2.StoragePersistentVolumeClaim, virtv2.VirtualImageDataSource{
			Type: virtv2.DataSourceTypeObjectRef,
			ObjectRef: &virtv2.VirtualImageObjectRef{
				Kind: virtv2.VirtualImageObjectRefKindVirtualDisk,
				Name: vd.Name,
			},
		})
		pvc = getPVC(vi, "")
	})

	Context("VirtualImage has just been created", func() {
		It("must create DV", func() {
			vi.Status = virtv2.VirtualImageStatus{}
			client := fake.NewClientBuilder().WithScheme(scheme).WithObjects(vd).Build()

			var started bool
			disk.StartImmediateFunc = func(_ context.Context, _ resource.Quantity, _ *storagev1.StorageClass, _ *cdiv1.DataVolumeSource, _ service.ObjectKind, _ *supplements.Generator) error {
				started = true
				return nil
			}

			syncer := NewObjectRefVirtualDiskPVC(bounder, client, disk, recorder)

			res, err := syncer.Sync(ctx, vi)
			Expect(err).ToNot(HaveOccurred())
			Expect(res.IsZero()).To(BeTrue())

			Expect(started).To(BeTrue())

			ExpectCondition(vi, metav1.ConditionFalse, vicondition.Provisioning, true)
			Expect(vi.Status.SourceUID).ToNot(BeNil())
			Expect(*vi.Status.SourceUID).ToNot(BeEmpty())
			Expect(vi.Status.Phase).To(Equal(virtv2.ImageProvisioning))
			Expect(vi.Status.Target.RegistryURL).To(BeEmpty())
			Expect(vi.Status.Target.PersistentVolumeClaim).NotTo(BeEmpty())
		})
	})

	Context("VirtualImage is ready", func() {
		It("has PVC in Bound phase", func() {
			pvc.Status.Phase = corev1.ClaimBound
			client := fake.NewClientBuilder().WithScheme(scheme).WithObjects(pvc).Build()

			syncer := NewObjectRefVirtualDiskPVC(bounder, client, disk, recorder)

			res, err := syncer.Sync(ctx, vi)
			Expect(err).ToNot(HaveOccurred())
			Expect(res.IsZero()).To(BeTrue())

			ExpectCondition(vi, metav1.ConditionTrue, vicondition.Ready, false)
			Expect(vi.Status.Phase).To(Equal(virtv2.ImageReady))
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

			syncer := NewObjectRefVirtualDiskPVC(bounder, client, disk, recorder)

			res, err := syncer.Sync(ctx, vi)
			Expect(err).ToNot(HaveOccurred())
			Expect(res.IsZero()).To(BeTrue())

			ExpectCondition(vi, metav1.ConditionFalse, vicondition.Lost, true)
			Expect(vi.Status.Phase).To(Equal(virtv2.ImageLost))
			Expect(vi.Status.Target.PersistentVolumeClaim).NotTo(BeEmpty())
		})

		It("is lost when PVC is lost as well", func() {
			pvc.Status.Phase = corev1.ClaimLost
			vi.Status.Target.PersistentVolumeClaim = pvc.Name
			client := fake.NewClientBuilder().WithScheme(scheme).WithObjects(pvc).Build()

			syncer := NewObjectRefVirtualDiskPVC(bounder, client, disk, recorder)

			res, err := syncer.Sync(ctx, vi)
			Expect(err).ToNot(HaveOccurred())
			Expect(res.IsZero()).To(BeTrue())

			ExpectCondition(vi, metav1.ConditionFalse, vicondition.Lost, true)
			Expect(vi.Status.Phase).To(Equal(virtv2.ImageLost))
			Expect(vi.Status.Target.PersistentVolumeClaim).NotTo(BeEmpty())
		})
	})
})
