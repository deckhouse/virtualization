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

package blockdevice

import (
	"context"
	"fmt"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	cvibuilder "github.com/deckhouse/virtualization-controller/pkg/builder/cvi"
	vdbuilder "github.com/deckhouse/virtualization-controller/pkg/builder/vd"
	vibuilder "github.com/deckhouse/virtualization-controller/pkg/builder/vi"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/test/e2e/internal/framework"
	"github.com/deckhouse/virtualization/test/e2e/internal/object"
	cviobs "github.com/deckhouse/virtualization/test/e2e/internal/observer/cvi"
	vdobs "github.com/deckhouse/virtualization/test/e2e/internal/observer/vd"
	viobs "github.com/deckhouse/virtualization/test/e2e/internal/observer/vi"
	"github.com/deckhouse/virtualization/test/e2e/internal/precheck"
)

// quotaExhaustedQuotaName is the name of the project ResourceQuota that
// blocks every Pod and PVC creation in the test namespace, forcing the
// virtualization-controller to surface the quota-exceeded condition on
// every newly created VirtualDisk, VirtualImage, and ClusterVirtualImage.
const quotaExhaustedQuotaName = "v12n-e2e-block-pods-and-pvcs"

var _ = Describe("QuotaExhausted", Ordered, Label(precheck.PrecheckMainStandbyStorageClass), func() {
	var (
		f   *framework.Framework
		ctx context.Context

		scPtr  *string
		baseVD *v1alpha2.VirtualDisk
	)

	BeforeAll(func() {
		ctx = context.Background()
		f = framework.NewFramework("")
		f.Before()
		DeferCleanup(f.After)
		setupProject(ctx, f, "quota-exhausted")

		scPtr = mainStorageClass()

		// Create a base VirtualDisk before applying the quota so that
		// the ClusterVirtualImage spec below has a valid object-ref
		// source to reference. Sourcing a CVI from a VD on PVC makes the
		// CVI importer Pod run in the user's namespace, which is exactly
		// what we need to exercise the user-namespace quota path for
		// CVIs. See ImporterService.GetPodSettingsWithPVC.
		baseVD = vdbuilder.New(
			vdbuilder.WithName("vd-quota-source"),
			vdbuilder.WithNamespace(f.Namespace().Name),
			vdbuilder.WithDataSourceObjectRef(v1alpha2.VirtualDiskObjectRefKindClusterVirtualImage, object.PrecreatedCVITestDataQCOW),
			vdbuilder.WithStorageClass(scPtr),
		)
		createVirtualDiskAndWait(ctx, f, baseVD)

		applyBlockingResourceQuota(ctx, f)
	})

	It("VirtualDisk reports QuotaExceeded reason on a fresh Ready condition", Label(precheck.PrecheckMainStandbyStorageClass), func() {
		vd := vdbuilder.New(
			vdbuilder.WithName("vd-quota-cvi"),
			vdbuilder.WithNamespace(f.Namespace().Name),
			vdbuilder.WithDataSourceObjectRef(v1alpha2.VirtualDiskObjectRefKindClusterVirtualImage, object.PrecreatedCVITestDataQCOW),
			vdbuilder.WithStorageClass(scPtr),
		)

		obs := vdobs.StartObserver(ctx, f, vd)

		By("Creating VirtualDisk", func() {
			err := f.CreateWithDeferredDeletion(ctx, vd)
			Expect(err).NotTo(HaveOccurred())
		})

		err := obs.WaitFor(vdobs.BeQuotaExceeded(), framework.LongTimeout)
		Expect(err).NotTo(HaveOccurred())
	})

	It("VirtualImage on DVCR reports a quota-exceeded ProvisioningFailed Ready condition", Label(precheck.PrecheckMainStandbyStorageClass), func() {
		vi := vibuilder.New(
			vibuilder.WithName("vi-dvcr-quota"),
			vibuilder.WithNamespace(f.Namespace().Name),
			vibuilder.WithStorage(v1alpha2.StorageContainerRegistry),
			vibuilder.WithDataSourceObjectRef(v1alpha2.VirtualImageObjectRefKindClusterVirtualImage, object.PrecreatedCVITestDataQCOW),
		)

		obs := viobs.StartObserver(ctx, f, vi)

		By("Creating VirtualImage on DVCR", func() {
			err := f.CreateWithDeferredDeletion(ctx, vi)
			Expect(err).NotTo(HaveOccurred())
		})

		err := obs.WaitFor(viobs.BeQuotaExceeded(), framework.LongTimeout)
		Expect(err).NotTo(HaveOccurred())
	})

	It("VirtualImage on PVC reports a quota-exceeded ProvisioningFailed Ready condition", Label(precheck.PrecheckMainStandbyStorageClass), func() {
		vi := vibuilder.New(
			vibuilder.WithName("vi-pvc-quota"),
			vibuilder.WithNamespace(f.Namespace().Name),
			vibuilder.WithStorage(v1alpha2.StoragePersistentVolumeClaim),
			vibuilder.WithDataSourceObjectRef(v1alpha2.VirtualImageObjectRefKindClusterVirtualImage, object.PrecreatedCVITestDataQCOW),
		)
		vi.Spec.PersistentVolumeClaim.StorageClass = scPtr

		obs := viobs.StartObserver(ctx, f, vi)

		By("Creating VirtualImage on PVC", func() {
			err := f.CreateWithDeferredDeletion(ctx, vi)
			Expect(err).NotTo(HaveOccurred())
		})

		err := obs.WaitFor(viobs.BeQuotaExceeded(), framework.LongTimeout)
		Expect(err).NotTo(HaveOccurred())
	})

	It("ClusterVirtualImage from a project-scoped VirtualDisk reports a quota-exceeded ProvisioningFailed Ready condition", Label(precheck.PrecheckMainStandbyStorageClass), func() {
		// CVI is cluster-scoped, so the name must be unique across
		// concurrent test runs; suffix it with the per-run namespace.
		cvi := cvibuilder.New(
			cvibuilder.WithName(fmt.Sprintf("cvi-quota-%s", f.Namespace().Name)),
			cvibuilder.WithDataSourceObjectRef(
				v1alpha2.ClusterVirtualImageObjectRefKindVirtualDisk,
				baseVD.Name,
				baseVD.Namespace,
			),
		)

		obs := cviobs.StartObserver(ctx, f, cvi)

		By("Creating ClusterVirtualImage", func() {
			err := f.CreateWithDeferredDeletion(ctx, cvi)
			Expect(err).NotTo(HaveOccurred())
		})

		err := obs.WaitFor(cviobs.BeQuotaExceeded(), framework.LongTimeout)
		Expect(err).NotTo(HaveOccurred())
	})
})

// applyBlockingResourceQuota installs a ResourceQuota in the framework
// namespace that hard-limits Pods and PersistentVolumeClaims to zero,
// thereby rejecting every importer/uploader Pod and every backing PVC
// the virtualization-controller tries to create for new resources.
//
// The function blocks until the kube-apiserver has populated the
// ResourceQuota .status.hard fields, ensuring that admission-time
// enforcement is in effect by the time the function returns.
func applyBlockingResourceQuota(ctx context.Context, f *framework.Framework) {
	GinkgoHelper()

	quota := &corev1.ResourceQuota{
		ObjectMeta: metav1.ObjectMeta{
			Name:      quotaExhaustedQuotaName,
			Namespace: f.Namespace().Name,
		},
		Spec: corev1.ResourceQuotaSpec{
			Hard: corev1.ResourceList{
				corev1.ResourceName("count/pods"):                   resource.MustParse("0"),
				corev1.ResourceName("count/persistentvolumeclaims"): resource.MustParse("0"),
			},
		},
	}

	By("Applying a project-blocking ResourceQuota", func() {
		err := f.CreateWithDeferredDeletion(ctx, quota)
		Expect(err).NotTo(HaveOccurred(), "failed to create quota %q", quota.Name)
	})

	By("Waiting for the ResourceQuota to be enforced by the project", func() {
		Eventually(func(g Gomega) {
			var rq corev1.ResourceQuota
			err := f.Clients.GenericClient().Get(ctx, types.NamespacedName{
				Namespace: quota.Namespace,
				Name:      quota.Name,
			}, &rq)
			g.Expect(err).NotTo(HaveOccurred())
			g.Expect(rq.Status.Hard).To(HaveKeyWithValue(
				corev1.ResourceName("count/pods"),
				resource.MustParse("0"),
			))
			g.Expect(rq.Status.Hard).To(HaveKeyWithValue(
				corev1.ResourceName("count/persistentvolumeclaims"),
				resource.MustParse("0"),
			))
		}).WithTimeout(framework.MiddleTimeout).WithPolling(framework.PollingInterval).Should(Succeed())
	})
}
