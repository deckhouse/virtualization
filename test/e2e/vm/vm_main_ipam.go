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

package vm

import (
	"context"
	"encoding/json"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	k8stypes "k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/ptr"
	crclient "sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/deckhouse/virtualization-controller/pkg/builder/vd"
	"github.com/deckhouse/virtualization-controller/pkg/builder/vm"
	vmipbuilder "github.com/deckhouse/virtualization-controller/pkg/builder/vmip"
	"github.com/deckhouse/virtualization-controller/pkg/common/annotations"
	"github.com/deckhouse/virtualization-controller/pkg/common/network"
	"github.com/deckhouse/virtualization-controller/pkg/common/network/sdnapi"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/test/e2e/eventually"
	"github.com/deckhouse/virtualization/test/e2e/internal/framework"
	"github.com/deckhouse/virtualization/test/e2e/internal/label"
	"github.com/deckhouse/virtualization/test/e2e/internal/object"
	vmobs "github.com/deckhouse/virtualization/test/e2e/internal/observer/vm"
	"github.com/deckhouse/virtualization/test/e2e/internal/precheck"
	"github.com/deckhouse/virtualization/test/e2e/internal/util"
)

const (
	// annCiliumIPAddress is the legacy (vmip-backed) way to request the Main
	// address from cni-cilium on the virt-launcher pod.
	annCiliumIPAddress = "cni.cilium.io/ipAddress"

	mainIPAMStaticIPName = "main-static-ip"
	mainIPAMVMIPName     = "vmip-e2e-main"
)

// The sdn module serves the Main network once the main ClusterNetwork has a pool,
// which the precheck of this suite verifies. The pod of every machine then carries
// both the Main interface spec and cni.cilium.io/ipAddress, whether a vmip or an
// IPAddress takes the address. The specs run serially because they share the
// address pool.
var _ = Describe("VirtualMachineMainIPAM", Serial, Label(label.SIGCompute, precheck.PrecheckMainIPAM), func() {
	var (
		ctx context.Context
		f   *framework.Framework
	)

	BeforeEach(func() {
		ctx = context.Background()
		f = framework.NewFramework("vm-main-ipam")
		DeferCleanup(f.After)
		f.Before()
	})

	It("allocates the Main address of a new machine and keeps it across restart", func() {
		var (
			vdRoot    *v1alpha2.VirtualDisk
			testVM    *v1alpha2.VirtualMachine
			testVMObs vmobs.Observer
		)

		By("Create VM without a networks section", func() {
			ns := f.Namespace().Name
			vdRoot = object.NewVDFromCVI("vd-root", ns, object.PrecreatedCVICustomBIOS,
				vd.WithSize(ptr.To(resource.MustParse(vdCustomImageSize))),
			)
			testVM = buildMainIPAMVM("vm-main-auto", ns, vdRoot.Name)

			Expect(f.CreateWithDeferredDeletion(ctx, vdRoot, testVM)).To(Succeed())
			testVMObs = vmobs.StartObserver(ctx, f, testVM)
		})

		By("Wait for VM Running", func() {
			Expect(testVMObs.WaitFor(vmobs.BeRunning(), framework.LongTimeout)).To(Succeed())
			testVM = refreshVM(ctx, f, testVM)
		})

		var ipaName, ipAddress string
		By("Verify the auto IPAddress owned by the VM is Allocated", func() {
			eventually.UntilAssertion(func(g Gomega) {
				ipa := findMainIPAddressByVMUID(ctx, g, testVM)
				g.Expect(ipa).NotTo(BeNil(), "IPAddress with label %s=%s and networkRef ClusterNetwork/main should exist",
					annotations.LabelVirtualMachineUID, testVM.UID)

				g.Expect(isOwnedByVM(ipa, testVM)).To(BeTrue(), "the auto IPAddress should have an ownerRef on the VM")
				g.Expect(network.IPAddressAllocated(ipa)).To(BeTrue(), "IPAddress %s should have condition Allocated=True", ipa.Name)
				g.Expect(ipa.Status.Address).NotTo(BeEmpty())

				ipaName = ipa.Name
				ipAddress = ipa.Status.Address
			}, framework.MiddleTimeout, eventually.WithPolling(time.Second))
		})

		By("Verify the template has the Main interface spec and no vmip is created", func() {
			// A machine created where the sdn module serves the Main network takes its address
			// from the IPAddress alone: the deprecated resource is not created for it
			// any more.
			vmips := &v1alpha2.VirtualMachineIPAddressList{}
			Expect(f.GenericClient().List(ctx, vmips, crclient.InNamespace(testVM.Namespace))).To(Succeed())
			Expect(vmips.Items).To(BeEmpty(), "no VirtualMachineIPAddress must be created for a new machine")

			kvvm, err := util.GetInternalVirtualMachine(ctx, testVM)
			Expect(err).NotTo(HaveOccurred())
			Expect(kvvm).NotTo(BeNil())
			Expect(network.KVVMHasMainInterfaceSpec(kvvm)).To(BeTrue(), "the template must advertise the Main interface spec")
		})

		By("Verify status.ipAddress and status.networks[Main]", func() {
			eventually.UntilAssertion(func(g Gomega) {
				updated := refreshVM(ctx, f, testVM)
				g.Expect(updated.Status.IPAddress).To(Equal(ipAddress))

				mainStatus := getMainNetworkStatus(updated)
				g.Expect(mainStatus).NotTo(BeNil())
				g.Expect(mainStatus.IPAddress).To(Equal(ipAddress))
				g.Expect(mainStatus.IPAddressName).To(Equal(ipaName))
			}, framework.MiddleTimeout, eventually.WithPolling(time.Second))
		})

		By("Verify the pod carries both address annotations", func() {
			expectPodHasBothAddressAnnotations(ctx, f, testVM, ipaName, ipAddress)
		})

		By("Verify the address is present on eth0 in the guest OS", func() {
			eventually.SSHReadyAsRoot(f, testVM, framework.LongTimeout)
			expectGuestHasMainAddress(f, testVM, ipAddress)
		})

		By("Restart VM and verify the same address is kept", func() {
			restartMainIPAMVM(ctx, f, testVM, testVMObs)

			eventually.UntilAssertion(func(g Gomega) {
				updated := refreshVM(ctx, f, testVM)
				g.Expect(updated.Status.IPAddress).To(Equal(ipAddress), "the auto Main address should be stable across restart")

				mainStatus := getMainNetworkStatus(updated)
				g.Expect(mainStatus).NotTo(BeNil())
				g.Expect(mainStatus.IPAddressName).To(Equal(ipaName))
			}, framework.MiddleTimeout, eventually.WithPolling(time.Second))
		})
	})

	It("uses the referenced static IPAddress for the Main network", func() {
		var (
			vdRoot    *v1alpha2.VirtualDisk
			testVM    *v1alpha2.VirtualMachine
			testVMObs vmobs.Observer
			staticIP  string
		)

		By("Pick an address the sdn module will accept", func() {
			staticIP = pickFreeMainAddress(ctx, f.Namespace().Name)
		})

		By("Create the static IPAddress and the VM referencing it", func() {
			ns := f.Namespace().Name
			Expect(util.CreateSDNIPAddress(ctx, f, mainIPAMStaticIPName, ns,
				v1alpha2.NetworksTypeClusterNetwork, network.MainClusterNetworkName, staticIP)).To(Succeed())

			eventually.UntilAssertion(func(g Gomega) {
				address, err := util.GetSDNAllocatedAddress(ctx, f, mainIPAMStaticIPName, ns)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(address).To(Equal(staticIP))
			}, framework.MiddleTimeout, eventually.WithPolling(time.Second))

			vdRoot = object.NewVDFromCVI("vd-root", ns, object.PrecreatedCVICustomBIOS,
				vd.WithSize(ptr.To(resource.MustParse(vdCustomImageSize))),
			)
			testVM = buildMainIPAMVM("vm-main-static", ns, vdRoot.Name,
				vm.WithNetwork(v1alpha2.NetworksSpec{
					Type:          v1alpha2.NetworksTypeMain,
					IPAddressName: mainIPAMStaticIPName,
				}),
			)

			Expect(f.CreateWithDeferredDeletion(ctx, vdRoot, testVM)).To(Succeed())
			testVMObs = vmobs.StartObserver(ctx, f, testVM)
		})

		By("Wait for VM Running", func() {
			Expect(testVMObs.WaitFor(vmobs.BeRunning(), framework.LongTimeout)).To(Succeed())
		})

		By("Verify status.ipAddress and status.networks[Main] have the static address", func() {
			eventually.UntilAssertion(func(g Gomega) {
				updated := refreshVM(ctx, f, testVM)
				g.Expect(updated.Status.IPAddress).To(Equal(staticIP))

				mainStatus := getMainNetworkStatus(updated)
				g.Expect(mainStatus).NotTo(BeNil())
				g.Expect(mainStatus.IPAddress).To(Equal(staticIP))
				g.Expect(mainStatus.IPAddressName).To(Equal(mainIPAMStaticIPName))
			}, framework.MiddleTimeout, eventually.WithPolling(time.Second))
		})

		By("Verify the pod carries both address annotations", func() {
			expectPodHasBothAddressAnnotations(ctx, f, testVM, mainIPAMStaticIPName, staticIP)
		})

		By("Verify the static address is present on eth0 in the guest OS", func() {
			eventually.SSHReadyAsRoot(f, testVM, framework.LongTimeout)
			expectGuestHasMainAddress(f, testVM, staticIP)
		})
	})

	It("keeps a machine on its vmip address and moves it to the mirror IPAddress", label.Slow(), func() {
		// A machine created in this cluster gets an IPAddress, so moving an existing fleet
		// across a module upgrade cannot be reproduced here: it is checked by hand. What
		// is reproducible is where that move lands: the vmip still takes the address, a
		// mirror IPAddress accounts it to the sdn module, and the running pod gets the
		// Main interface spec without a restart. From there the owner moves the machine
		// to the mirror.
		var (
			vdRoot      *v1alpha2.VirtualDisk
			testVM      *v1alpha2.VirtualMachine
			testVMObs   vmobs.Observer
			vmipAddress string
			podUID      k8stypes.UID
		)

		By("Create VirtualMachineIPAddress (Auto) and a VM referencing it", func() {
			ns := f.Namespace().Name
			vmip := vmipbuilder.New(
				vmipbuilder.WithName(mainIPAMVMIPName),
				vmipbuilder.WithNamespace(ns),
				vmipbuilder.WithTypeAuto(),
			)
			vdRoot = object.NewVDFromCVI("vd-root", ns, object.PrecreatedCVICustomBIOS,
				vd.WithSize(ptr.To(resource.MustParse(vdCustomImageSize))),
			)
			testVM = buildMainIPAMVM("vm-main-vmip", ns, vdRoot.Name,
				vm.WithIpAddress(mainIPAMVMIPName),
			)

			Expect(f.CreateWithDeferredDeletion(ctx, vmip, vdRoot, testVM)).To(Succeed())
			testVMObs = vmobs.StartObserver(ctx, f, testVM)
			Expect(testVMObs.WaitFor(vmobs.BeRunning(), framework.LongTimeout)).To(Succeed())
		})

		By("Verify the address comes from the vmip and the pod requests it from cni-cilium", func() {
			testVM = refreshVM(ctx, f, testVM)
			vmipAddress = testVM.Status.IPAddress
			Expect(vmipAddress).NotTo(BeEmpty())
			Expect(testVM.Status.VirtualMachineIPAddress).To(Equal(mainIPAMVMIPName))

			pod := getActiveVirtLauncherPod(ctx, f, testVM.Name, testVM.Namespace)
			Expect(pod).NotTo(BeNil(), "active virt-launcher pod should exist")
			Expect(pod.Annotations).To(HaveKeyWithValue(annCiliumIPAddress, vmipAddress), "the pod of a VM on a vmip must carry the cilium address request")
			podUID = pod.UID
		})

		By("Verify the mirror is owned by the VM, carries its UID label and has the address", func() {
			// The label is what the Main lookup goes by, and a user vmip never has one:
			// the mirror gets it from the controller, not from the resource it mirrors.
			eventually.UntilAssertion(func(g Gomega) {
				mirror := findMainIPAddressByVMUID(ctx, g, testVM)
				g.Expect(mirror).NotTo(BeNil(), "the mirror of the vmip should be labeled with the VM UID")
				g.Expect(mirror.Name).To(Equal(mainIPAMVMIPName))
				g.Expect(isOwnedByVM(mirror, testVM)).To(BeTrue())
				g.Expect(network.IPAddressAllocated(mirror)).To(BeTrue())
				g.Expect(mirror.Status.Address).To(Equal(vmipAddress))
			}, framework.MiddleTimeout, eventually.WithPolling(time.Second))
		})

		By("Wait for the Main interface spec on the internal VM", func() {
			eventually.UntilAssertion(func(g Gomega) {
				kvvm, err := util.GetInternalVirtualMachine(ctx, testVM)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(kvvm).NotTo(BeNil())
				g.Expect(network.KVVMHasMainInterfaceSpec(kvvm)).To(BeTrue())
				// The Main interface spec comes without taking cni.cilium.io/ipAddress away: a
				// node with an older cni-cilium keeps serving the address the machine already
				// runs on.
				g.Expect(kvvm.Spec.Template.ObjectMeta.Annotations).To(HaveKeyWithValue(annCiliumIPAddress, vmipAddress))
			}, framework.LongTimeout, eventually.WithPolling(2*time.Second))
		})

		By("Verify the address is unchanged and the running pod was not touched", func() {
			updated := refreshVM(ctx, f, testVM)
			Expect(updated.Status.IPAddress).To(Equal(vmipAddress), "writing the Main interface spec must not change the address")

			pod := getActiveVirtLauncherPod(ctx, f, testVM.Name, testVM.Namespace)
			Expect(pod).NotTo(BeNil())
			Expect(pod.UID).To(Equal(podUID), "writing the Main interface spec must not recreate the running pod")
			Expect(pod.Annotations).To(HaveKey(annCiliumIPAddress), "the live pod keeps cni.cilium.io/ipAddress")
		})

		By("Restart the VM and verify the new pod gets the Main interface spec with the same address", func() {
			restartMainIPAMVM(ctx, f, testVM, testVMObs)

			updated := refreshVM(ctx, f, testVM)
			Expect(updated.Status.IPAddress).To(Equal(vmipAddress))
			expectPodHasBothAddressAnnotations(ctx, f, updated, mainIPAMVMIPName, vmipAddress)
		})

		By("Move the VM to the mirror: clear the vmip field, reference the mirror", func() {
			updated := refreshVM(ctx, f, testVM)
			updated.Spec.VirtualMachineIPAddress = ""
			updated.Spec.Networks = []v1alpha2.NetworksSpec{{
				Type:          v1alpha2.NetworksTypeMain,
				IPAddressName: mainIPAMVMIPName,
			}}
			Expect(f.GenericClient().Update(ctx, updated)).To(Succeed())

			Expect(util.IsRestartRequired(updated, framework.MiddleTimeout)).To(BeTrue(), "the move should await a restart")
		})

		By("Restart the VM and verify it runs on the mirror with the same address", func() {
			restartMainIPAMVM(ctx, f, testVM, testVMObs)

			eventually.UntilAssertion(func(g Gomega) {
				updated := refreshVM(ctx, f, testVM)
				g.Expect(updated.Status.IPAddress).To(Equal(vmipAddress), "the address must survive the move")

				mainStatus := getMainNetworkStatus(updated)
				g.Expect(mainStatus).NotTo(BeNil())
				g.Expect(mainStatus.IPAddress).To(Equal(vmipAddress))
				g.Expect(mainStatus.IPAddressName).To(Equal(mainIPAMVMIPName))
			}, framework.MiddleTimeout, eventually.WithPolling(time.Second))

			expectPodHasBothAddressAnnotations(ctx, f, testVM, mainIPAMVMIPName, vmipAddress)
		})

		By("Delete the vmip and verify the mirror IPAddress survives", func() {
			vmip := &v1alpha2.VirtualMachineIPAddress{ObjectMeta: metav1.ObjectMeta{
				Name:      mainIPAMVMIPName,
				Namespace: testVM.Namespace,
			}}
			Expect(f.GenericClient().Delete(ctx, vmip)).To(Succeed())

			eventually.UntilAssertion(func(g Gomega) {
				err := f.GenericClient().Get(ctx, crclient.ObjectKeyFromObject(vmip), &v1alpha2.VirtualMachineIPAddress{})
				g.Expect(k8serrors.IsNotFound(err)).To(BeTrue(), "vmip should be deleted after the move, got: %v", err)
			}, framework.MiddleTimeout, eventually.WithPolling(time.Second))

			mirror := &sdnapi.IPAddress{}
			Expect(f.GenericClient().Get(ctx, crclient.ObjectKeyFromObject(vmip), mirror)).To(Succeed(), "the mirror IPAddress must survive the vmip deletion")
			Expect(mirror.GetDeletionTimestamp()).To(BeNil())
		})
	})

	It("keeps the Main address across a live migration", label.Slow(), func() {
		// A live-migration target pod is created from the annotations of the running
		// instance, not from the template of the internal VM. If the instance were left
		// behind, the machine would land on the target without the Main interface spec —
		// and on a cluster without vm-route-forge, without its route.
		var (
			vdRoot     *v1alpha2.VirtualDisk
			testVM     *v1alpha2.VirtualMachine
			testVMObs  vmobs.Observer
			address    string
			ipaName    string
			sourceNode string
		)

		By("Create a VM with an auto Main address", func() {
			ns := f.Namespace().Name
			vdRoot = object.NewVDFromCVI("vd-root", ns, object.PrecreatedCVICustomBIOS,
				vd.WithSize(ptr.To(resource.MustParse(vdCustomImageSize))),
			)
			testVM = buildMainIPAMVM("vm-main-migrated", ns, vdRoot.Name)

			Expect(f.CreateWithDeferredDeletion(ctx, vdRoot, testVM)).To(Succeed())
			testVMObs = vmobs.StartObserver(ctx, f, testVM)
			Expect(testVMObs.WaitFor(vmobs.BeRunning(), framework.LongTimeout)).To(Succeed())
		})

		By("Remember the address and the node the machine runs on", func() {
			testVM = refreshVM(ctx, f, testVM)
			address = testVM.Status.IPAddress
			Expect(address).NotTo(BeEmpty())

			mainStatus := getMainNetworkStatus(testVM)
			Expect(mainStatus).NotTo(BeNil())
			ipaName = mainStatus.IPAddressName
			Expect(ipaName).NotTo(BeEmpty())

			pod := getActiveVirtLauncherPod(ctx, f, testVM.Name, testVM.Namespace)
			Expect(pod).NotTo(BeNil())
			sourceNode = pod.Spec.NodeName
			Expect(sourceNode).NotTo(BeEmpty())
		})

		By("Verify the guest has the address before the migration", func() {
			eventually.SSHReadyAsRoot(f, testVM, framework.LongTimeout)
			expectGuestHasMainAddress(f, testVM, address)
		})

		By("Migrate the VM", func() {
			vmop := util.MigrateVirtualMachine(f, testVM)
			util.UntilVMOPMigrationSucceeded(ctx, vmop, framework.MaxTimeout)
			if err := testVMObs.WaitFor(vmobs.HaveMigrationSucceeded(), framework.MaxTimeout); err != nil {
				util.SkipIfKnownMigrationFailureWithContext(ctx, testVM)
				Expect(err).NotTo(HaveOccurred())
			}
		})

		By("Verify the target pod runs on another node with the same address", func() {
			eventually.UntilAssertion(func(g Gomega) {
				pod := getActiveVirtLauncherPod(ctx, f, testVM.Name, testVM.Namespace)
				g.Expect(pod).NotTo(BeNil())
				g.Expect(pod.Spec.NodeName).NotTo(Equal(sourceNode), "the machine should have moved to another node")
			}, framework.MiddleTimeout, eventually.WithPolling(time.Second))

			updated := refreshVM(ctx, f, testVM)
			Expect(updated.Status.IPAddress).To(Equal(address), "the address must survive the live migration")
			expectPodHasBothAddressAnnotations(ctx, f, updated, ipaName, address)
		})

		By("Verify the guest still has the address after the migration", func() {
			expectGuestHasMainAddress(f, testVM, address)
		})
	})
})

// Networks are not set: the VM gets the implicit Main network.
func buildMainIPAMVM(name, ns, vdRootName string, extraOpts ...vm.Option) *v1alpha2.VirtualMachine {
	opts := []vm.Option{
		vm.WithName(name),
		vm.WithNamespace(ns),
		vm.WithBootloader(v1alpha2.BIOS),
		vm.WithCPU(1, ptr.To(object.CustomImageVMCoreFraction)),
		vm.WithMemory(resource.MustParse(object.CustomImageVMMemory)),
		vm.WithRestartApprovalMode(v1alpha2.Manual),
		vm.WithVirtualMachineClass(object.DefaultVMClass),
		vm.WithLiveMigrationPolicy(v1alpha2.PreferSafeMigrationPolicy),
		vm.WithBlockDeviceRefs(v1alpha2.BlockDeviceSpecRef{
			Kind: v1alpha2.VirtualDiskKind,
			Name: vdRootName,
		}),
	}
	opts = append(opts, extraOpts...)
	return vm.New(opts...)
}

func getMainNetworkStatus(vm *v1alpha2.VirtualMachine) *v1alpha2.NetworksStatus {
	for i, n := range vm.Status.Networks {
		if n.Type == v1alpha2.NetworksTypeMain {
			return &vm.Status.Networks[i]
		}
	}
	return nil
}

func findMainIPAddressByVMUID(ctx context.Context, g Gomega, vm *v1alpha2.VirtualMachine) *sdnapi.IPAddress {
	ipas := &sdnapi.IPAddressList{}
	g.Expect(framework.GetClients().GenericClient().List(ctx, ipas, crclient.InNamespace(vm.Namespace),
		crclient.MatchingLabels{annotations.LabelVirtualMachineUID: string(vm.UID)})).To(Succeed())
	for i := range ipas.Items {
		if network.IsMainNetworkIPAddress(&ipas.Items[i]) {
			return &ipas.Items[i]
		}
	}
	return nil
}

func isOwnedByVM(ipa *sdnapi.IPAddress, vm *v1alpha2.VirtualMachine) bool {
	for _, ref := range ipa.GetOwnerReferences() {
		if ref.Kind == v1alpha2.VirtualMachineKind && ref.UID == vm.UID {
			return true
		}
	}
	return false
}

func getSDNIPAddress(ctx context.Context, g Gomega, ns, name string) *sdnapi.IPAddress {
	ipa := &sdnapi.IPAddress{}
	err := framework.GetClients().GenericClient().Get(ctx, k8stypes.NamespacedName{Namespace: ns, Name: name}, ipa)
	if k8serrors.IsNotFound(err) {
		return nil
	}
	g.Expect(err).NotTo(HaveOccurred())
	return ipa
}

func restartMainIPAMVM(ctx context.Context, f *framework.Framework, vm *v1alpha2.VirtualMachine, vmObs vmobs.Observer) {
	GinkgoHelper()

	previousRunningTime := time.Now()
	//nolint:contextcheck // RebootVirtualMachineByVMOP creates the VMOP on its own context
	util.RebootVirtualMachineByVMOP(f, vm)
	expectVMRebooted(ctx, vmObs, vm, previousRunningTime, framework.LongTimeout)
	Expect(vmObs.WaitFor(vmobs.BeRunning(), framework.LongTimeout)).To(Succeed())
}

// expectPodHasBothAddressAnnotations verifies the running virt-launcher pod carries
// both: cni.cilium.io/ipAddress for a node whose cni-cilium predates the Main
// interface spec, and the Main interface spec in networks-spec for a node that reads
// it. Both must name the same address.
func expectPodHasBothAddressAnnotations(ctx context.Context, f *framework.Framework, vm *v1alpha2.VirtualMachine, ipaName, address string) {
	GinkgoHelper()

	eventually.UntilAssertion(func(g Gomega) {
		pod := getActiveVirtLauncherPod(ctx, f, vm.Name, vm.Namespace)
		g.Expect(pod).NotTo(BeNil(), "active virt-launcher pod should exist")
		g.Expect(pod.Annotations).To(HaveKeyWithValue(annCiliumIPAddress, address),
			"the pod keeps cni.cilium.io/ipAddress next to the Main interface spec")

		var specs network.InterfaceSpecList
		g.Expect(json.Unmarshal([]byte(pod.Annotations[annotations.AnnNetworksSpec]), &specs)).To(Succeed())
		mainSpec := specs.MainWithAddress()
		g.Expect(mainSpec).NotTo(BeNil(), "networks-spec should have a Main interface spec with an address")
		g.Expect(mainSpec.InterfaceName).To(Equal(network.MainPodInterfaceName))
		g.Expect(mainSpec.IPAddressNames).To(ContainElement(ipaName))
		g.Expect(mainSpec.IPAddress).To(Equal(address), "the Main interface spec must carry the resolved address")

		// The sdn module accounts the address to the pod that runs on it, and that
		// is how it learns which node to program the route to: without this the
		// machine has an address and nothing reaches it.
		ipa := getSDNIPAddress(ctx, g, vm.Namespace, ipaName)
		g.Expect(ipa).NotTo(BeNil(), "the IPAddress %s should exist", ipaName)
		g.Expect(ipa.Status.UsedByPods).To(ContainElement(vm.Namespace + "/" + pod.Name))
	}, framework.MiddleTimeout, eventually.WithPolling(time.Second))
}

// The resources may agree while the guest does not.
func expectGuestHasMainAddress(f *framework.Framework, vm *v1alpha2.VirtualMachine, address string) {
	GinkgoHelper()

	// EXCEPTION: guest-side wait (addresses over SSH), not a Kubernetes resource:
	// nothing to observe via an Observer.
	eventually.UntilAssertion(func(g Gomega) {
		// The busybox ip of the guest image has no JSON output; the prefix length
		// delimits the address, so 10.0.0.1 does not match 10.0.0.12.
		output, err := f.SSHCommand(vm.Name, vm.Namespace, "ip -4 addr show eth0", framework.WithSSHUser("root"))
		g.Expect(err).NotTo(HaveOccurred(), "failed to execute command: %s", output)
		g.Expect(output).To(ContainSubstring("inet "+address+"/"), "eth0 of the guest should have the Main address")
	}, framework.MiddleTimeout, eventually.WithPolling(3*time.Second))
}

// pickFreeMainAddress asks the sdn module for a free address of the Main network:
// an Auto IPAddress is created in the namespace of the spec, and its address is
// returned once it is released again. The sdn module accounts the reservations
// of deleted resources, which the spec cannot see; the reservation this probe
// leaves behind is released by util.CreateSDNIPAddress.
func pickFreeMainAddress(ctx context.Context, namespace string) string {
	GinkgoHelper()

	probe := &sdnapi.IPAddress{
		ObjectMeta: metav1.ObjectMeta{Name: "main-ip-probe", Namespace: namespace},
		Spec: sdnapi.IPAddressSpec{
			NetworkRef: network.SDNNetworkRef(v1alpha2.NetworksSpec{Type: v1alpha2.NetworksTypeMain}),
			Type:       sdnapi.IPAddressTypeAuto,
		},
	}
	genericClient := framework.GetClients().GenericClient()
	Expect(genericClient.Create(ctx, probe)).To(Succeed())

	var address string
	eventually.UntilAssertion(func(g Gomega) {
		ipa := getSDNIPAddress(ctx, g, namespace, probe.Name)
		g.Expect(ipa).NotTo(BeNil())
		g.Expect(network.IPAddressAllocated(ipa)).To(BeTrue(), "the probe IPAddress should be allocated")
		address = ipa.Status.Address
	}, framework.MiddleTimeout, eventually.WithPolling(time.Second))

	Expect(genericClient.Delete(ctx, probe)).To(Succeed())
	eventually.UntilAssertion(func(g Gomega) {
		g.Expect(getSDNIPAddress(ctx, g, namespace, probe.Name)).To(BeNil())
	}, framework.MiddleTimeout, eventually.WithPolling(time.Second))
	return address
}
