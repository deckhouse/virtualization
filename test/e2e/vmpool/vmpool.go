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
	"encoding/json"
	"slices"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
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

	// buildPool returns a pool of tiny VMs with a single per-replica root disk
	// template. The template's blockDeviceRefs references that disk by name (a
	// placeholder the controller resolves per replica); the bijection with
	// virtualDiskTemplates is enforced by admission.
	buildPool := func(replicas int32, policy v1alpha2.ScaleDownPolicy, reclaim v1alpha2.VirtualDiskReclaim) *v1alpha2.VirtualMachinePool {
		tmpl := vmbuilder.New(
			vmbuilder.WithCPU(1, ptr.To("20%")),
			vmbuilder.WithMemory(*resource.NewQuantity(object.Mi512, resource.BinarySI)),
			vmbuilder.WithVirtualMachineClass(object.DefaultVMClass),
			vmbuilder.WithRunPolicy(v1alpha2.AlwaysOnPolicy),
			vmbuilder.WithLiveMigrationPolicy(v1alpha2.AlwaysSafeMigrationPolicy),
			vmbuilder.WithProvisioningUserData(object.AlpineCloudInit),
		)
		tmpl.Spec.BlockDeviceRefs = []v1alpha2.BlockDeviceSpecRef{
			{Kind: v1alpha2.DiskDevice, Name: "root"},
		}
		rootDisk := vdbuilder.New(
			vdbuilder.WithSize(ptr.To(resource.MustParse("100Mi"))),
			vdbuilder.WithDataSourceObjectRef(v1alpha2.VirtualDiskObjectRefKindClusterVirtualImage, object.PrecreatedCVIMyOS),
		)
		return &v1alpha2.VirtualMachinePool{
			ObjectMeta: metav1.ObjectMeta{GenerateName: "pool-", Namespace: f.Namespace().Name},
			Spec: v1alpha2.VirtualMachinePoolSpec{
				Replicas:               ptr.To(replicas),
				ScaleDownPolicy:        policy,
				VirtualMachineTemplate: v1alpha2.VirtualMachineTemplateSpec{Spec: tmpl.Spec},
				VirtualDiskTemplates: []v1alpha2.VirtualDiskTemplateSpec{{
					Name:    "root",
					Reclaim: reclaim,
					Spec:    rootDisk.Spec,
				}},
			},
		}
	}
	deleteReclaim := v1alpha2.VirtualDiskReclaim{OnScaleDown: v1alpha2.VirtualDiskReclaimDelete}

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

	// scaleDownWith POSTs to the aggregated-apiserver scaleDownWith subresource,
	// the same call `kubectl create --raw` makes. Returns the request error so
	// both the success and the rejection paths can be asserted.
	scaleDownWith := func(poolName string, targets ...string) error {
		body, err := json.Marshal(map[string][]string{"targets": targets})
		Expect(err).NotTo(HaveOccurred())
		return f.KubeClient().Discovery().RESTClient().Post().
			AbsPath("apis", "subresources.virtualization.deckhouse.io", "v1alpha2",
				"namespaces", f.Namespace().Name, "virtualmachinepools", poolName, "scaledownwith").
			Body(body).
			SetHeader("Content-Type", "application/json").
			Do(ctx).Error()
	}

	It("maintains the requested number of tiny replicas, each with its own root disk, and scales", func() {
		By("Creating a pool of 2 tiny VMs with a per-replica root disk from the myos image", func() {
			pool = buildPool(2, v1alpha2.ScaleDownPolicyNewestFirst, deleteReclaim)
			Expect(f.CreateWithDeferredDeletion(ctx, pool)).To(Succeed())
		})

		By("Waiting until both replicas are Running", func() {
			Eventually(runningCount).WithTimeout(framework.LongTimeout).WithPolling(3 * time.Second).Should(Equal(2))
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
			Eventually(runningCount).WithTimeout(framework.LongTimeout).WithPolling(3 * time.Second).Should(Equal(3))
		})
	})

	It("attaches a shared image (CD-ROM) referenced in blockDeviceRefs to every replica", func() {
		sharedImage := object.PrecreatedCVIUbuntuISO

		By("Creating a pool whose template references a per-replica root disk and a shared ClusterVirtualImage", func() {
			pool = buildPool(2, v1alpha2.ScaleDownPolicyNewestFirst, deleteReclaim)
			// The image is NOT a virtualDiskTemplates entry (it is shared, read-only,
			// not per-replica) and is not subject to the bijection.
			pool.Spec.VirtualMachineTemplate.Spec.BlockDeviceRefs = append(
				pool.Spec.VirtualMachineTemplate.Spec.BlockDeviceRefs,
				v1alpha2.BlockDeviceSpecRef{Kind: v1alpha2.ClusterImageDevice, Name: sharedImage},
			)
			Expect(f.CreateWithDeferredDeletion(ctx, pool)).To(Succeed())
		})

		By("Waiting until both replicas are Running", func() {
			Eventually(runningCount).WithTimeout(framework.LongTimeout).WithPolling(3 * time.Second).Should(Equal(2))
		})

		By("Checking every replica attaches the same shared image, unresolved", func() {
			ms := members()
			Expect(ms).To(HaveLen(2))
			for _, m := range ms {
				Expect(m.Spec.BlockDeviceRefs).To(ContainElement(v1alpha2.BlockDeviceSpecRef{
					Kind: v1alpha2.ClusterImageDevice, Name: sharedImage,
				}), "replica %s must reference the shared image verbatim", m.Name)
			}
		})
	})

	It("removes addressed replicas via scaleDownWith, shrinks the pool, and does not replace them", func() {
		By("Creating a pool of 2 and waiting until both are Running", func() {
			pool = buildPool(2, v1alpha2.ScaleDownPolicyNewestFirst, deleteReclaim)
			Expect(f.CreateWithDeferredDeletion(ctx, pool)).To(Succeed())
			Eventually(runningCount).WithTimeout(framework.LongTimeout).WithPolling(3 * time.Second).Should(Equal(2))
		})

		By("Rejecting a target that does not belong to the pool, without deleting anything", func() {
			Expect(apierrors.IsBadRequest(scaleDownWith(pool.Name, "not-a-member"))).To(BeTrue())
			Expect(members()).To(HaveLen(2))
		})

		var victim string
		By("Removing one addressed replica", func() {
			victim = members()[0].Name
			Expect(scaleDownWith(pool.Name, victim)).To(Succeed())
		})

		By("Verifying the pool shrank to 1, spec.replicas was decremented, and no replacement appears", func() {
			Eventually(func() int { return len(members()) }).WithTimeout(framework.LongTimeout).WithPolling(3 * time.Second).Should(Equal(1))
			Expect(f.GenericClient().Get(ctx, crclient.ObjectKeyFromObject(pool), pool)).To(Succeed())
			Expect(ptr.Deref(pool.Spec.Replicas, -1)).To(Equal(int32(1)))
			// Unlike a plain delete, scaleDownWith is not a lost replica: the count
			// stays at 1 and the removed VM does not come back.
			Consistently(func() int { return len(members()) }).WithTimeout(20 * time.Second).WithPolling(4 * time.Second).Should(Equal(1))
			for _, m := range members() {
				Expect(m.Name).NotTo(Equal(victim))
			}
		})
	})

	// The reclaim CEL rules live in the apiserver, so they can only be exercised
	// end-to-end. These pools use replicas: 0, so admission is checked without
	// booting any VM.
	Context("reclaim validation (CEL)", func() {
		It("rejects keep/ttl unless onScaleDown is Retain", func() {
			p := buildPool(0, v1alpha2.ScaleDownPolicyNewestFirst, v1alpha2.VirtualDiskReclaim{
				OnScaleDown: v1alpha2.VirtualDiskReclaimDelete,
				Keep:        1,
			})
			Expect(f.GenericClient().Create(ctx, p)).NotTo(Succeed())
		})

		It("rejects keep without ttl on Retain", func() {
			p := buildPool(0, v1alpha2.ScaleDownPolicyNewestFirst, v1alpha2.VirtualDiskReclaim{
				OnScaleDown: v1alpha2.VirtualDiskReclaimRetain,
				Keep:        1,
			})
			Expect(f.GenericClient().Create(ctx, p)).NotTo(Succeed())
		})

		It("accepts a valid Retain reclaim with keep and ttl", func() {
			pool = buildPool(0, v1alpha2.ScaleDownPolicyNewestFirst, v1alpha2.VirtualDiskReclaim{
				OnScaleDown: v1alpha2.VirtualDiskReclaimRetain,
				Keep:        1,
				TTL:         &metav1.Duration{Duration: time.Hour},
			})
			Expect(f.CreateWithDeferredDeletion(ctx, pool)).To(Succeed())
		})
	})
})
