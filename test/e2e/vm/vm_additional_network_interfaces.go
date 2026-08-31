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
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/util/retry"
	"k8s.io/utils/ptr"
	crclient "sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/deckhouse/virtualization-controller/pkg/builder/vd"
	"github.com/deckhouse/virtualization-controller/pkg/builder/vm"
	"github.com/deckhouse/virtualization-controller/pkg/builder/vmop"
	"github.com/deckhouse/virtualization-controller/pkg/controller/conditions"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/vmcondition"
	"github.com/deckhouse/virtualization/test/e2e/eventually"
	"github.com/deckhouse/virtualization/test/e2e/internal/framework"
	"github.com/deckhouse/virtualization/test/e2e/internal/label"
	"github.com/deckhouse/virtualization/test/e2e/internal/network"
	"github.com/deckhouse/virtualization/test/e2e/internal/object"
	vmobs "github.com/deckhouse/virtualization/test/e2e/internal/observer/vm"
	vmopobs "github.com/deckhouse/virtualization/test/e2e/internal/observer/vmop"
	"github.com/deckhouse/virtualization/test/e2e/internal/precheck"
	"github.com/deckhouse/virtualization/test/e2e/internal/util"
)

type additionalNetworkTestCase struct {
	vmBarHasMainNetwork bool
	vmFooAdditionalIP   string
	vmBarAdditionalIP   string
}

const (
	// L2OnlyNetworkVLANID is a ClusterNetwork without an IPAM pool. Tests that
	// configure IPs manually inside the guest OS (cloud-init static) use it to
	// avoid conflicting with SDN-allocated addresses from a pool.
	L2OnlyNetworkVLANID = 4007
	// WithIPPoolNetworkVLANID is a ClusterNetwork with an IPAM pool bound
	// (e2e-ipam-pool, 192.168.200.0/24). Used by IPAM tests and as a second
	// additional network in interface-name-persistence tests.
	WithIPPoolNetworkVLANID = 4006
)

var _ = Describe("VirtualMachineAdditionalNetworkInterfaces", Label(label.SIGCompute, precheck.PrecheckSDN), func() {
	var (
		vdFooRoot *v1alpha2.VirtualDisk
		vdBarRoot *v1alpha2.VirtualDisk
		vmFoo     *v1alpha2.VirtualMachine
		vmBar     *v1alpha2.VirtualMachine
		vmFooObs  vmobs.Observer
		vmBarObs  vmobs.Observer

		ctx context.Context
		f   *framework.Framework
	)

	BeforeEach(func() {
		ctx = context.Background()
		f = framework.NewFramework("vm-additional-network-interfaces")
		DeferCleanup(f.After)

		f.Before()
	})

	DescribeTable("verifies additional network interfaces and connectivity before and after migration",
		func(tc additionalNetworkTestCase) {
			By("Environment preparation", func() {
				ns := f.Namespace().Name

				// The static addresses are applied at boot by the custom
				// NoCloud handler (S04cloudinit executes the runcmd subset of the
				// provisioning below) — required for the "vm-bar without Main"
				// entry, whose guest has no SSH path.
				vdFooRoot = object.NewVDFromCVI("vd-foo-root", ns, object.PrecreatedCVICustomBIOS,
					vd.WithSize(ptr.To(resource.MustParse(vdCustomImageSize))),
				)
				vdBarRoot = object.NewVDFromCVI("vd-bar-root", ns, object.PrecreatedCVICustomBIOS,
					vd.WithSize(ptr.To(resource.MustParse(vdCustomImageSize))),
				)

				// vm-foo always has Main + ClusterNetwork so we can SSH to it.
				vmFoo = buildVMWithNetworks("vm-foo", ns, vdFooRoot.Name, tc.vmFooAdditionalIP, true)
				vmBar = buildVMWithNetworks("vm-bar", ns, vdBarRoot.Name, tc.vmBarAdditionalIP, tc.vmBarHasMainNetwork)

				err := f.CreateWithDeferredDeletion(ctx, vdFooRoot, vdBarRoot, vmFoo, vmBar)
				Expect(err).NotTo(HaveOccurred())

				vmFooObs = vmobs.StartObserver(ctx, f, vmFoo)
				vmFooObs.Never(vmobs.BeFailed())
				vmBarObs = vmobs.StartObserver(ctx, f, vmBar)
				vmBarObs.Never(vmobs.BeFailed())

				err = vmFooObs.WaitFor(vmobs.BeRunning(), framework.LongTimeout)
				Expect(err).NotTo(HaveOccurred())
				err = vmBarObs.WaitFor(vmobs.BeRunning(), framework.LongTimeout)
				Expect(err).NotTo(HaveOccurred())
				eventually.SSHReadyAsRoot(f, vmFoo, framework.LongTimeout)
				if tc.vmBarHasMainNetwork {
					eventually.SSHReadyAsRoot(f, vmBar, framework.LongTimeout)
				}
			})

			// If test fail due this timeout, rollback in test waiting for agent to be ready.
			By("Wait for additional network interfaces to be ready", func() {
				err := vmFooObs.WaitFor(haveNetworkReady(), framework.LongTimeout)
				Expect(err).NotTo(HaveOccurred())
				err = vmBarObs.WaitFor(haveNetworkReady(), framework.LongTimeout)
				Expect(err).NotTo(HaveOccurred())
			})

			By("Check connectivity between VMs via additional network", func() {
				checkConnectivityBetweenVMs(f, vmFoo, vmBar, tc.vmBarHasMainNetwork, tc.vmBarAdditionalIP, tc.vmFooAdditionalIP)
			})

			var vmopFooObs, vmopBarObs vmopobs.Observer
			By("Create VMOPs to trigger migration", func() {
				vmopFoo := util.MigrateVirtualMachine(f, vmFoo, vmop.WithGenerateName("vmop-migrate-foo-"))
				vmopBar := util.MigrateVirtualMachine(f, vmBar, vmop.WithGenerateName("vmop-migrate-bar-"))
				vmopFooObs = vmopobs.StartObserver(ctx, vmopFoo)
				vmopBarObs = vmopobs.StartObserver(ctx, vmopBar)
			})

			By("Wait for migration to complete", func() {
				// MaxTimeout: with parallelMigrationsPerCluster=3 the two VMIMs of this spec
				// queue behind slow migrations from parallel suites and may wait longer than 300s.
				err := vmopFooObs.WaitFor(vmopobs.BeCompleted(), framework.MaxTimeout)
				Expect(err).NotTo(HaveOccurred())
				err = vmopBarObs.WaitFor(vmopobs.BeCompleted(), framework.MaxTimeout)
				Expect(err).NotTo(HaveOccurred())
				for vmRef, obs := range map[*v1alpha2.VirtualMachine]vmobs.Observer{vmFoo: vmFooObs, vmBar: vmBarObs} {
					err := obs.WaitFor(vmobs.HaveMigrationSucceeded(), framework.MaxTimeout)
					if err != nil {
						// TODO: remove temporary migration skip logic when both known issues
						// are fixed: kubevirt "client socket is closed" and Volume(s)UpdateError.
						util.SkipIfKnownMigrationFailureWithContext(ctx, vmRef)
					}
					Expect(err).NotTo(HaveOccurred())
				}
			})

			By("Check Cilium agents after migration", func() {
				network.EnsureCiliumAgents(ctx, f.Kubectl(), vmFoo.Name, f.Namespace().Name)

				if tc.vmBarHasMainNetwork {
					network.EnsureCiliumAgents(ctx, f.Kubectl(), vmBar.Name, f.Namespace().Name)
				}
			})

			By("Check VM can reach external network after migration", func() {
				expectExternalConnectivityAsRoot(f, vmFoo.Name, f.Namespace().Name)

				if tc.vmBarHasMainNetwork {
					expectExternalConnectivityAsRoot(f, vmBar.Name, f.Namespace().Name)
				}
			})

			By("Wait for additional network interfaces to be ready after migration", func() {
				err := vmFooObs.WaitFor(haveNetworkReady(), framework.LongTimeout)
				Expect(err).NotTo(HaveOccurred())
				err = vmBarObs.WaitFor(haveNetworkReady(), framework.LongTimeout)
				Expect(err).NotTo(HaveOccurred())
			})

			By("Check connectivity between VMs via additional network after migration", func() {
				checkConnectivityBetweenVMs(f, vmFoo, vmBar, tc.vmBarHasMainNetwork, tc.vmBarAdditionalIP, tc.vmFooAdditionalIP)
			})
		},
		Entry("Main + additional network", additionalNetworkTestCase{vmBarHasMainNetwork: true, vmFooAdditionalIP: "192.168.42.10", vmBarAdditionalIP: "192.168.42.11"}),
		Entry("Only additional network (vm-bar without Main)", additionalNetworkTestCase{vmBarHasMainNetwork: false, vmFooAdditionalIP: "192.168.42.12", vmBarAdditionalIP: "192.168.42.13"}),
	)

	Describe("verifies interface name persistence after removing middle ClusterNetwork", func() {
		// Ubuntu needs the EFI bootloader and more than the custom-image sizing
		// buildVMWithNetworks defaults to.
		cloudInitOpt := vm.WithProvisioningUserData(object.UbuntuCloudInit)
		bootloaderOpt := vm.WithBootloader(v1alpha2.EFI)
		cpuOpt := vm.WithCPU(1, ptr.To("50%"))
		memoryOpt := vm.WithMemory(resource.MustParse("512Mi"))

		var (
			vdRoot *v1alpha2.VirtualDisk
			vm     *v1alpha2.VirtualMachine
			vmObs  vmobs.Observer
		)

		// The interfaces the guest names after the ACPI index of each network: the
		// middle ClusterNetwork is the one being removed, and the last one has to keep
		// its name across the removal and the reboot instead of taking the freed one.
		const (
			middleInterfaceName = "eno2"
			lastInterfaceName   = "eno3"
		)

		It("should preserve interface name after removing middle ClusterNetwork and rebooting", func() {
			By("Create VM with Main network and two additional ClusterNetworks", func() {
				ns := f.Namespace().Name

				// Stays on Ubuntu + cloud-init: interface-name persistence relies on
				// the distro's predictable interface naming and the EFI bootloader;
				// the custom image supports neither.
				vdRoot = object.NewVDFromCVI("vd-root", ns, object.PrecreatedCVIUbuntu)

				vm = buildVMWithNetworks("vm", ns, vdRoot.Name, "192.168.1.20", true, cloudInitOpt, bootloaderOpt, cpuOpt, memoryOpt)
				vm.Spec.Networks = append(vm.Spec.Networks, v1alpha2.NetworksSpec{
					Type: v1alpha2.NetworksTypeClusterNetwork,
					Name: util.ClusterNetworkName(WithIPPoolNetworkVLANID),
				})

				err := f.CreateWithDeferredDeletion(ctx, vdRoot, vm)
				Expect(err).NotTo(HaveOccurred())

				vmObs = vmobs.StartObserver(ctx, f, vm)
				vmObs.Never(vmobs.BeFailed())
				err = vmObs.WaitFor(vmobs.BeRunning(), framework.LongTimeout)
				Expect(err).NotTo(HaveOccurred())
				err = vmObs.WaitFor(vmobs.BeAgentReady(), framework.LongTimeout)
				Expect(err).NotTo(HaveOccurred())
				err = vmObs.WaitFor(haveNetworkReady(), framework.LongTimeout)
				Expect(err).NotTo(HaveOccurred())
			})

			By("Get interface names via SSH", func() {
				eventually.SSHReady(f, vm, framework.LongTimeout)
				checkGuestInterfaceNames(f, vm.Name, vm.Namespace,
					[]string{middleInterfaceName, lastInterfaceName}, nil)
			})

			By("Remove middle ClusterNetwork from VM spec", func() {
				setVMNetworks(ctx, f, vm, func(networks []v1alpha2.NetworksSpec) []v1alpha2.NetworksSpec {
					return []v1alpha2.NetworksSpec{networks[0], networks[2]}
				})
			})

			By("Reboot VM via VMOP", func() {
				err := f.Clients.GenericClient().Get(ctx, crclient.ObjectKeyFromObject(vm), vm)
				Expect(err).NotTo(HaveOccurred())

				runningCondition, _ := conditions.GetCondition(vmcondition.TypeRunning, vm.Status.Conditions)
				previousRunningTime := runningCondition.LastTransitionTime.Time

				util.RebootVirtualMachineByVMOP(f, vm)

				err = vmObs.WaitFor(vmobs.BeRebootedAfter(previousRunningTime), framework.LongTimeout)
				if err != nil {
					util.SkipIfGuestPowerActionStuck(ctx, crclient.ObjectKeyFromObject(vm))
				}
				Expect(err).NotTo(HaveOccurred())
				err = vmObs.WaitFor(vmobs.BeRunning(), framework.LongTimeout)
				Expect(err).NotTo(HaveOccurred())
				err = vmObs.WaitFor(vmobs.BeAgentReady(), framework.LongTimeout)
				Expect(err).NotTo(HaveOccurred())
				err = vmObs.WaitFor(haveNetworkReady(), framework.LongTimeout)
				Expect(err).NotTo(HaveOccurred())
			})

			By("Verify the remaining interface kept its name", func() {
				eventually.SSHReady(f, vm, framework.LongTimeout)
				checkGuestInterfaceNames(f, vm.Name, vm.Namespace,
					[]string{lastInterfaceName}, []string{middleInterfaceName})
			})
		})
	})

	Describe("verifies hotplug and hotunplug of additional network interfaces", func() {
		const countNonLoopbackInterfacesCmd = "ip -o link show | grep -v 'lo:' | wc -l"

		var (
			vdRoot    *v1alpha2.VirtualDisk
			testVM    *v1alpha2.VirtualMachine
			testVMObs vmobs.Observer
		)

		// getIfaceCount counts the non-loopback interfaces of the guest, or returns -1 when
		// the guest cannot be reached. Every caller polls it, and reaching the guest over
		// SSH fails on its own from time to time - a refused connection, a closed
		// websocket, a timeout during the banner exchange - so asserting in here would let
		// a single hiccup decide the outcome instead of being retried.
		getIfaceCount := func() int {
			output, err := f.SSHCommand(testVM.Name, testVM.Namespace, countNonLoopbackInterfacesCmd)
			if err != nil {
				return -1
			}
			count, err := strconv.Atoi(strings.TrimSpace(output))
			if err != nil {
				return -1
			}
			return count
		}

		It("should attach and detach ClusterNetwork on a running VM without reboot", func() {
			var initialIfaceCount int

			By("Create VM with only Main network", func() {
				ns := f.Namespace().Name
				vdRoot = object.NewVDFromCVI("vd-root", ns, object.PrecreatedCVIUbuntu)

				testVM = vm.New(
					vm.WithName("vm-hotplug"),
					vm.WithNamespace(ns),
					vm.WithBootloader(v1alpha2.EFI),
					vm.WithCPU(1, ptr.To("5%")),
					vm.WithMemory(resource.MustParse("512Mi")),
					vm.WithRestartApprovalMode(v1alpha2.Manual),
					vm.WithVirtualMachineClass(object.DefaultVMClass),
					vm.WithLiveMigrationPolicy(v1alpha2.PreferSafeMigrationPolicy),
					vm.WithProvisioningUserData(object.UbuntuCloudInit),
					vm.WithBlockDeviceRefs(v1alpha2.BlockDeviceSpecRef{
						Kind: v1alpha2.VirtualDiskKind,
						Name: vdRoot.Name,
					}),
					vm.WithNetwork(v1alpha2.NetworksSpec{Type: v1alpha2.NetworksTypeMain}),
				)

				err := f.CreateWithDeferredDeletion(context.Background(), vdRoot, testVM)
				Expect(err).NotTo(HaveOccurred())

				testVMObs = vmobs.StartObserver(ctx, f, testVM)
				testVMObs.Never(vmobs.BeFailed())
				err = testVMObs.WaitFor(vmobs.BeRunning(), framework.LongTimeout)
				Expect(err).NotTo(HaveOccurred())
				err = testVMObs.WaitFor(vmobs.BeAgentReady(), framework.LongTimeout)
				Expect(err).NotTo(HaveOccurred())
				eventually.SSHReady(f, testVM, framework.LongTimeout)

				// A non-Main network change must never ask for a restart: enforced as
				// an invariant on every VM status update from here to the end of the
				// spec (it replaces the previous bounded Consistently windows).
				testVMObs.Never(func(vm *v1alpha2.VirtualMachine) (bool, error) {
					cond, _ := conditions.GetCondition(vmcondition.TypeAwaitingRestartToApplyConfiguration, vm.Status.Conditions)
					if cond.Status == metav1.ConditionTrue {
						return true, fmt.Errorf("VM must not require restart for non-Main network change")
					}
					return false, nil
				})

				// EXCEPTION: guest-side wait (interface count over SSH), not a
				// Kubernetes resource - nothing to observe via an Observer.
				eventually.UntilMatch(getIfaceCount, BeNumerically(">=", 1), framework.LongTimeout,
					eventually.WithPolling(3*time.Second),
					eventually.WithExplanation("the guest should report at least one non-loopback interface"))
				initialIfaceCount = getIfaceCount()
				Expect(initialIfaceCount).To(BeNumerically(">=", 1),
					"VM should have at least one non-loopback interface")
			})

			By("Hotplug: add a ClusterNetwork to spec.networks", func() {
				setVMNetworks(ctx, f, testVM, func(networks []v1alpha2.NetworksSpec) []v1alpha2.NetworksSpec {
					return append(networks, v1alpha2.NetworksSpec{
						Type: v1alpha2.NetworksTypeClusterNetwork,
						Name: util.ClusterNetworkName(L2OnlyNetworkVLANID),
					})
				})
			})

			By("Verify new interface appears in the guest OS", func() {
				err := testVMObs.WaitFor(haveNetworkReady(), framework.LongTimeout)
				Expect(err).NotTo(HaveOccurred())
				// EXCEPTION: guest-side wait (interface count over SSH), not a
				// Kubernetes resource — nothing to observe via an Observer.
				eventually.UntilMatch(getIfaceCount, Equal(initialIfaceCount+1), framework.LongTimeout,
					eventually.WithPolling(3*time.Second),
					eventually.WithExplanation("new interface should appear after hotplug"))
			})

			By("Hotunplug: remove the ClusterNetwork from spec.networks", func() {
				setVMNetworks(ctx, f, testVM, func(networks []v1alpha2.NetworksSpec) []v1alpha2.NetworksSpec {
					return []v1alpha2.NetworksSpec{networks[0]}
				})
			})

			By("Verify interface disappears from the guest OS", func() {
				// EXCEPTION: guest-side wait (interface count over SSH), not a
				// Kubernetes resource — nothing to observe via an Observer.
				eventually.UntilMatch(getIfaceCount, Equal(initialIfaceCount), framework.LongTimeout,
					eventually.WithPolling(3*time.Second),
					eventually.WithExplanation("interface should disappear after hotunplug"))
			})

			By("Verify VM phase stayed Running throughout", func() {
				err := f.Clients.GenericClient().Get(context.Background(), crclient.ObjectKeyFromObject(testVM), testVM)
				Expect(err).NotTo(HaveOccurred())
				Expect(string(testVM.Status.Phase)).To(Equal(string(v1alpha2.MachineRunning)))
			})
		})
	})
})

// haveNetworkReady reports the VM's NetworkReady condition is True. Intended
// for use with the VM observer's WaitFor.
func haveNetworkReady() vmobs.Predicate {
	return func(vm *v1alpha2.VirtualMachine) (bool, error) {
		cond, _ := conditions.GetCondition(vmcondition.TypeNetworkReady, vm.Status.Conditions)
		return cond.Status == metav1.ConditionTrue, nil
	}
}

// buildVMWithNetworks creates a VM with optional Main + ClusterNetwork.
// If hasMain is false, only ClusterNetwork is added (VM without Main network).
// The additional network interface is eth1 when hasMain is true, eth0 otherwise.
// The bootloader defaults to BIOS, which is what the custom-image specs boot;
// the Ubuntu spec below overrides it, as its guest needs EFI.
func buildVMWithNetworks(name, ns, vdRootName, additionalIP string, hasMain bool, extraOpts ...vm.Option) *v1alpha2.VirtualMachine {
	opts := []vm.Option{
		vm.WithName(name),
		vm.WithNamespace(ns),
		vm.WithBootloader(v1alpha2.BIOS),
		vm.WithCPU(1, ptr.To(object.CustomImageVMCoreFraction)),
		vm.WithMemory(resource.MustParse(object.CustomImageVMMemory)),
		vm.WithRestartApprovalMode(v1alpha2.Manual),
		vm.WithVirtualMachineClass(object.DefaultVMClass),
		vm.WithLiveMigrationPolicy(v1alpha2.PreferSafeMigrationPolicy),
		vm.WithProvisioningUserData(cloudInitAdditionalNetwork(additionalIP, hasMain)),
		vm.WithBlockDeviceRefs(v1alpha2.BlockDeviceSpecRef{
			Kind: v1alpha2.VirtualDiskKind,
			Name: vdRootName,
		}),
	}
	if hasMain {
		opts = append(opts,
			vm.WithNetwork(v1alpha2.NetworksSpec{Type: v1alpha2.NetworksTypeMain}),
		)
	}
	opts = append(opts,
		vm.WithNetwork(v1alpha2.NetworksSpec{
			Type: v1alpha2.NetworksTypeClusterNetwork,
			Name: util.ClusterNetworkName(L2OnlyNetworkVLANID),
		}),
	)
	opts = append(opts, extraOpts...)
	return vm.New(opts...)
}

// cloudInitAdditionalNetwork returns provisioning that assigns the static
// address to the additional interface at boot. The custom image executes the
// runcmd subset via its NoCloud handler (S04cloudinit), which runs before both
// networking steps.
//
// With a Main network the additional interface is eth1, and plain `ip addr add`
// is enough: the extra-NIC DHCP step (S41extranics) leaves an interface that
// already carries an address alone.
//
// Without a Main network the additional interface is eth0, and `ip addr add`
// does not survive: the image ships a fixed `iface eth0 inet dhcp` stanza, and
// udhcpc always starts with a `deconfig` that flushes the interface. Since the
// L2-only network has no DHCP server, the address would be wiped and never
// replaced, so the stanza is rewritten to a static one instead and ifup
// configures the address itself.
func cloudInitAdditionalNetwork(additionalIP string, hasMain bool) string {
	if !hasMain {
		return fmt.Sprintf(`#cloud-config
runcmd:
  - printf 'auto lo\niface lo inet loopback\n\nauto eth0\niface eth0 inet static\n  address %s\n  netmask 255.255.255.0\n' > /etc/network/interfaces
`, additionalIP)
	}
	return fmt.Sprintf(`#cloud-config
runcmd:
  - ip link set eth1 up
  - ip addr add %s/24 dev eth1
`, additionalIP)
}

func checkConnectivityBetweenVMs(f *framework.Framework, vmFoo, vmBar *v1alpha2.VirtualMachine, vmBarHasMainNetwork bool, vmBarAdditionalIP, vmFooAdditionalIP string) {
	GinkgoHelper()

	pingCmd := "ping -c 2 -W 2 -w 5 -q %s"
	expectedPacketLoss := "0"

	By(fmt.Sprintf("VM %s should have connectivity to %s (vm-bar)", vmFoo.Name, vmBarAdditionalIP))
	checkPingPacketLoss(f, vmFoo.Name, vmFoo.Namespace, fmt.Sprintf(pingCmd, vmBarAdditionalIP), expectedPacketLoss)

	if vmBarHasMainNetwork {
		By(fmt.Sprintf("VM %s should have connectivity to %s (vm-foo)", vmBar.Name, vmFooAdditionalIP))
		checkPingPacketLoss(f, vmBar.Name, vmBar.Namespace, fmt.Sprintf(pingCmd, vmFooAdditionalIP), expectedPacketLoss)
	}
}

const (
	Interval = 1 * time.Second
	Timeout  = 90 * time.Second
	// SSH command timeout should be safely above command-level deadlines (e.g. ping -w 5)
	// to avoid killing healthy commands at the timeout boundary.
	SSHCommandTimeout = 15 * time.Second
)

// setVMNetworks rewrites spec.networks of the virtual machine, retrying the write when a
// concurrent status update wins the race - the controller writes to the machine on every
// reconcile, so a plain read-modify-write loses often enough to matter.
func setVMNetworks(
	ctx context.Context,
	f *framework.Framework,
	vm *v1alpha2.VirtualMachine,
	rewrite func([]v1alpha2.NetworksSpec) []v1alpha2.NetworksSpec,
) {
	GinkgoHelper()
	Expect(retry.RetryOnConflict(retry.DefaultRetry, func() error {
		if err := f.Clients.GenericClient().Get(ctx, crclient.ObjectKeyFromObject(vm), vm); err != nil {
			return err
		}
		vm.Spec.Networks = rewrite(vm.Spec.Networks)
		return f.Clients.GenericClient().Update(ctx, vm)
	})).To(Succeed())
}

func checkPingPacketLoss(f *framework.Framework, vmName, vmNamespace, cmd, expectedPacketLoss string) {
	GinkgoHelper()
	packetLossRE := regexp.MustCompile(`([0-9]+)%\s*packet loss`)

	// EXCEPTION: guest-side wait (ping packet loss over SSH), not a Kubernetes
	// resource — nothing to observe via an Observer.
	eventually.UntilMatch(func() (string, error) {
		res, err := f.SSHCommand(vmName, vmNamespace, cmd, framework.WithSSHUser("root"), framework.WithSSHTimeout(SSHCommandTimeout))
		if err != nil {
			return "", fmt.Errorf("cmd: %s\nstderr: %w", cmd, err)
		}
		match := packetLossRE.FindStringSubmatch(res)
		if len(match) < 2 {
			return "", fmt.Errorf("cmd: %s\nfailed to parse packet loss from output: %q", cmd, strings.TrimSpace(res))
		}

		return match[1], nil
	}, Equal(expectedPacketLoss), Timeout, eventually.WithPolling(Interval))
}

// checkGuestInterfaceNames waits until the guest reports every name in present and none
// of the names in absent. The names are the invariant, not their order: the guest indexes
// interfaces by PCI slot while it names them after the ACPI index, so the order of the
// internal spec - which a re-assigned MAC address rewrites - decides which one comes last
// even when every name is what it should be.
func checkGuestInterfaceNames(f *framework.Framework, vmName, vmNamespace string, present, absent []string) {
	GinkgoHelper()
	// EXCEPTION: guest-side wait (interface list over SSH), not a Kubernetes
	// resource — nothing to observe via an Observer.
	eventually.UntilAssertion(func(g Gomega) {
		cmd := "ip -j link show"
		result, err := f.SSHCommand(vmName, vmNamespace, cmd, framework.WithSSHTimeout(SSHCommandTimeout))
		g.Expect(err).NotTo(HaveOccurred(), "failed to execute command: %s", result)

		var links IPLinks
		g.Expect(json.Unmarshal([]byte(result), &links)).To(Succeed(), "failed to parse ip JSON output: %s", result)

		var names []string
		for _, link := range links {
			if link.IFName == "lo" {
				continue
			}
			names = append(names, link.IFName)
		}

		for _, name := range present {
			g.Expect(names).To(ContainElement(name), "the guest should carry interface %s", name)
		}
		for _, name := range absent {
			g.Expect(names).NotTo(ContainElement(name), "the guest should have let interface %s go", name)
		}
	}, Timeout, eventually.WithPolling(Interval))
}

// IPLinks represents the JSON output of ip -j link show command.
type IPLinks []IPLink

// IPLink represents a single network interface in the ip JSON output.
type IPLink struct {
	IFName string `json:"ifname"`
}
