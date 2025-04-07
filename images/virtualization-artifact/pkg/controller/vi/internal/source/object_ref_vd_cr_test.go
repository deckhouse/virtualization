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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/deckhouse/virtualization-controller/pkg/common/datasource"
	importersettings "github.com/deckhouse/virtualization-controller/pkg/controller/importer"
	"github.com/deckhouse/virtualization-controller/pkg/controller/supplements"
	"github.com/deckhouse/virtualization-controller/pkg/dvcr"
	"github.com/deckhouse/virtualization-controller/pkg/eventrecord"
	"github.com/deckhouse/virtualization-controller/pkg/logger"
	virtv2 "github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/vicondition"
)

var _ = Describe("ObjectRef VirtualDisk ContainerRegistry", func() {
	var (
		ctx      context.Context
		scheme   *runtime.Scheme
		vi       *virtv2.VirtualImage
		vd       *virtv2.VirtualDisk
		pod      *corev1.Pod
		settings *dvcr.Settings
		recorder eventrecord.EventRecorderLogger
		importer *ImporterMock
		disk     *DiskMock
		stat     *StatMock
	)

	BeforeEach(func() {
		ctx = logger.ToContext(context.TODO(), slog.Default())

		scheme = runtime.NewScheme()
		Expect(virtv2.AddToScheme(scheme)).To(Succeed())
		Expect(corev1.AddToScheme(scheme)).To(Succeed())

		importer, disk, stat, recorder, _ = getServiceMocks()
		settings = &dvcr.Settings{}

		vd = getVirtualDisk()
		vi = getVirtualImage(virtv2.StorageContainerRegistry, virtv2.VirtualImageDataSource{
			Type: virtv2.DataSourceTypeObjectRef,
			ObjectRef: &virtv2.VirtualImageObjectRef{
				Kind: virtv2.VirtualImageObjectRefKindVirtualDisk,
				Name: vd.Name,
			},
		})
		pod = getPod(vi)
	})

	Context("VirtualImage has just been created", func() {
		It("must create Pod", func() {
			var podCreated bool
			importer.StartWithPodSettingFunc = func(_ context.Context, _ *importersettings.Settings, _ *supplements.Generator, _ *datasource.CABundle, _ *importersettings.PodSettings) error {
				podCreated = true
				return nil
			}

			client := fake.NewClientBuilder().WithScheme(scheme).WithObjects(vd).Build()

			vi.Status = virtv2.VirtualImageStatus{}

			syncer := NewObjectRefVirtualDiskCR(client, importer, nil, stat, settings, recorder)

			res, err := syncer.Sync(ctx, vi)
			Expect(err).ToNot(HaveOccurred())
			Expect(res.IsZero()).To(BeTrue())

			Expect(podCreated).To(BeTrue())

			ExpectCondition(vi, metav1.ConditionFalse, vicondition.Provisioning, true)
			Expect(vi.Status.SourceUID).ToNot(BeNil())
			Expect(*vi.Status.SourceUID).ToNot(BeEmpty())
			Expect(vi.Status.Phase).To(Equal(virtv2.ImageProvisioning))
			Expect(vi.Status.Target.RegistryURL).ToNot(BeEmpty())
			Expect(vi.Status.Target.PersistentVolumeClaim).To(BeEmpty())
		})
	})

	Context("VirtualImage waits for the Pod to be Completed", func() {
		It("waits for the Pod to be Running", func() {
			pod.Status.Phase = corev1.PodPending
			client := fake.NewClientBuilder().WithScheme(scheme).WithObjects(vd, pod).Build()

			syncer := NewObjectRefVirtualDiskCR(client, importer, nil, stat, settings, recorder)

			res, err := syncer.Sync(ctx, vi)
			Expect(err).ToNot(HaveOccurred())
			Expect(res.IsZero()).To(BeTrue())

			ExpectCondition(vi, metav1.ConditionFalse, vicondition.Provisioning, true)
			Expect(vi.Status.Phase).To(Equal(virtv2.ImageProvisioning))
		})

		It("waits for the Pod to be Succeeded", func() {
			pod.Status.Phase = corev1.PodRunning
			client := fake.NewClientBuilder().WithScheme(scheme).WithObjects(vd, pod).Build()

			syncer := NewObjectRefVirtualDiskCR(client, importer, nil, stat, settings, recorder)

			res, err := syncer.Sync(ctx, vi)
			Expect(err).ToNot(HaveOccurred())
			Expect(res.RequeueAfter).ToNot(BeZero())

			ExpectCondition(vi, metav1.ConditionFalse, vicondition.Provisioning, true)
			Expect(vi.Status.Phase).To(Equal(virtv2.ImageProvisioning))
		})
	})

	Context("VirtualImage is ready", func() {
		It("has Pod in Succeeded phase", func() {
			pod.Status.Phase = corev1.PodSucceeded
			client := fake.NewClientBuilder().WithScheme(scheme).WithObjects(pod).Build()

			syncer := NewObjectRefVirtualDiskCR(client, importer, disk, stat, settings, recorder)

			res, err := syncer.Sync(ctx, vi)
			Expect(err).ToNot(HaveOccurred())
			Expect(res.IsZero()).To(BeTrue())

			ExpectCondition(vi, metav1.ConditionTrue, vicondition.Ready, false)
			Expect(vi.Status.Phase).To(Equal(virtv2.ImageReady))
		})

		It("does not have Pod", func() {
			vi.Status.Conditions = []metav1.Condition{
				{
					Type:   vicondition.ReadyType.String(),
					Status: metav1.ConditionTrue,
					Reason: vicondition.Ready.String(),
				},
			}

			client := fake.NewClientBuilder().WithScheme(scheme).WithObjects().Build()

			syncer := NewObjectRefVirtualDiskCR(client, importer, nil, stat, settings, recorder)

			res, err := syncer.Sync(ctx, vi)
			Expect(err).ToNot(HaveOccurred())
			Expect(res.IsZero()).To(BeTrue())

			ExpectCondition(vi, metav1.ConditionTrue, vicondition.Ready, false)
			Expect(vi.Status.Phase).To(Equal(virtv2.ImageReady))
		})
	})
})
