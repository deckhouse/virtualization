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

package vm

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/ptr"

	vdbuilder "github.com/deckhouse/virtualization-controller/pkg/builder/vd"
	vmbuilder "github.com/deckhouse/virtualization-controller/pkg/builder/vm"
	vmipoption "github.com/deckhouse/virtualization-controller/pkg/builder/vmip"
	"github.com/deckhouse/virtualization-controller/pkg/controller/conditions"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/vmcondition"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/vmipcondition"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/vmiplcondition"
	"github.com/deckhouse/virtualization/test/e2e/eventually"
	"github.com/deckhouse/virtualization/test/e2e/internal/framework"
	"github.com/deckhouse/virtualization/test/e2e/internal/label"
	"github.com/deckhouse/virtualization/test/e2e/internal/object"
	"github.com/deckhouse/virtualization/test/e2e/internal/observer"
	vmobs "github.com/deckhouse/virtualization/test/e2e/internal/observer/vm"
	"github.com/deckhouse/virtualization/test/e2e/internal/precheck"
)

var _ = Describe("IPAM", Label(label.SIGCompute, precheck.NoPrecheck), func() {
	var (
		f   *framework.Framework
		ctx context.Context
	)

	BeforeEach(func() {
		f = framework.NewFramework("ipam")
		f.Before()
		ctx = context.Background()
	})

	AfterEach(func() {
		f.After(context.Background())
	})

	Context("vmip with type Auto", func() {
		It("Creates vmip with type Auto", func() {
			By("Create the auto vmip and check its binding with a lease")
			vmipAuto := object.NewVirtualMachineIPAddress("vmip-auto", f.Namespace().Name, vmipoption.WithTypeAuto())

			err := f.CreateWithDeferredDeletion(ctx, vmipAuto)
			Expect(err).NotTo(HaveOccurred())

			var lease *v1alpha2.VirtualMachineIPAddressLease
			vmipAuto, lease = WaitToBeBound(ctx, f, vmipAuto.Name)

			By("Remove labels from the lease")
			patch, err := json.Marshal([]map[string]interface{}{{
				"op":   "remove",
				"path": "/metadata/labels",
			}})
			Expect(err).NotTo(HaveOccurred())
			_, err = f.Clients.VirtClient().VirtualMachineIPAddressLeases().Patch(ctx, lease.Name, types.JSONPatchType, patch, metav1.PatchOptions{})
			Expect(err).NotTo(HaveOccurred())

			By("Wait for the label to be restored by the controller")
			_, _ = WaitToBeBound(ctx, f, vmipAuto.Name)
		})
	})

	Context("vmip with type Static", func() {
		It("Creates vmip with type Static", func() {
			By("Create an intermediate vmip to learn the pool subnet")
			intermediate := object.NewVirtualMachineIPAddress("vmip-intermediate", f.Namespace().Name, vmipoption.WithTypeAuto())
			err := f.CreateWithDeferredDeletion(ctx, intermediate)
			Expect(err).NotTo(HaveOccurred())

			var lease *v1alpha2.VirtualMachineIPAddressLease
			intermediate, lease = WaitToBeBound(ctx, f, intermediate.Name)

			By("Delete the intermediate vmip and check that the lease is released")
			err = f.Delete(ctx, intermediate)
			Expect(err).NotTo(HaveOccurred())
			WaitForLeaseToBeReleased(ctx, f, lease.Name)

			// The dance below deletes a lease and reclaims its IP, which requires
			// the IP to stay exclusively ours: leases are cluster-scoped and named
			// after the IP, so a parallel suite grabbing the freed address would
			// recreate the lease mid-wait. Auto allocation hands out the lowest
			// free addresses, so an address high in the intermediate's subnet is
			// safe from concurrent auto claims.
			staticAddress := highPoolAddress(ctx, f, intermediate.Status.Address)
			staticLeaseName := ipAddressToLeaseName(staticAddress)

			By("Allocate the address with a static vmip")
			vmipStatic := object.NewVirtualMachineIPAddress(
				"vmip-static",
				f.Namespace().Name,
				vmipoption.WithTypeStatic(staticAddress),
			)

			err = f.CreateWithDeferredDeletion(ctx, vmipStatic)
			Expect(err).NotTo(HaveOccurred())
			WaitToBeBound(ctx, f, vmipStatic.Name)

			By("Delete the static vmip and check that the lease is released")
			err = f.Delete(ctx, vmipStatic)
			Expect(err).NotTo(HaveOccurred())
			lease = WaitForLeaseToBeReleased(ctx, f, staticLeaseName)

			By("Reuse the released lease with another static vmip")
			vmipStatic = object.NewVirtualMachineIPAddress(
				"vmip-one-more-static",
				f.Namespace().Name,
				vmipoption.WithTypeStatic(staticAddress),
			)
			err = f.CreateWithDeferredDeletion(ctx, vmipStatic)
			Expect(err).NotTo(HaveOccurred())
			WaitToBeBound(ctx, f, vmipStatic.Name)

			By("Delete the static vmip and lease, then create another static vmip with this ip address")
			err = f.Delete(ctx, vmipStatic, lease)
			Expect(err).NotTo(HaveOccurred())

			vmipStatic = object.NewVirtualMachineIPAddress(
				"vmip-third-static",
				f.Namespace().Name,
				vmipoption.WithTypeStatic(staticAddress),
			)
			err = f.CreateWithDeferredDeletion(ctx, vmipStatic)
			Expect(err).NotTo(HaveOccurred())
			WaitToBeBound(ctx, f, vmipStatic.Name)
		})

		It("Create vm with static ip", func() {
			var (
				vmipStatic *v1alpha2.VirtualMachineIPAddress
				vm         *v1alpha2.VirtualMachine
			)

			By("Create an intermediate vmip to allocate a new ip address", func() {
				vmipStatic = object.NewVirtualMachineIPAddress("vmip-static", f.Namespace().Name, vmipoption.WithTypeAuto())
				err := f.CreateWithDeferredDeletion(ctx, vmipStatic)
				Expect(err).NotTo(HaveOccurred())
			})

			var vmObs vmobs.Observer
			By("Create vm with static ip", func() {
				vd := object.NewVDFromCVI("vd-with-static-ip", f.Namespace().Name, object.PrecreatedCVICustomBIOS, vdbuilder.WithSize(ptr.To(resource.MustParse(vdCustomImageSize))))
				err := f.CreateWithDeferredDeletion(ctx, vd)
				Expect(err).NotTo(HaveOccurred())

				vm = object.NewMinimalVM("vm-with-static-ip", f.Namespace().Name,
					vmbuilder.WithIpAddress(vmipStatic.Name),
					// The custom image has no cloud-init; the guest agent is
					// baked into the image, so no provisioning is needed.
					vmbuilder.WithBlockDeviceRefs(v1alpha2.BlockDeviceSpecRef{
						Kind: v1alpha2.VirtualDiskKind,
						Name: vd.Name,
					}))
				err = f.CreateWithDeferredDeletion(ctx, vm)
				Expect(err).NotTo(HaveOccurred())

				// The VM uses generateName, so its name is known only after creation;
				// an observer armed earlier would watch an empty name and never match.
				vmObs = vmobs.StartObserver(ctx, f, vm)
				vmObs.Never(vmobs.BeFailed())
			})

			By("Wait virtual machine to be running", func() {
				err := vmObs.WaitFor(vmobs.BeAgentReady(), framework.LongTimeout)
				Expect(err).NotTo(HaveOccurred())
			})

			By("Verify vmip attached to vm", func() {
				var err error
				vmipStatic, err = f.VirtClient().VirtualMachineIPAddresses(f.Namespace().Name).Get(ctx, vmipStatic.Name, metav1.GetOptions{})
				Expect(err).NotTo(HaveOccurred())
				cond, _ := conditions.GetCondition(vmipcondition.AttachedType, vmipStatic.Status.Conditions)
				Expect(cond.Status).To(Equal(metav1.ConditionTrue))
			})

			By("Verify ip address on vm", func() {
				vm = getVirtualMachine(ctx, f, vm.Name)
				cond, _ := conditions.GetCondition(vmcondition.TypeIPAddressReady, vm.Status.Conditions)
				Expect(cond.Status).To(Equal(metav1.ConditionTrue))
				Expect(vm.Status.IPAddress).To(Equal(vmipStatic.Status.Address))

				expectAddr := fmt.Sprintf("%s/32", vmipStatic.Status.Address)
				// EXCEPTION: this is a guest-side wait (ip over SSH), not a Kubernetes
				// resource, so there is nothing to observe via an Observer. The custom
				// custom image has no cloud user, so log in as root.
				eventually.UntilAssertion(func(g Gomega) {
					result, err := f.SSHCommand(vm.Name, vm.Namespace, "ip -4 addr show eth0", framework.WithSSHUser("root"))
					g.Expect(err).NotTo(HaveOccurred())
					g.Expect(result).Should(ContainSubstring(expectAddr))
				}, framework.ShortTimeout)
			})
		})
	})
})

func ExpectToBeReleased(g Gomega, lease *v1alpha2.VirtualMachineIPAddressLease) {
	boundCondition, _ := conditions.GetCondition(vmiplcondition.BoundType, lease.Status.Conditions)
	g.Expect(boundCondition.Status).To(Equal(metav1.ConditionFalse))
	g.Expect(boundCondition.Reason).To(Equal(vmiplcondition.Released.String()))
	g.Expect(boundCondition.ObservedGeneration).To(Equal(lease.Generation))
	g.Expect(lease.Status.Phase).To(Equal(v1alpha2.VirtualMachineIPAddressLeasePhaseReleased))
}

func ExpectToBeBound(g Gomega, vmip *v1alpha2.VirtualMachineIPAddress, lease *v1alpha2.VirtualMachineIPAddressLease) {
	// 1. Check vmip to be Bound.
	boundCondition, _ := conditions.GetCondition(vmipcondition.BoundType, vmip.Status.Conditions)
	g.Expect(boundCondition.Status).To(Equal(metav1.ConditionTrue), "vmip status is not bound")
	g.Expect(boundCondition.Reason).To(Equal(vmipcondition.Bound.String()), "vmip  reason is not bound")
	g.Expect(boundCondition.ObservedGeneration).To(Equal(vmip.Generation), "vmip observed generation is not equal")

	g.Expect(vmip.Status.Phase).To(Equal(v1alpha2.VirtualMachineIPAddressPhaseBound), "phase is not bound")
	g.Expect(vmip.Status.Address).NotTo(BeEmpty(), "vmip.Status.Address is empty")
	g.Expect(ipAddressToLeaseName(vmip.Status.Address)).To(Equal(lease.Name), "lease name is not equal")

	// 2. Check lease to be Bound.
	boundCondition, _ = conditions.GetCondition(vmiplcondition.BoundType, lease.Status.Conditions)
	g.Expect(boundCondition.Status).To(Equal(metav1.ConditionTrue), "lease status is not bound")
	g.Expect(boundCondition.Reason).To(Equal(vmiplcondition.Bound.String()), "lease reason is not bound")
	g.Expect(boundCondition.ObservedGeneration).To(Equal(lease.Generation), "lease observed generation is not equal")

	g.Expect(lease.Status.Phase).To(Equal(v1alpha2.VirtualMachineIPAddressLeasePhaseBound))
	g.Expect(lease.Labels["virtualization.deckhouse.io/virtual-machine-ip-address-uid"]).To(Equal(string(vmip.UID)), "lease label is not equal")
	g.Expect(lease.Spec.VirtualMachineIPAddressRef).NotTo(BeNil(), "lease spec.VirtualMachineIPAddressRef is nil")
	g.Expect(lease.Spec.VirtualMachineIPAddressRef.Name).To(Equal(vmip.Name), "lease spec.VirtualMachineIPAddressRef.Name is not equal")
	g.Expect(lease.Spec.VirtualMachineIPAddressRef.Namespace).To(Equal(vmip.Namespace), "lease spec.VirtualMachineIPAddressRef.Namespace is not equal")
}

// WaitToBeBound observes the vmip and the lease it derives through watches
// until both report the Bound state, and returns the settled objects.
func WaitToBeBound(ctx context.Context, f *framework.Framework, vmipName string) (*v1alpha2.VirtualMachineIPAddress, *v1alpha2.VirtualMachineIPAddressLease) {
	GinkgoHelper()

	namespace := f.Namespace().Name
	vmipObs, err := observer.New[*v1alpha2.VirtualMachineIPAddress](
		ctx,
		f.VirtClient().VirtualMachineIPAddresses(namespace),
		vmipName, namespace,
	)
	Expect(err).NotTo(HaveOccurred(), "failed to start observer for vmip %s/%s", namespace, vmipName)
	defer vmipObs.Stop()

	err = vmipObs.WaitFor(vmipBound(), framework.ShortTimeout)
	Expect(err).NotTo(HaveOccurred(), "vmip %s/%s should become bound", namespace, vmipName)

	vmip, err := f.VirtClient().VirtualMachineIPAddresses(namespace).Get(ctx, vmipName, metav1.GetOptions{})
	Expect(err).NotTo(HaveOccurred())

	leaseName := ipAddressToLeaseName(vmip.Status.Address)
	leaseObs, err := observer.New[*v1alpha2.VirtualMachineIPAddressLease](
		ctx,
		f.VirtClient().VirtualMachineIPAddressLeases(),
		leaseName, "",
	)
	Expect(err).NotTo(HaveOccurred(), "failed to start observer for lease %s", leaseName)
	defer leaseObs.Stop()

	err = leaseObs.WaitFor(leaseBound(), framework.ShortTimeout)
	Expect(err).NotTo(HaveOccurred(), "lease %s should become bound", leaseName)

	lease, err := f.VirtClient().VirtualMachineIPAddressLeases().Get(ctx, leaseName, metav1.GetOptions{})
	Expect(err).NotTo(HaveOccurred())

	// Re-assert the settled pair with the detailed cross-object checks (labels,
	// back-references) and their rich failure messages.
	ExpectToBeBound(Default, vmip, lease)

	return vmip, lease
}

// vmipBound reports the vmip has settled in the Bound state with a fresh
// Bound condition and an allocated address.
func vmipBound() observer.Predicate[*v1alpha2.VirtualMachineIPAddress] {
	return func(vmip *v1alpha2.VirtualMachineIPAddress) (bool, error) {
		boundCondition, _ := conditions.GetCondition(vmipcondition.BoundType, vmip.Status.Conditions)
		return boundCondition.Status == metav1.ConditionTrue &&
			boundCondition.Reason == vmipcondition.Bound.String() &&
			boundCondition.ObservedGeneration == vmip.Generation &&
			vmip.Status.Phase == v1alpha2.VirtualMachineIPAddressPhaseBound &&
			vmip.Status.Address != "", nil
	}
}

// leaseBound reports the lease has settled in the Bound state with a fresh
// Bound condition.
func leaseBound() observer.Predicate[*v1alpha2.VirtualMachineIPAddressLease] {
	return func(lease *v1alpha2.VirtualMachineIPAddressLease) (bool, error) {
		boundCondition, _ := conditions.GetCondition(vmiplcondition.BoundType, lease.Status.Conditions)
		return boundCondition.Status == metav1.ConditionTrue &&
			boundCondition.Reason == vmiplcondition.Bound.String() &&
			boundCondition.ObservedGeneration == lease.Generation &&
			lease.Status.Phase == v1alpha2.VirtualMachineIPAddressLeasePhaseBound, nil
	}
}

// leaseReleased reports the lease has settled in the Released state with a
// fresh Bound=False/Released condition.
func leaseReleased() observer.Predicate[*v1alpha2.VirtualMachineIPAddressLease] {
	return func(lease *v1alpha2.VirtualMachineIPAddressLease) (bool, error) {
		boundCondition, _ := conditions.GetCondition(vmiplcondition.BoundType, lease.Status.Conditions)
		return boundCondition.Status == metav1.ConditionFalse &&
			boundCondition.Reason == vmiplcondition.Released.String() &&
			boundCondition.ObservedGeneration == lease.Generation &&
			lease.Status.Phase == v1alpha2.VirtualMachineIPAddressLeasePhaseReleased, nil
	}
}

// WaitForLeaseToBeReleased observes the lease through a watch until it reports
// the Released state, and returns the settled object.
func WaitForLeaseToBeReleased(ctx context.Context, f *framework.Framework, leaseName string) *v1alpha2.VirtualMachineIPAddressLease {
	GinkgoHelper()

	leaseObs, err := observer.New[*v1alpha2.VirtualMachineIPAddressLease](
		ctx,
		f.VirtClient().VirtualMachineIPAddressLeases(),
		leaseName, "",
	)
	Expect(err).NotTo(HaveOccurred(), "failed to start observer for lease %s", leaseName)
	defer leaseObs.Stop()

	err = leaseObs.WaitFor(leaseReleased(), framework.ShortTimeout)
	Expect(err).NotTo(HaveOccurred(), "lease %s should be released", leaseName)

	lease, err := f.VirtClient().VirtualMachineIPAddressLeases().Get(ctx, leaseName, metav1.GetOptions{})
	Expect(err).NotTo(HaveOccurred())

	// Re-assert the settled object with the rich failure messages.
	ExpectToBeReleased(Default, lease)

	return lease
}

func ipAddressToLeaseName(ipAddress string) string {
	return "ip-" + strings.ReplaceAll(ipAddress, ".", "-")
}

// highPoolAddress returns an address near the top of the pool the given address
// belongs to, free of any lease, for the spec to claim statically. The e2e pool
// is a /24, so the address stays inside it.
func highPoolAddress(ctx context.Context, f *framework.Framework, poolAddress string) string {
	GinkgoHelper()

	parts := strings.Split(poolAddress, ".")
	Expect(parts).To(HaveLen(4), "expected an IPv4 pool address, got %q", poolAddress)
	prefix := strings.Join(parts[:3], ".")

	// A fixed address would not do. Leases are cluster-scoped, named after the
	// address, and outlive the namespace that held them: a released lease sits
	// around for minutes. Back-to-back runs of this spec would then pick the very
	// same address and the second one would wait out its timeout on a lease that
	// still belongs to the first one's namespace. So take the highest address the
	// cluster currently has no lease for at all.
	leases, err := f.VirtClient().VirtualMachineIPAddressLeases().List(ctx, metav1.ListOptions{})
	Expect(err).NotTo(HaveOccurred(), "failed to list VirtualMachineIPAddressLeases")

	taken := make(map[string]struct{}, len(leases.Items))
	for _, lease := range leases.Items {
		taken[lease.Name] = struct{}{}
	}

	for octet := highPoolAddressLast; octet >= highPoolAddressFirst; octet-- {
		address := fmt.Sprintf("%s.%d", prefix, octet)
		if _, held := taken[ipAddressToLeaseName(address)]; !held {
			return address
		}
	}

	Fail(fmt.Sprintf("no free address left in %s.%d-%d to claim statically", prefix, highPoolAddressFirst, highPoolAddressLast))
	return ""
}

// The band of the pool this spec claims from. It sits above the addresses auto
// allocation hands out (it starts from the lowest free one), so a concurrent
// suite cannot take one of these from under the spec.
const (
	highPoolAddressFirst = 200
	highPoolAddressLast  = 250
)
