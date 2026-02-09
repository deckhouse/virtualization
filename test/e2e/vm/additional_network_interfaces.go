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
	"fmt"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/utils/ptr"
	crclient "sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/deckhouse/virtualization-controller/pkg/builder/vd"
	"github.com/deckhouse/virtualization-controller/pkg/builder/vm"
	"github.com/deckhouse/virtualization-controller/pkg/builder/vmop"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/vmcondition"
	"github.com/deckhouse/virtualization/test/e2e/internal/framework"
	"github.com/deckhouse/virtualization/test/e2e/internal/network"
	"github.com/deckhouse/virtualization/test/e2e/internal/object"
	"github.com/deckhouse/virtualization/test/e2e/internal/util"
)

const (
	// IPs on additional interface (eth1) for connectivity check between VMs.
	vmFooAdditionalIP = "192.168.1.10"
	vmBarAdditionalIP = "192.168.1.11"
)

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

		Expect(util.IsClusterNetworkExists(f)).To(BeTrue(),
			fmt.Sprintf("Cluster network %s does not exist. Create it first: %s", util.ClusterNetworkName, util.ClusterNetworkCreateCommand))
	})

	It("verifies additional network interfaces and connectivity before and after migration", func() {
		By("Environment preparation", func() {
			ns := f.Namespace().Name

			vdFooRoot = vd.New(
				vd.WithName("vd-foo-root"),
				vd.WithNamespace(ns),
				vd.WithSize(ptr.To(resource.MustParse("512Mi"))),
				vd.WithDataSourceHTTP(&v1alpha2.DataSourceHTTP{
					URL: object.ImageURLAlpineUEFIPerf,
				}),
			)
			vdBarRoot = vd.New(
				vd.WithName("vd-bar-root"),
				vd.WithNamespace(ns),
				vd.WithSize(ptr.To(resource.MustParse("512Mi"))),
				vd.WithDataSourceHTTP(&v1alpha2.DataSourceHTTP{
					URL: object.ImageURLAlpineUEFIPerf,
				}),
			)

			vmFoo = vm.New(
				vm.WithName("vm-foo"),
				vm.WithNamespace(ns),
				vm.WithBootloader(v1alpha2.EFI),
				vm.WithCPU(1, ptr.To("5%")),
				vm.WithMemory(resource.MustParse("256Mi")),
				vm.WithRestartApprovalMode(v1alpha2.Manual),
				vm.WithVirtualMachineClass(object.DefaultVMClass),
				vm.WithLiveMigrationPolicy(v1alpha2.PreferSafeMigrationPolicy),
				vm.WithProvisioningUserData(cloudInitAdditionalNetwork(vmFooAdditionalIP)),
				vm.WithBlockDeviceRefs(v1alpha2.BlockDeviceSpecRef{
					Kind: v1alpha2.VirtualDiskKind,
					Name: vdFooRoot.Name,
				}),
				vm.WithNetwork(v1alpha2.NetworksSpec{Type: v1alpha2.NetworksTypeMain}),
				vm.WithNetwork(v1alpha2.NetworksSpec{
					Type: v1alpha2.NetworksTypeClusterNetwork,
					Name: util.ClusterNetworkName,
				}),
			)
			vmBar = vm.New(
				vm.WithName("vm-bar"),
				vm.WithNamespace(ns),
				vm.WithBootloader(v1alpha2.EFI),
				vm.WithCPU(1, ptr.To("5%")),
				vm.WithMemory(resource.MustParse("256Mi")),
				vm.WithRestartApprovalMode(v1alpha2.Manual),
				vm.WithVirtualMachineClass(object.DefaultVMClass),
				vm.WithLiveMigrationPolicy(v1alpha2.PreferSafeMigrationPolicy),
				vm.WithProvisioningUserData(cloudInitAdditionalNetwork(vmBarAdditionalIP)),
				vm.WithBlockDeviceRefs(v1alpha2.BlockDeviceSpecRef{
					Kind: v1alpha2.VirtualDiskKind,
					Name: vdBarRoot.Name,
				}),
				vm.WithNetwork(v1alpha2.NetworksSpec{Type: v1alpha2.NetworksTypeMain}),
				vm.WithNetwork(v1alpha2.NetworksSpec{
					Type: v1alpha2.NetworksTypeClusterNetwork,
					Name: util.ClusterNetworkName,
				}),
			)

			err := f.CreateWithDeferredDeletion(context.Background(), vdFooRoot, vdBarRoot, vmFoo, vmBar)
			Expect(err).NotTo(HaveOccurred())

			util.UntilObjectPhase(string(v1alpha2.MachineRunning), framework.LongTimeout, vmFoo, vmBar)
			util.UntilVMAgentReady(crclient.ObjectKeyFromObject(vmFoo), framework.LongTimeout)
			util.UntilVMAgentReady(crclient.ObjectKeyFromObject(vmBar), framework.LongTimeout)
		})

		By("Wait for additional network interfaces to be ready", func() {
			util.UntilConditionStatus(vmcondition.TypeNetworkReady.String(), "True", framework.LongTimeout, vmFoo, vmBar)
		})

		By("Check connectivity between VMs via additional network", func() {
			checkConnectivityBetweenVMs(f, vmFoo, vmBar)
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
			err := network.CheckCiliumAgents(context.Background(), f.Clients.Kubectl(), vmFoo.Name, f.Namespace().Name)
			Expect(err).NotTo(HaveOccurred(), "Cilium agents check for VM %s", vmFoo.Name)
			err = network.CheckCiliumAgents(context.Background(), f.Clients.Kubectl(), vmBar.Name, f.Namespace().Name)
			Expect(err).NotTo(HaveOccurred(), "Cilium agents check for VM %s", vmBar.Name)
		})

		By("Check VM can reach external network after migration", func() {
			network.CheckExternalConnectivity(f, vmFoo.Name, network.ExternalHost, network.HTTPStatusOk)
			network.CheckExternalConnectivity(f, vmBar.Name, network.ExternalHost, network.HTTPStatusOk)
		})

		By("Wait for additional network interfaces to be ready after migration", func() {
			util.UntilConditionStatus(vmcondition.TypeNetworkReady.String(), "True", framework.LongTimeout, vmFoo, vmBar)
		})

		By("Check connectivity between VMs via additional network after migration", func() {
			checkConnectivityBetweenVMs(f, vmFoo, vmBar)
		})
	})
})

// cloudInitAdditionalNetwork returns cloud-init that configures eth1 with the given static IP (Alpine /etc/network/interfaces).
func cloudInitAdditionalNetwork(eth1Address string) string {
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
packages:
  - qemu-guest-agent
write_files:
  - path: /etc/network/interfaces
    append: true
    content: |

      auto eth1
      iface eth1 inet static
          address %s
          netmask 255.255.255.0
runcmd:
  - sudo rc-update add qemu-guest-agent default
  - sudo rc-service qemu-guest-agent start
  - sudo /etc/init.d/networking restart
  - chown -R cloud:cloud /home/cloud
`, eth1Address)
}

func checkConnectivityBetweenVMs(f *framework.Framework, vmFoo, vmBar *v1alpha2.VirtualMachine) {
	GinkgoHelper()

	pingCmd := "ping -c 2 -W 2 -w 5 -q %s 2>&1 | grep -o \"[0-9]\\+%%\\s*packet loss\"" // %% -> % in output
	expectedOut := "0% packet loss"

	By(fmt.Sprintf("VM %s should have connectivity to %s (vm-bar)", vmFoo.Name, vmBarAdditionalIP))
	checkResultSSHCommand(f, vmFoo.Name, vmFoo.Namespace, fmt.Sprintf(pingCmd, vmBarAdditionalIP), expectedOut)

	By(fmt.Sprintf("VM %s should have connectivity to %s (vm-foo)", vmBar.Name, vmFooAdditionalIP))
	checkResultSSHCommand(f, vmBar.Name, vmBar.Namespace, fmt.Sprintf(pingCmd, vmFooAdditionalIP), expectedOut)
}

const (
	Interval = 5 * time.Second
	Timeout  = 90 * time.Second
)

func checkResultSSHCommand(f *framework.Framework, vmName, vmNamespace, cmd, equal string) {
	GinkgoHelper()
	Eventually(func() (string, error) {
		res, err := f.SSHCommand(vmName, vmNamespace, cmd)
		if err != nil {
			return "", fmt.Errorf("cmd: %s\nstderr: %w", cmd, err)
		}
		return strings.TrimSpace(res), nil
	}).WithTimeout(Timeout).WithPolling(Interval).Should(Equal(equal))
}
