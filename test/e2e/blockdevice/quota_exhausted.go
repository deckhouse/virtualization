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

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"

	vdbuilder "github.com/deckhouse/virtualization-controller/pkg/builder/vd"
	vibuilder "github.com/deckhouse/virtualization-controller/pkg/builder/vi"
	vmbuilder "github.com/deckhouse/virtualization-controller/pkg/builder/vm"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/test/e2e/internal/framework"
	"github.com/deckhouse/virtualization/test/e2e/internal/label"
	"github.com/deckhouse/virtualization/test/e2e/internal/object"
	rqobs "github.com/deckhouse/virtualization/test/e2e/internal/observer/resourcequota"
	vdobs "github.com/deckhouse/virtualization/test/e2e/internal/observer/vd"
	viobs "github.com/deckhouse/virtualization/test/e2e/internal/observer/vi"
	"github.com/deckhouse/virtualization/test/e2e/internal/precheck"
)

// quotaExhaustedQuotaName is the name of the project ResourceQuota that
// blocks every PersistentVolumeClaim creation in the test namespace, forcing
// the virtualization-controller to surface the quota-exceeded condition on
// resources that create backing PVCs.
//
// Pods are deliberately NOT capped: on a WaitForFirstConsumer StorageClass the
// VirtualDisk creates its target PVC only after the consumer VirtualMachine is
// scheduled (see PVCImportStep), and the VM's virt-launcher Pod carries no
// resource-quota-overrides.deckhouse.io/ignore label — a Pod cap would reject
// it, the VM would never schedule, and the disk would park in
// WaitingForFirstConsumer forever instead of reporting QuotaExceeded.
const quotaExhaustedQuotaName = "v12n-e2e-block-pvcs"

var _ = label.SIGDescribe(label.SIGStorage, "QuotaExhausted", Ordered, Label(precheck.PrecheckDefaultStorageClass), func() {
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

		scPtr = defaultStorageClass()

		// Create a base VirtualDisk before applying the quota so that
		// the ClusterVirtualImage spec below has a valid object-ref
		// source to reference. Sourcing a CVI from a VD on PVC makes the
		// CVI importer Pod run in the user's namespace, which is exactly
		// what we need to exercise the user-namespace quota path for
		// CVIs. See ImporterService.GetPodSettingsWithPVC.
		baseVD = vdbuilder.New(
			vdbuilder.WithName("vd-quota-source"),
			vdbuilder.WithNamespace(f.Namespace().Name),
			// A bootable image: on a WaitForFirstConsumer StorageClass the
			// disk is provisioned by booting a VM from it, see below.
			vdbuilder.WithDataSourceObjectRef(v1alpha2.VirtualDiskObjectRefKindClusterVirtualImage, object.PrecreatedCVICustomBIOS),
			vdbuilder.WithStorageClass(scPtr),
		)
		if storageClassIsWaitForFirstConsumer(ctx, f, ptr.Deref(scPtr, "")) {
			// A WaitForFirstConsumer disk provisions only once a VirtualMachine
			// consumes it, so boot a throwaway VM and delete it once the disk
			// is Ready, leaving the namespace Pod-free for the blocking quota
			// below.
			obs := startVirtualDisk(ctx, f, baseVD)
			vm := runVirtualMachineFromDisks(ctx, f, observedDisk{vd: baseVD, obs: obs})
			Expect(f.Delete(ctx, vm)).To(Succeed())
		} else {
			createVirtualDiskAndWait(ctx, f, baseVD)
		}

		applyBlockingResourceQuota(ctx, f)
	})

	It("VirtualDisk reports QuotaExceeded reason on a fresh Ready condition", Label(precheck.PrecheckDefaultStorageClass), func() {
		vd := vdbuilder.New(
			vdbuilder.WithName("vd-quota-cvi"),
			vdbuilder.WithNamespace(f.Namespace().Name),
			vdbuilder.WithDataSourceObjectRef(v1alpha2.VirtualDiskObjectRefKindClusterVirtualImage, object.PrecreatedCVICustomBIOS),
			vdbuilder.WithStorageClass(scPtr),
		)

		obs := vdobs.StartObserver(ctx, f, vd)

		By("Creating VirtualDisk", func() {
			err := f.CreateWithDeferredDeletion(ctx, vd)
			Expect(err).NotTo(HaveOccurred())
		})

		// On a WaitForFirstConsumer StorageClass the disk parks in the
		// WaitForFirstConsumer phase and never attempts to create its target
		// PVC (see PVCImportStep), so the quota would never be exercised.
		// Give the disk a consumer: the quota caps only PVCs, so the VM's
		// virt-launcher pod schedules, the VM gets a node (PVCImportStep gates
		// target-PVC creation on it), and the disk proceeds to the PVC creation
		// the quota then rejects.
		// The VM never becomes Running — its disk never provisions — so don't
		// wait for it.
		if storageClassIsWaitForFirstConsumer(ctx, f, ptr.Deref(scPtr, "")) {
			By("Creating a consumer VirtualMachine to unpark the WaitForFirstConsumer disk", func() {
				vm := object.NewMinimalVM("vm-quota-consumer-", f.Namespace().Name, vmbuilder.WithDisks(vd))
				Expect(f.CreateWithDeferredDeletion(ctx, vm)).To(Succeed())
			})
		}

		err := obs.WaitFor(vdobs.BeQuotaExceeded(), framework.LongTimeout)
		Expect(err).NotTo(HaveOccurred())
	})

	It("VirtualImage on PVC reports a quota-exceeded ProvisioningFailed Ready condition", Label(precheck.PrecheckDefaultStorageClass), func() {
		vi := vibuilder.New(
			vibuilder.WithName("vi-pvc-quota"),
			vibuilder.WithNamespace(f.Namespace().Name),
			vibuilder.WithStorage(v1alpha2.StoragePersistentVolumeClaim),
			vibuilder.WithDataSourceObjectRef(v1alpha2.VirtualImageObjectRefKindClusterVirtualImage, object.PrecreatedCVICustomBIOS),
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
})

// applyBlockingResourceQuota installs a ResourceQuota in the framework
// namespace that hard-limits PersistentVolumeClaims to zero, thereby
// rejecting every backing PVC the virtualization-controller tries to
// create for new resources. Pods are left uncapped so the consumer
// VirtualMachine of the WaitForFirstConsumer test can schedule (see
// quotaExhaustedQuotaName).
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
				corev1.ResourceName("count/persistentvolumeclaims"): resource.MustParse("0"),
			},
		},
	}

	By("Applying a project-blocking ResourceQuota", func() {
		obs := rqobs.StartObserver(ctx, f, quota)

		err := f.CreateWithDeferredDeletion(ctx, quota)
		Expect(err).NotTo(HaveOccurred(), "failed to create quota %q", quota.Name)

		err = obs.WaitFor(rqobs.BeEnforced(), framework.MiddleTimeout)
		Expect(err).NotTo(HaveOccurred(), "ResourceQuota %q was not enforced by the project", quota.Name)
	})
}
