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

package vmpool

import (
	"context"
	"slices"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"
	crclient "sigs.k8s.io/controller-runtime/pkg/client"

	vdbuilder "github.com/deckhouse/virtualization-controller/pkg/builder/vd"
	vmbuilder "github.com/deckhouse/virtualization-controller/pkg/builder/vm"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/test/e2e/internal/framework"
	"github.com/deckhouse/virtualization/test/e2e/internal/object"
	"github.com/deckhouse/virtualization/test/e2e/internal/precheck"
)

var _ = Describe("VirtualMachinePool", Label(precheck.NoPrecheck), func() {
	var (
		f    *framework.Framework
		ctx  context.Context
		pool *v1alpha2.VirtualMachinePool
	)

	BeforeEach(func() {
		ctx = context.Background()
		f = framework.NewFramework("vmpool")

		mc, err := f.GetVirtualizationModuleConfig(ctx)
		Expect(err).NotTo(HaveOccurred())
		if !slices.Contains(mc.Spec.Settings.FeatureGates, "VirtualMachinePool") {
			Skip("the VirtualMachinePool feature gate is disabled")
		}

		f.Before()
		DeferCleanup(f.After)
	})

	// members returns the VirtualMachines owned by the pool (membership is by the
	// controller ownerReference, so this does not depend on controller-internal
	// label keys that live in another Go module).
	members := func() []v1alpha2.VirtualMachine {
		var list v1alpha2.VirtualMachineList
		Expect(f.GenericClient().List(ctx, &list, crclient.InNamespace(f.Namespace().Name))).To(Succeed())
		var mine []v1alpha2.VirtualMachine
		for i := range list.Items {
			if ref := metav1.GetControllerOf(&list.Items[i]); ref != nil && ref.UID == pool.UID {
				mine = append(mine, list.Items[i])
			}
		}
		return mine
	}
	runningCount := func() int {
		n := 0
		for _, m := range members() {
			if m.Status.Phase == v1alpha2.MachineRunning {
				n++
			}
		}
		return n
	}

	It("maintains the requested number of tiny replicas, each with its own root disk, and scales", func() {
		By("Creating a pool of 2 tiny VMs with a per-replica root disk from the alpine image", func() {
			tmpl := vmbuilder.New(
				vmbuilder.WithCPU(1, ptr.To("5%")),
				vmbuilder.WithMemory(*resource.NewQuantity(object.Mi512, resource.BinarySI)),
				vmbuilder.WithVirtualMachineClass(object.DefaultVMClass),
				vmbuilder.WithRunPolicy(v1alpha2.AlwaysOnPolicy),
				vmbuilder.WithLiveMigrationPolicy(v1alpha2.AlwaysSafeMigrationPolicy),
				vmbuilder.WithProvisioningUserData(object.AlpineCloudInit),
				// No blockDeviceRefs: the pool template has no such field. The
				// controller derives each replica's block devices from
				// virtualDiskTemplates order (first = boot).
			)
			rootDisk := vdbuilder.New(
				vdbuilder.WithSize(ptr.To(resource.MustParse("1Gi"))),
				vdbuilder.WithDataSourceObjectRef(v1alpha2.VirtualDiskObjectRefKindClusterVirtualImage, object.PrecreatedCVIAlpineBIOS),
			)

			pool = &v1alpha2.VirtualMachinePool{
				ObjectMeta: metav1.ObjectMeta{GenerateName: "pool-", Namespace: f.Namespace().Name},
				Spec: v1alpha2.VirtualMachinePoolSpec{
					Replicas:               ptr.To(int32(2)),
					ScaleDownPolicy:        v1alpha2.ScaleDownPolicyNewestFirst,
					VirtualMachineTemplate: v1alpha2.VirtualMachineTemplateSpec{Spec: tmpl.Spec},
					VirtualDiskTemplates: []v1alpha2.VirtualDiskTemplateSpec{{
						Name:    "root",
						Reclaim: v1alpha2.VirtualDiskReclaim{OnScaleDown: v1alpha2.VirtualDiskReclaimDelete},
						Spec:    rootDisk.Spec,
					}},
				},
			}
			Expect(f.CreateWithDeferredDeletion(ctx, pool)).To(Succeed())
		})

		By("Waiting until both replicas are Running", func() {
			Eventually(runningCount).WithTimeout(framework.LongTimeout).WithPolling(5 * time.Second).Should(Equal(2))
		})

		By("Checking every replica has its own Delete-policy root disk", func() {
			for _, m := range members() {
				vd := &v1alpha2.VirtualDisk{}
				Expect(f.GenericClient().Get(ctx, crclient.ObjectKey{Namespace: f.Namespace().Name, Name: m.Name + "-root"}, vd)).To(Succeed())
				ref := metav1.GetControllerOf(vd)
				Expect(ref).NotTo(BeNil())
				Expect(ref.Kind).To(Equal(v1alpha2.VirtualMachineKind)) // owned by the VM → removed with it
				Expect(ref.Name).To(Equal(m.Name))
			}
		})

		By("Scaling the pool up to 3 replicas", func() {
			Expect(f.GenericClient().Get(ctx, crclient.ObjectKeyFromObject(pool), pool)).To(Succeed())
			pool.Spec.Replicas = ptr.To(int32(3))
			Expect(f.GenericClient().Update(ctx, pool)).To(Succeed())
		})

		By("Waiting until all 3 replicas are Running", func() {
			Eventually(runningCount).WithTimeout(framework.LongTimeout).WithPolling(5 * time.Second).Should(Equal(3))
		})
	})
})
