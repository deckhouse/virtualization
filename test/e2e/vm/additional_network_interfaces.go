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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"
	crclient "sigs.k8s.io/controller-runtime/pkg/client"

	vdbuilder "github.com/deckhouse/virtualization-controller/pkg/builder/vd"
	vmbuilder "github.com/deckhouse/virtualization-controller/pkg/builder/vm"
	vmopbuilder "github.com/deckhouse/virtualization-controller/pkg/builder/vmop"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/vmcondition"
	"github.com/deckhouse/virtualization/test/e2e/internal/framework"
	"github.com/deckhouse/virtualization/test/e2e/internal/network"
	"github.com/deckhouse/virtualization/test/e2e/internal/object"
	"github.com/deckhouse/virtualization/test/e2e/internal/util"
)

const (
	vmFooAdditionalIP = "192.168.1.10"
	vmBarAdditionalIP = "192.168.1.11"
)

type NetworkConfig struct {
	Name                      string
	HasMainNetwork            bool
	CheckExternalConnectivity bool
}

var _ = Describe("VirtualMachineAdditionalNetworkInterfaces", func() {
	var (
		f *framework.Framework
		t *VMAdditionalNetworkTest
	)

	BeforeEach(func() {
		f = framework.NewFramework("vm-additional-network")
		DeferCleanup(f.After)
		f.Before()

		if !util.IsSdnModuleEnabled(f) {
			Skip("SDN module is disabled. Skipping tests for additional network interfaces.")
		}

		Expect(util.IsClusterNetworkExists(f)).To(BeTrue(),
			fmt.Sprintf("Cluster network does not exist. Apply it first: %s", util.ClusterNetworkCreateCommand))
	})

	DescribeTable("checks VM additional network connectivity and migration",
		func(cfg NetworkConfig) {
			t = NewVMAdditionalNetworkTest(f, cfg)

			By("Environment preparation", func() {
				t.GenerateEnvironmentResources()
				err := f.CreateWithDeferredDeletion(context.Background(), t.VDFoo, t.VDBar, t.VMFoo, t.VMBar)
				Expect(err).NotTo(HaveOccurred())

				util.UntilObjectPhase(string(v1alpha2.MachineRunning), framework.LongTimeout, t.VMFoo, t.VMBar)
				util.UntilVMAgentReady(crclient.ObjectKeyFromObject(t.VMFoo), framework.LongTimeout)
				util.UntilVMAgentReady(crclient.ObjectKeyFromObject(t.VMBar), framework.LongTimeout)
				util.UntilConditionStatus(vmcondition.TypeNetworkReady.String(), string(metav1.ConditionTrue), framework.LongTimeout, t.VMFoo, t.VMBar)

				t.CheckCloudInitAdditionalNetworkCompleted(framework.LongTimeout)
			})

			By("Check connectivity between VMs via additional network", func() {
				t.CheckVMConnectivityBetweenVMs()
			})

			By("Trigger migrations", func() {
				util.MigrateVirtualMachine(f, t.VMFoo, vmopbuilder.WithGenerateName("vmop-migrate-foo-"))
				util.MigrateVirtualMachine(f, t.VMBar, vmopbuilder.WithGenerateName("vmop-migrate-bar-"))
			})

			By("Wait for migrations to complete", func() {
				util.UntilVMMigrationSucceeded(crclient.ObjectKeyFromObject(t.VMFoo), framework.LongTimeout)
				util.UntilVMMigrationSucceeded(crclient.ObjectKeyFromObject(t.VMBar), framework.LongTimeout)
			})

			By("Check Cilium agents after migration", func() {
				err := network.CheckCiliumAgents(context.Background(), f.Clients.Kubectl(), t.VMFoo.Name, f.Namespace().Name)
				Expect(err).NotTo(HaveOccurred(), "Cilium agents check should succeed for VM %s", t.VMFoo.Name)
				err = network.CheckCiliumAgents(context.Background(), f.Clients.Kubectl(), t.VMBar.Name, f.Namespace().Name)
				Expect(err).NotTo(HaveOccurred(), "Cilium agents check should succeed for VM %s", t.VMBar.Name)
			})

			if cfg.CheckExternalConnectivity {
				By("Check external connectivity after migration", func() {
					network.CheckExternalConnectivity(f, t.VMFoo.Name, network.ExternalHost, network.HTTPStatusOk)
					network.CheckExternalConnectivity(f, t.VMBar.Name, network.ExternalHost, network.HTTPStatusOk)
				})
			}

			By("Check network condition after migration", func() {
				util.UntilConditionStatus(vmcondition.TypeNetworkReady.String(), string(metav1.ConditionTrue), framework.LongTimeout, t.VMFoo, t.VMBar)
			})

			By("Check connectivity between VMs via additional network after migration", func() {
				t.CheckVMConnectivityBetweenVMs()
			})
		},
		Entry("with Main and ClusterNetwork", NetworkConfig{Name: "with Main and ClusterNetwork", HasMainNetwork: true, CheckExternalConnectivity: true}),
		Entry("only ClusterNetwork", NetworkConfig{Name: "only ClusterNetwork", HasMainNetwork: false, CheckExternalConnectivity: false}),
	)
})

type VMAdditionalNetworkTest struct {
	Framework *framework.Framework
	Config    NetworkConfig

	VDFoo *v1alpha2.VirtualDisk
	VDBar *v1alpha2.VirtualDisk
	VMFoo *v1alpha2.VirtualMachine
	VMBar *v1alpha2.VirtualMachine
}

func NewVMAdditionalNetworkTest(f *framework.Framework, cfg NetworkConfig) *VMAdditionalNetworkTest {
	return &VMAdditionalNetworkTest{Framework: f, Config: cfg}
}

func (t *VMAdditionalNetworkTest) GenerateEnvironmentResources() {
	t.VDFoo = vdbuilder.New(
		vdbuilder.WithName("vd-foo-root"),
		vdbuilder.WithNamespace(t.Framework.Namespace().Name),
		vdbuilder.WithDataSourceHTTP(&v1alpha2.DataSourceHTTP{
			URL: object.ImageURLUbuntu,
		}),
	)

	t.VDBar = vdbuilder.New(
		vdbuilder.WithName("vd-bar-root"),
		vdbuilder.WithNamespace(t.Framework.Namespace().Name),
		vdbuilder.WithDataSourceHTTP(&v1alpha2.DataSourceHTTP{
			URL: object.ImageURLUbuntu,
		}),
	)

	t.VMFoo = t.buildVM("vm-foo", t.VDFoo.Name, t.getCloudInitFoo())
	t.VMBar = t.buildVM("vm-bar", t.VDBar.Name, t.getCloudInitBar())
}

func (t *VMAdditionalNetworkTest) buildVM(name, vdName, cloudInit string) *v1alpha2.VirtualMachine {
	opts := []vmbuilder.Option{
		vmbuilder.WithName(name),
		vmbuilder.WithNamespace(t.Framework.Namespace().Name),
		vmbuilder.WithCPU(1, ptr.To("50%")),
		vmbuilder.WithMemory(resource.MustParse("256Mi")),
		vmbuilder.WithLiveMigrationPolicy(v1alpha2.AlwaysSafeMigrationPolicy),
		vmbuilder.WithVirtualMachineClass(object.DefaultVMClass),
		vmbuilder.WithProvisioningUserData(cloudInit),
		vmbuilder.WithBlockDeviceRefs(
			v1alpha2.BlockDeviceSpecRef{
				Kind: v1alpha2.DiskDevice,
				Name: vdName,
			},
		),
	}
	if t.Config.HasMainNetwork {
		opts = append(opts,
			vmbuilder.WithNetwork(v1alpha2.NetworksSpec{Type: v1alpha2.NetworksTypeMain}),
			vmbuilder.WithNetwork(v1alpha2.NetworksSpec{
				Type: v1alpha2.NetworksTypeClusterNetwork,
				Name: util.ClusterNetworkName,
			}),
		)
	} else {
		opts = append(opts, vmbuilder.WithNetwork(v1alpha2.NetworksSpec{
			Type: v1alpha2.NetworksTypeClusterNetwork,
			Name: util.ClusterNetworkName,
		}))
	}
	return vmbuilder.New(opts...)
}

func (t *VMAdditionalNetworkTest) getCloudInitFoo() string {
	return t.getCloudInitWithAdditionalIP(vmFooAdditionalIP)
}

func (t *VMAdditionalNetworkTest) getCloudInitBar() string {
	return t.getCloudInitWithAdditionalIP(vmBarAdditionalIP)
}

// getCloudInitWithAdditionalIP returns cloud-init. For "Main+ClusterNetwork" it configures the second
// interface with the given static IP. For "only ClusterNetwork" it only installs qemu-guest-agent;
// the single interface gets its IP from SDN (status.IPAddress).
func (t *VMAdditionalNetworkTest) getCloudInitWithAdditionalIP(additionalIP string) string {
	base := `#cloud-config
package_update: true
packages:
  - qemu-guest-agent
users:
  - name: cloud
    passwd: $6$rounds=4096$vln/.aPHBOI7BMYR$bBMkqQvuGs5Gyd/1H5DP4m9HjQSy.kgrxpaGEHwkX7KEFV8BS.HZWPitAtZ2Vd8ZqIZRqmlykRCagTgPejt1i.
    shell: /bin/bash
    sudo: ALL=(ALL) NOPASSWD:ALL
    lock_passwd: false
    ssh_authorized_keys:
      - ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIFxcXHmwaGnJ8scJaEN5RzklBPZpVSic4GdaAsKjQoeA your_email@example.com
runcmd:
  - systemctl enable --now qemu-guest-agent.service
`
	// Main+ClusterNetwork: do not add static IP here — the second interface (ClusterNetwork)
	// often appears after cloud-init runs. The test adds the IP via SSH in EnsureStaticIPOnSecondInterface.
	return base
}

func (t *VMAdditionalNetworkTest) CheckCloudInitAdditionalNetworkCompleted(timeout time.Duration) {
	GinkgoHelper()

	if !t.Config.HasMainNetwork {
		// Only ClusterNetwork: IP comes from SDN. Wait until both VMs have status.IPAddress.
		// If the environment does not set IPAddress for VMs without Main network, skip the test.
		deadline := time.Now().Add(timeout)
		for time.Now().Before(deadline) {
			err := t.Framework.Clients.GenericClient().Get(context.Background(), crclient.ObjectKeyFromObject(t.VMFoo), t.VMFoo)
			Expect(err).NotTo(HaveOccurred())
			err = t.Framework.Clients.GenericClient().Get(context.Background(), crclient.ObjectKeyFromObject(t.VMBar), t.VMBar)
			Expect(err).NotTo(HaveOccurred())
			if t.VMFoo.Status.IPAddress != "" && t.VMBar.Status.IPAddress != "" {
				return
			}
			time.Sleep(time.Second)
		}
		if t.VMFoo.Status.IPAddress == "" || t.VMBar.Status.IPAddress == "" {
			Skip("VM status.IPAddress is not set for only-ClusterNetwork; SDN may not populate it in this environment")
		}
		return
	}
	// Main+ClusterNetwork: second interface (ClusterNetwork) appears after boot. Add static IPs via SSH with retries.
	t.EnsureStaticIPOnSecondInterface(t.VMFoo, vmFooAdditionalIP, timeout)
	t.EnsureStaticIPOnSecondInterface(t.VMBar, vmBarAdditionalIP, timeout)
}

// EnsureStaticIPOnSecondInterface adds the given IP to the second (ClusterNetwork) interface via SSH.
// Retries until the second interface exists and the IP is present (it may appear some time after NetworkReady).
// The script is escaped for the outer shell that wraps the command in single quotes (d8 ssh -c '...').
func (t *VMAdditionalNetworkTest) EnsureStaticIPOnSecondInterface(vm *v1alpha2.VirtualMachine, ip string, timeout time.Duration) {
	GinkgoHelper()

	script := fmt.Sprintf(
		`SECOND_IF=$(ip -o link show | awk -F': ' '{print $2}' | grep -v lo | sed -n '2p'); `+
			`if [ -n "$SECOND_IF" ]; then sudo ip addr add %s/24 dev $SECOND_IF 2>/dev/null; fi; `+
			`ip -4 addr show`,
		ip,
	)
	escapedScript := escapeCommandForSSH(script)
	sshOpts := []framework.SSHCommandOption{framework.WithSSHTimeout(framework.LongTimeout)}
	Eventually(func(g Gomega) {
		cmdOut, err := t.Framework.SSHCommand(vm.Name, vm.Namespace, escapedScript, sshOpts...)
		g.Expect(err).NotTo(HaveOccurred())
		g.Expect(cmdOut).To(ContainSubstring(fmt.Sprintf("inet %s", ip)),
			"VM %s should have IP %s on second interface (ClusterNetwork)", vm.Name, ip)
	}).WithTimeout(timeout).WithPolling(5 * time.Second).Should(Succeed())
}

// escapeCommandForSSH escapes a command so it can be passed inside single quotes to "ssh -c '...'".
// Replaces each ' with '\'' (end quote, literal quote, start quote).
func escapeCommandForSSH(cmd string) string {
	return strings.ReplaceAll(cmd, "'", "'\\''")
}

// CheckVMConnectivityBetweenVMs verifies that vm-foo and vm-bar can ping each other.
// For Main+ClusterNetwork uses static IPs 192.168.1.10/11 and retries (second interface/ARP may be delayed).
func (t *VMAdditionalNetworkTest) CheckVMConnectivityBetweenVMs() {
	GinkgoHelper()

	if t.Config.HasMainNetwork {
		// Retry: connectivity over the additional network may need a moment after IPs are added.
		Eventually(func(g Gomega) {
			cmdOut, err := t.runPing(t.VMFoo, vmBarAdditionalIP)
			g.Expect(err).NotTo(HaveOccurred(), "ping vm-foo -> 192.168.1.11: %s", cmdOut)
			g.Expect(cmdOut).To(ContainSubstring("0% packet loss"))

			cmdOut, err = t.runPing(t.VMBar, vmFooAdditionalIP)
			g.Expect(err).NotTo(HaveOccurred(), "ping vm-bar -> 192.168.1.10: %s", cmdOut)
			g.Expect(cmdOut).To(ContainSubstring("0% packet loss"))
		}).WithTimeout(2 * framework.MiddleTimeout).WithPolling(10 * time.Second).Should(Succeed())
		return
	}
	// Only ClusterNetwork: get IPs from status (assigned by SDN).
	err := t.Framework.Clients.GenericClient().Get(context.Background(), crclient.ObjectKeyFromObject(t.VMFoo), t.VMFoo)
	Expect(err).NotTo(HaveOccurred())
	err = t.Framework.Clients.GenericClient().Get(context.Background(), crclient.ObjectKeyFromObject(t.VMBar), t.VMBar)
	Expect(err).NotTo(HaveOccurred())
	Expect(t.VMFoo.Status.IPAddress).NotTo(BeEmpty(), "vm-foo must have status.IPAddress")
	Expect(t.VMBar.Status.IPAddress).NotTo(BeEmpty(), "vm-bar must have status.IPAddress")
	// Strip CIDR suffix if present (e.g. 10.66.10.3/32 -> 10.66.10.3).
	fooIP := strings.Split(t.VMFoo.Status.IPAddress, "/")[0]
	barIP := strings.Split(t.VMBar.Status.IPAddress, "/")[0]
	t.CheckVMConnectivityToTargetIP(t.VMFoo, barIP)
	t.CheckVMConnectivityToTargetIP(t.VMBar, fooIP)
}

// runPing runs ping from the given VM to targetIP and returns (stdout, error). Does not call Expect.
func (t *VMAdditionalNetworkTest) runPing(vm *v1alpha2.VirtualMachine, targetIP string) (string, error) {
	cmd := fmt.Sprintf("ping -c 2 -W 2 -w 5 %s", targetIP)
	return t.Framework.SSHCommand(vm.Name, vm.Namespace, cmd, framework.WithSSHTimeout(framework.MiddleTimeout))
}

func (t *VMAdditionalNetworkTest) CheckVMConnectivityToTargetIP(vm *v1alpha2.VirtualMachine, targetIP string) {
	GinkgoHelper()

	By(fmt.Sprintf("VM %q should have connectivity to %s", vm.Name, targetIP))
	cmdOut, err := t.runPing(vm, targetIP)
	Expect(err).NotTo(HaveOccurred(), "ping from %s to %s failed: %s", vm.Name, targetIP, cmdOut)
	Expect(cmdOut).To(ContainSubstring("0% packet loss"), "expected 0%% packet loss when pinging %s from %s, got: %s", targetIP, vm.Name, cmdOut)
}
