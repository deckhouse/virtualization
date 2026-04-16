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
	"fmt"
	"strconv"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"
	crclient "sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/deckhouse/virtualization-controller/pkg/builder/vd"
	"github.com/deckhouse/virtualization-controller/pkg/builder/vm"
	"github.com/deckhouse/virtualization-controller/pkg/builder/vmop"
	"github.com/deckhouse/virtualization-controller/pkg/controller/conditions"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/vmcondition"
	"github.com/deckhouse/virtualization/test/e2e/internal/framework"
	"github.com/deckhouse/virtualization/test/e2e/internal/network"
	"github.com/deckhouse/virtualization/test/e2e/internal/object"
	"github.com/deckhouse/virtualization/test/e2e/internal/util"
)

type additionalNetworkTestCase struct {
	vmBarHasMainNetwork bool
	vmFooAdditionalIP   string
	vmBarAdditionalIP   string
}

const (
	additionalInterfaceVLANID       = 4006
	secondAdditionalInterfaceVLANID = 4007
)

func expectClusterNetworkExists(f *framework.Framework, vlanID int) {
	Expect(util.IsClusterNetworkExists(f, vlanID)).To(BeTrue(),
		fmt.Sprintf("Cluster network %s does not exist. Create it first: %s", util.ClusterNetworkName(vlanID), util.ClusterNetworkCreateCommand(vlanID)))
}

var _ = Describe("VirtualMachineAdditionalNetworkInterfaces", func() {
	var (
		vdFooRoot *v1alpha2.VirtualDisk
		vdBarRoot *v1alpha2.VirtualDisk
		vmFoo     *v1alpha2.VirtualMachine
		vmBar     *v1alpha2.VirtualMachine

		f = framework.NewFramework("vm-additional-network")
	)

	BeforeEach(func() {
		DeferCleanup(f.After)

		f.Before()

		if !util.IsSdnModuleEnabled(f) {
			Skip("SDN module is disabled. Skipping test.")
		}

		expectClusterNetworkExists(f, additionalInterfaceVLANID)
		expectClusterNetworkExists(f, secondAdditionalInterfaceVLANID)
	})

	DescribeTable("verifies additional network interfaces and connectivity before and after migration",
		func(tc additionalNetworkTestCase) {
			By("Environment preparation", func() {
				ns := f.Namespace().Name

				vdFooRoot = object.NewVDFromCVI("vd-foo-root", ns, object.PrecreatedCVIAlpineUEFIPerf,
					vd.WithSize(ptr.To(resource.MustParse("512Mi"))),
				)
				vdBarRoot = object.NewVDFromCVI("vd-bar-root", ns, object.PrecreatedCVIAlpineUEFIPerf,
					vd.WithSize(ptr.To(resource.MustParse("512Mi"))),
				)

				// vm-foo always has Main + ClusterNetwork so we can SSH to it.
				vmFoo = buildVMWithNetworks("vm-foo", ns, vdFooRoot.Name, tc.vmFooAdditionalIP, true)
				vmBar = buildVMWithNetworks("vm-bar", ns, vdBarRoot.Name, tc.vmBarAdditionalIP, tc.vmBarHasMainNetwork)

				err := f.CreateWithDeferredDeletion(context.Background(), vdFooRoot, vdBarRoot, vmFoo, vmBar)
				Expect(err).NotTo(HaveOccurred())

				util.UntilObjectPhase(string(v1alpha2.MachineRunning), framework.LongTimeout, vmFoo, vmBar)
				util.UntilSSHReady(f, vmFoo, framework.LongTimeout)
				if tc.vmBarHasMainNetwork {
					util.UntilSSHReady(f, vmBar, framework.LongTimeout)
				}

				By(fmt.Sprintf("Wait until vms %s and %s in phase running", vmFoo.GetName(), vmBar.GetName()), func() {
					util.UntilObjectPhase(string(v1alpha2.MachineRunning), framework.LongTimeout, vmFoo, vmBar)
				})
			})

			// If test fail due this timeout, rollback in test waiting for agent to be ready.
			By("Wait for additional network interfaces to be ready", func() {
				util.UntilConditionStatus(vmcondition.TypeNetworkReady.String(), "True", framework.LongTimeout, vmFoo, vmBar)
			})

			By("Check connectivity between VMs via additional network", func() {
				checkConnectivityBetweenVMs(f, vmFoo, vmBar, tc.vmBarHasMainNetwork, tc.vmBarAdditionalIP, tc.vmFooAdditionalIP)
			})

			By("Create VMOPs to trigger migration", func() {
				util.MigrateVirtualMachine(f, vmFoo, vmop.WithGenerateName("vmop-migrate-foo-"))
				util.MigrateVirtualMachine(f, vmBar, vmop.WithGenerateName("vmop-migrate-bar-"))
			})

			By("Wait for migration to complete", func() {
				util.UntilVMMigrationSucceeded(crclient.ObjectKeyFromObject(vmFoo), framework.LongTimeout)
				util.UntilVMMigrationSucceeded(crclient.ObjectKeyFromObject(vmBar), framework.LongTimeout)
			})

			By("Check Cilium agents after migration", func() {
				err := network.CheckCiliumAgents(context.Background(), f.Kubectl(), vmFoo.Name, f.Namespace().Name)
				Expect(err).NotTo(HaveOccurred(), "Cilium agents check for VM %s", vmFoo.Name)

				if tc.vmBarHasMainNetwork {
					err = network.CheckCiliumAgents(context.Background(), f.Kubectl(), vmBar.Name, f.Namespace().Name)
					Expect(err).NotTo(HaveOccurred(), "Cilium agents check for VM %s", vmBar.Name)
				}
			})

			By("Check VM can reach external network after migration", func() {
				network.CheckExternalConnectivity(f, vmFoo.Name, network.ExternalHost, network.HTTPStatusOk)

				if tc.vmBarHasMainNetwork {
					network.CheckExternalConnectivity(f, vmBar.Name, network.ExternalHost, network.HTTPStatusOk)
				}
			})

			By("Wait for additional network interfaces to be ready after migration", func() {
				util.UntilConditionStatus(vmcondition.TypeNetworkReady.String(), "True", framework.LongTimeout, vmFoo, vmBar)
			})

			By("Check connectivity between VMs via additional network after migration", func() {
				checkConnectivityBetweenVMs(f, vmFoo, vmBar, tc.vmBarHasMainNetwork, tc.vmBarAdditionalIP, tc.vmFooAdditionalIP)
			})
		},
		Entry("Main + additional network", additionalNetworkTestCase{vmBarHasMainNetwork: true, vmFooAdditionalIP: "192.168.42.10", vmBarAdditionalIP: "192.168.42.11"}),
		Entry("Only additional network (vm-bar without Main)", additionalNetworkTestCase{vmBarHasMainNetwork: false, vmFooAdditionalIP: "192.168.42.12", vmBarAdditionalIP: "192.168.42.13"}),
	)

	Describe("verifies interface name persistence after removing middle ClusterNetwork", func() {
		cloudInitOpt := vm.WithProvisioningUserData(object.UbuntuCloudInit)

		var (
			vdRoot *v1alpha2.VirtualDisk
			vm     *v1alpha2.VirtualMachine
		)

		const (
			getLastInterfaceNameCmd = "ip -o link show | tail -1 | cut -d: -f2 | awk \"{print \\$1}\""
		)

		It("should preserve interface name after removing middle ClusterNetwork and rebooting", func() {
			var lastInterfaceNameBeforeRemoval string

			By("Create VM with Main network and two additional ClusterNetworks", func() {
				ns := f.Namespace().Name

				vdRoot = object.NewVDFromCVI("vd-root", ns, object.PrecreatedCVIUbuntu)

				vm = buildVMWithNetworks("vm", ns, vdRoot.Name, "192.168.1.20", true, cloudInitOpt)
				vm.Spec.Networks = append(vm.Spec.Networks, v1alpha2.NetworksSpec{
					Type: v1alpha2.NetworksTypeClusterNetwork,
					Name: util.ClusterNetworkName(secondAdditionalInterfaceVLANID),
				})

				err := f.CreateWithDeferredDeletion(context.Background(), vdRoot, vm)
				Expect(err).NotTo(HaveOccurred())

				util.UntilObjectPhase(string(v1alpha2.MachineRunning), framework.LongTimeout, vm)
				util.UntilVMAgentReady(crclient.ObjectKeyFromObject(vm), framework.LongTimeout)
				util.UntilConditionStatus(vmcondition.TypeNetworkReady.String(), "True", framework.LongTimeout, vm)
			})

			By("Get last interface name via SSH", func() {
				util.UntilSSHReady(f, vm, framework.LongTimeout)
				output, err := f.SSHCommand(vm.Name, vm.Namespace, getLastInterfaceNameCmd)
				Expect(err).NotTo(HaveOccurred())
				lastInterfaceNameBeforeRemoval = strings.TrimSpace(output)
				Expect(lastInterfaceNameBeforeRemoval).NotTo(BeEmpty(), "Failed to get last interface name")
			})

			By("Remove middle ClusterNetwork from VM spec", func() {
				err := f.Clients.GenericClient().Get(context.Background(), crclient.ObjectKeyFromObject(vm), vm)
				Expect(err).NotTo(HaveOccurred())
				vm.Spec.Networks = []v1alpha2.NetworksSpec{vm.Spec.Networks[0], vm.Spec.Networks[2]}
				err = f.Clients.GenericClient().Update(context.Background(), vm)
				Expect(err).NotTo(HaveOccurred())
			})

			By("Reboot VM via VMOP", func() {
				err := f.Clients.GenericClient().Get(context.Background(), crclient.ObjectKeyFromObject(vm), vm)
				Expect(err).NotTo(HaveOccurred())

				runningCondition, _ := conditions.GetCondition(vmcondition.TypeRunning, vm.Status.Conditions)
				previousRunningTime := runningCondition.LastTransitionTime.Time

				util.RebootVirtualMachineByVMOP(f, vm)

				util.UntilVirtualMachineRebooted(crclient.ObjectKeyFromObject(vm), previousRunningTime, framework.LongTimeout)
				util.UntilObjectPhase(string(v1alpha2.MachineRunning), framework.LongTimeout, vm)
				util.UntilVMAgentReady(crclient.ObjectKeyFromObject(vm), framework.LongTimeout)
				util.UntilConditionStatus(vmcondition.TypeNetworkReady.String(), "True", framework.LongTimeout, vm)
			})

			By("Verify last interface name has not changed", func() {
				util.UntilSSHReady(f, vm, framework.LongTimeout)
				output, err := f.SSHCommand(vm.Name, vm.Namespace, getLastInterfaceNameCmd)
				Expect(err).NotTo(HaveOccurred())
				lastInterfaceNameAfterRemoval := strings.TrimSpace(output)
				Expect(lastInterfaceNameAfterRemoval).NotTo(BeEmpty(), "Failed to get last interface name")

				Expect(lastInterfaceNameAfterRemoval).To(Equal(lastInterfaceNameBeforeRemoval),
					fmt.Sprintf("Interface name changed from %s to %s after removing middle ClusterNetwork", lastInterfaceNameBeforeRemoval, lastInterfaceNameAfterRemoval))
			})
		})
	})

	Describe("verifies hotplug and hotunplug of additional network interfaces", func() {
		const countNonLoopbackInterfacesCmd = "ip -o link show | grep -v 'lo:' | wc -l"

		var (
			vdRoot *v1alpha2.VirtualDisk
			testVM *v1alpha2.VirtualMachine
		)

		getIfaceCount := func() int {
			GinkgoHelper()
			output, err := f.SSHCommand(testVM.Name, testVM.Namespace, countNonLoopbackInterfacesCmd)
			Expect(err).NotTo(HaveOccurred())
			count, err := strconv.Atoi(strings.TrimSpace(output))
			Expect(err).NotTo(HaveOccurred())
			return count
		}

		expectNoRestartRequired := func() {
			GinkgoHelper()
			Consistently(func(g Gomega) {
				err := f.Clients.GenericClient().Get(context.Background(), crclient.ObjectKeyFromObject(testVM), testVM)
				g.Expect(err).NotTo(HaveOccurred())
				cond, _ := conditions.GetCondition(vmcondition.TypeAwaitingRestartToApplyConfiguration, testVM.Status.Conditions)
				g.Expect(cond.Status).NotTo(Equal(metav1.ConditionTrue),
					"VM should not require restart for non-Main network change")
			}).WithTimeout(10 * time.Second).WithPolling(2 * time.Second).Should(Succeed())
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

				util.UntilObjectPhase(string(v1alpha2.MachineRunning), framework.LongTimeout, testVM)
				util.UntilVMAgentReady(crclient.ObjectKeyFromObject(testVM), framework.LongTimeout)
				util.UntilSSHReady(f, testVM, framework.LongTimeout)

				initialIfaceCount = getIfaceCount()
				Expect(initialIfaceCount).To(BeNumerically(">=", 1),
					"VM should have at least one non-loopback interface")
			})

			By("Hotplug: add a ClusterNetwork to spec.networks", func() {
				err := f.Clients.GenericClient().Get(context.Background(), crclient.ObjectKeyFromObject(testVM), testVM)
				Expect(err).NotTo(HaveOccurred())
				testVM.Spec.Networks = append(testVM.Spec.Networks, v1alpha2.NetworksSpec{
					Type: v1alpha2.NetworksTypeClusterNetwork,
					Name: util.ClusterNetworkName(additionalInterfaceVLANID),
				})
				err = f.Clients.GenericClient().Update(context.Background(), testVM)
				Expect(err).NotTo(HaveOccurred())
			})

			By("Verify new interface appears in the guest OS", func() {
				util.UntilConditionStatus(vmcondition.TypeNetworkReady.String(), "True", framework.LongTimeout, testVM)
				Eventually(getIfaceCount).
					WithTimeout(framework.LongTimeout).
					WithPolling(3 * time.Second).
					Should(Equal(initialIfaceCount+1), "new interface should appear after hotplug")
			})

			By("Verify VM did not ask for restart after hotplug", expectNoRestartRequired)

			By("Hotunplug: remove the ClusterNetwork from spec.networks", func() {
				err := f.Clients.GenericClient().Get(context.Background(), crclient.ObjectKeyFromObject(testVM), testVM)
				Expect(err).NotTo(HaveOccurred())
				testVM.Spec.Networks = []v1alpha2.NetworksSpec{testVM.Spec.Networks[0]}
				err = f.Clients.GenericClient().Update(context.Background(), testVM)
				Expect(err).NotTo(HaveOccurred())
			})

			By("Verify interface disappears from the guest OS", func() {
				Eventually(getIfaceCount).
					WithTimeout(framework.LongTimeout).
					WithPolling(3 * time.Second).
					Should(Equal(initialIfaceCount), "interface should disappear after hotunplug")
			})

			By("Verify VM did not ask for restart after hotunplug", expectNoRestartRequired)

			By("Verify VM phase stayed Running throughout", func() {
				err := f.Clients.GenericClient().Get(context.Background(), crclient.ObjectKeyFromObject(testVM), testVM)
				Expect(err).NotTo(HaveOccurred())
				Expect(string(testVM.Status.Phase)).To(Equal(string(v1alpha2.MachineRunning)))
			})
		})
	})
})

// buildVMWithNetworks creates a VM with optional Main + ClusterNetwork.
// If hasMain is false, only ClusterNetwork is added (VM without Main network).
// The additional network interface is eth1 when hasMain is true, eth0 otherwise.
func buildVMWithNetworks(name, ns, vdRootName, additionalIP string, hasMain bool, extraOpts ...vm.Option) *v1alpha2.VirtualMachine {
	opts := []vm.Option{
		vm.WithName(name),
		vm.WithNamespace(ns),
		vm.WithBootloader(v1alpha2.EFI),
		vm.WithCPU(1, ptr.To("5%")),
		vm.WithMemory(resource.MustParse("256Mi")),
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
			Name: util.ClusterNetworkName(additionalInterfaceVLANID),
		}),
	)
	opts = append(opts, extraOpts...)
	return vm.New(opts...)
}

// cloudInitAdditionalNetwork returns cloud-init that configures the additional network interface with the given static IP.
// When hasMain is true, the additional interface is eth1; when false, it's eth0.
func cloudInitAdditionalNetwork(additionalIP string, hasMain bool) string {
	ifaceName := "eth0"
	if hasMain {
		ifaceName = "eth1"
	}
	return fmt.Sprintf(`#cloud-config
ssh_pwauth: True
users:
  - name: cloud
    passwd: $6$rounds=4096$vln/.aPHBOI7BMYR$bBMkqQvuGs5Gyd/1H5DP4m9HjQSy.kgrxpaGEHwkX7KEFV8BS.HZWPitAtZ2Vd8ZqIZRqmlykRCagTgPejt1i.
    shell: /bin/bash
    sudo: ALL=(ALL) NOPASSWD:ALL
    chpasswd: { expire: False }
    lock_passwd: False
    ssh_authorized_keys:
      - ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIFxcXHmwaGnJ8scJaEN5RzklBPZpVSic4GdaAsKjQoeA your_email@example.com
write_files:
  - path: /etc/network/interfaces
    append: true
    content: |
      auto %s
      iface %s inet static
          address %s
          netmask 255.255.255.0
runcmd:
  - "rc-update add sshd && rc-service sshd start"
  - "rc-update add networking boot && rc-service networking restart"
`, ifaceName, ifaceName, additionalIP)
}

func checkConnectivityBetweenVMs(f *framework.Framework, vmFoo, vmBar *v1alpha2.VirtualMachine, vmBarHasMainNetwork bool, vmBarAdditionalIP, vmFooAdditionalIP string) {
	GinkgoHelper()

	pingCmd := "ping -c 2 -W 2 -w 5 -q %s 2>&1 | grep -o \"[0-9]\\+%%\\s*packet loss\"" // %% -> % in output
	expectedOut := "0% packet loss"

	By(fmt.Sprintf("VM %s should have connectivity to %s (vm-bar)", vmFoo.Name, vmBarAdditionalIP))
	checkResultSSHCommand(f, vmFoo.Name, vmFoo.Namespace, fmt.Sprintf(pingCmd, vmBarAdditionalIP), expectedOut)

	if vmBarHasMainNetwork {
		By(fmt.Sprintf("VM %s should have connectivity to %s (vm-foo)", vmBar.Name, vmFooAdditionalIP))
		checkResultSSHCommand(f, vmBar.Name, vmBar.Namespace, fmt.Sprintf(pingCmd, vmFooAdditionalIP), expectedOut)
	}
}

const (
	Interval = 1 * time.Second
	Timeout  = 90 * time.Second
)

func checkResultSSHCommand(f *framework.Framework, vmName, vmNamespace, cmd, equal string) {
	GinkgoHelper()
	Eventually(func() (string, error) {
		res, err := f.SSHCommand(vmName, vmNamespace, cmd, framework.WithSSHTimeout(5*time.Second))
		if err != nil {
			return "", fmt.Errorf("cmd: %s\nstderr: %w", cmd, err)
		}
		return strings.TrimSpace(res), nil
	}).WithTimeout(Timeout).WithPolling(Interval).Should(Equal(equal))
}
