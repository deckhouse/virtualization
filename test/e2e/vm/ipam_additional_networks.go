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
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/ptr"
	crclient "sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/deckhouse/virtualization-controller/pkg/builder/vd"
	"github.com/deckhouse/virtualization-controller/pkg/builder/vm"
	"github.com/deckhouse/virtualization-controller/pkg/builder/vmop"
	"github.com/deckhouse/virtualization-controller/pkg/common/annotations"
	"github.com/deckhouse/virtualization-controller/pkg/controller/conditions"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/vmcondition"
	"github.com/deckhouse/virtualization/test/e2e/internal/framework"
	"github.com/deckhouse/virtualization/test/e2e/internal/object"
	"github.com/deckhouse/virtualization/test/e2e/internal/precheck"
	"github.com/deckhouse/virtualization/test/e2e/internal/util"
)

const (
	ipamNetworkName   = "cn-4006-for-e2e-test"
	noPoolNetworkName = "cn-4007-for-e2e-test"
)

var _ = Describe("VirtualMachineIPAMForAdditionalNetworks", Label(precheck.PrecheckSDN), func() {
	var (
		ctx context.Context
		f   *framework.Framework
	)

	BeforeEach(func() {
		ctx = context.Background()
		f = framework.NewFramework("vm-ipam")
		DeferCleanup(f.After)
		f.Before()
	})

	Describe("auto mode (DHCP)", func() {
		var (
			vdRoot *v1alpha2.VirtualDisk
			testVM *v1alpha2.VirtualMachine
		)

		It("should allocate IP from pool, deliver via DHCP, and keep stable across restart", func() {
			By("Create VM with Main + cn-4006 (auto, no ipAddressName)", func() {
				ns := f.Namespace().Name
				vdRoot = object.NewVDFromCVI("vd-root", ns, object.PrecreatedCVIAlpineUEFIPerf,
					vd.WithSize(ptr.To(resource.MustParse("512Mi"))),
				)
				testVM = buildIPAMVM("vm-auto", ns, vdRoot.Name, "")

				Expect(f.CreateWithDeferredDeletion(ctx, vdRoot, testVM)).To(Succeed())
			})

			By("Wait for VM Running and NetworkReady=True", func() {
				util.UntilObjectPhase(ctx, string(v1alpha2.MachineRunning), framework.LongTimeout, testVM)
				util.UntilConditionStatus(ctx, vmcondition.TypeNetworkReady.String(), "True", framework.LongTimeout, testVM)
			})

			var allocatedIP string
			By("Verify status.networks has ipAddress for cn-4006", func() {
				updated := refreshVM(ctx, f, testVM)
				allocatedIP = getVMNetworkIPAddress(updated)
				Expect(allocatedIP).NotTo(BeEmpty(), "auto IPAddress should be allocated in status.networks")
			})

			By("Verify IP is present in guest OS via DHCP", func() {
				util.UntilSSHReady(f, testVM, framework.LongTimeout)
				Eventually(func(g Gomega) {
					output, err := f.SSHCommand(testVM.Name, testVM.Namespace, "ip -br -4 addr show eth1")
					g.Expect(err).NotTo(HaveOccurred())
					g.Expect(output).To(ContainSubstring(allocatedIP), "eth1 should have the allocated IP %s", allocatedIP)
				}).WithTimeout(framework.LongTimeout).WithPolling(3 * time.Second).Should(Succeed())
			})

			By("Restart VM and verify the same IP is kept (stability)", func() {
				previousRunningTime := time.Now()
				util.RebootVirtualMachineByVMOP(f, testVM)
				util.UntilVirtualMachineRebooted(crclient.ObjectKeyFromObject(testVM), previousRunningTime, framework.LongTimeout)
				util.UntilObjectPhase(ctx, string(v1alpha2.MachineRunning), framework.LongTimeout, testVM)
				util.UntilConditionStatus(ctx, vmcondition.TypeNetworkReady.String(), "True", framework.LongTimeout, testVM)

				updated := refreshVM(ctx, f, testVM)
				Expect(getVMNetworkIPAddress(updated)).To(Equal(allocatedIP),
					"auto IP should be stable across restart (ownerRef on VM)")
			})
		})
	})

	Describe("static mode (ipAddressName)", func() {
		var (
			vdRoot *v1alpha2.VirtualDisk
			testVM *v1alpha2.VirtualMachine
		)

		It("should use user-provided IPAddress and keep stable across restart", func() {
			By("Create IPAddress (Static) and VM referencing it", func() {
				ns := f.Namespace().Name
				Expect(util.CreateSDNIPAddress(ctx, f, "my-static-ip", ns,
					v1alpha2.NetworksTypeClusterNetwork, ipamNetworkName, "192.168.200.50")).To(Succeed())

				// Wait for the IPAddress to be allocated.
				Eventually(func(g Gomega) {
					addr, err := util.GetSDNIPAddressAddress(ctx, f, "my-static-ip", ns)
					g.Expect(err).NotTo(HaveOccurred())
					g.Expect(addr).To(Equal("192.168.200.50"))
				}).WithTimeout(framework.LongTimeout).WithPolling(3 * time.Second).Should(Succeed())

				vdRoot = object.NewVDFromCVI("vd-root", ns, object.PrecreatedCVIAlpineUEFIPerf,
					vd.WithSize(ptr.To(resource.MustParse("512Mi"))),
				)
				testVM = buildIPAMVM("vm-static", ns, vdRoot.Name, "my-static-ip")

				Expect(f.CreateWithDeferredDeletion(ctx, vdRoot, testVM)).To(Succeed())
			})

			By("Wait for VM Running and NetworkReady=True", func() {
				util.UntilObjectPhase(ctx, string(v1alpha2.MachineRunning), framework.LongTimeout, testVM)
				util.UntilConditionStatus(ctx, vmcondition.TypeNetworkReady.String(), "True", framework.LongTimeout, testVM)
			})

			By("Verify status.networks has the static IP", func() {
				updated := refreshVM(ctx, f, testVM)
				Expect(getVMNetworkIPAddress(updated)).To(Equal("192.168.200.50"))
			})

			By("Verify IP is present in guest OS", func() {
				util.UntilSSHReady(f, testVM, framework.LongTimeout)
				Eventually(func(g Gomega) {
					output, err := f.SSHCommand(testVM.Name, testVM.Namespace, "ip -br -4 addr show eth1")
					g.Expect(err).NotTo(HaveOccurred())
					g.Expect(output).To(ContainSubstring("192.168.200.50"))
				}).WithTimeout(framework.LongTimeout).WithPolling(3 * time.Second).Should(Succeed())
			})

			By("Restart VM and verify the same static IP is kept", func() {
				previousRunningTime := time.Now()
				util.RebootVirtualMachineByVMOP(f, testVM)
				util.UntilVirtualMachineRebooted(crclient.ObjectKeyFromObject(testVM), previousRunningTime, framework.LongTimeout)
				util.UntilObjectPhase(ctx, string(v1alpha2.MachineRunning), framework.LongTimeout, testVM)
				util.UntilConditionStatus(ctx, vmcondition.TypeNetworkReady.String(), "True", framework.LongTimeout, testVM)

				updated := refreshVM(ctx, f, testVM)
				Expect(getVMNetworkIPAddress(updated)).To(Equal("192.168.200.50"))
			})
		})
	})

	Describe("skip problematic interface", func() {
		var (
			vdRoot *v1alpha2.VirtualDisk
			testVM *v1alpha2.VirtualMachine
		)

		It("should start VM without problematic network and report error in NetworkReady", func() {
			By("Create VM with cn-4006 (auto) + cn-4007 (no pool, ipAddressName=nonexistent)", func() {
				ns := f.Namespace().Name
				vdRoot = object.NewVDFromCVI("vd-root", ns, object.PrecreatedCVIAlpineUEFIPerf,
					vd.WithSize(ptr.To(resource.MustParse("512Mi"))),
				)
				testVM = buildIPAMVM("vm-skip", ns, vdRoot.Name, "",
					vm.WithNetwork(v1alpha2.NetworksSpec{
						Type:          v1alpha2.NetworksTypeClusterNetwork,
						Name:          noPoolNetworkName,
						IPAddressName: "nonexistent-ip",
					}),
				)

				Expect(f.CreateWithDeferredDeletion(ctx, vdRoot, testVM)).To(Succeed())
			})

			By("Wait for VM Running (not blocked by problematic network)", func() {
				util.UntilObjectPhase(ctx, string(v1alpha2.MachineRunning), framework.LongTimeout, testVM)
			})

			By("Verify NetworkReady=False with error about cn-4007", func() {
				Eventually(func(g Gomega) {
					updated := refreshVM(ctx, f, testVM)
					cond, exists := conditions.GetCondition(vmcondition.TypeNetworkReady, updated.Status.Conditions)
					g.Expect(exists).To(BeTrue())
					g.Expect(cond.Status).To(Equal(metav1.ConditionFalse))
					g.Expect(cond.Reason).To(Equal(vmcondition.ReasonNetworkNotReady.String()))
					g.Expect(cond.Message).To(ContainSubstring(noPoolNetworkName))
				}).WithTimeout(framework.LongTimeout).WithPolling(3 * time.Second).Should(Succeed())
			})

			By("Verify cn-4006 has IP in status (works) and cn-4007 is skipped in networks-spec", func() {
				updated := refreshVM(ctx, f, testVM)
				Expect(getVMNetworkIPAddress(updated)).NotTo(BeEmpty(),
					"cn-4006 should have an allocated IP despite cn-4007 being problematic")

				spec := getPodNetworksSpec(ctx, f, testVM.Name, testVM.Namespace)
				Expect(networkInPodSpec(spec, noPoolNetworkName)).To(BeFalse(),
					"cn-4007 should be skipped from networks-spec")
				Expect(networkInPodSpec(spec, ipamNetworkName)).To(BeTrue(),
					"cn-4006 should be present in networks-spec")
			})
		})
	})

	Describe("skip to working via watcher", func() {
		var (
			vdRoot *v1alpha2.VirtualDisk
			testVM *v1alpha2.VirtualMachine
		)

		It("should provision interface when IPAddress is created after VM start (watcher)", func() {
			By("Create VM with cn-4006 + ipAddressName=not-yet-created (static, IPAddress absent)", func() {
				ns := f.Namespace().Name
				vdRoot = object.NewVDFromCVI("vd-root", ns, object.PrecreatedCVIAlpineUEFIPerf,
					vd.WithSize(ptr.To(resource.MustParse("512Mi"))),
				)
				testVM = buildIPAMVM("vm-watcher", ns, vdRoot.Name, "watcher-ip")

				Expect(f.CreateWithDeferredDeletion(ctx, vdRoot, testVM)).To(Succeed())
			})

			By("Wait for VM Running and NetworkReady=False (IPAddress does not exist)", func() {
				util.UntilObjectPhase(ctx, string(v1alpha2.MachineRunning), framework.LongTimeout, testVM)
				Eventually(func(g Gomega) {
					updated := refreshVM(ctx, f, testVM)
					cond, _ := conditions.GetCondition(vmcondition.TypeNetworkReady, updated.Status.Conditions)
					g.Expect(cond.Status).To(Equal(metav1.ConditionFalse))
				}).WithTimeout(framework.LongTimeout).WithPolling(3 * time.Second).Should(Succeed())
			})

			By("Verify cn-4006 is skipped in networks-spec", func() {
				spec := getPodNetworksSpec(ctx, f, testVM.Name, testVM.Namespace)
				Expect(networkInPodSpec(spec, ipamNetworkName)).To(BeFalse(),
					"cn-4006 should be skipped while IPAddress does not exist")
			})

			By("Create the IPAddress (Static) — watcher should trigger reconciliation", func() {
				Expect(util.CreateSDNIPAddress(ctx, f, "watcher-ip", f.Namespace().Name,
					v1alpha2.NetworksTypeClusterNetwork, ipamNetworkName, "192.168.200.51")).To(Succeed())
			})

			By("Verify NetworkReady=True and cn-4006 is now provisioned (via watcher, no restart)", func() {
				podUIDBefore := getVirtLauncherPodUID(ctx, f, testVM.Name, testVM.Namespace)

				Eventually(func(g Gomega) {
					updated := refreshVM(ctx, f, testVM)
					cond, _ := conditions.GetCondition(vmcondition.TypeNetworkReady, updated.Status.Conditions)
					g.Expect(cond.Status).To(Equal(metav1.ConditionTrue))
					g.Expect(getVMNetworkIPAddress(updated)).To(Equal("192.168.200.51"))
				}).WithTimeout(framework.LongTimeout).WithPolling(3 * time.Second).Should(Succeed())

				By("Verify pod was not recreated (hotplug via watcher)")
				podUIDAfter := getVirtLauncherPodUID(ctx, f, testVM.Name, testVM.Namespace)
				Expect(podUIDAfter).To(Equal(podUIDBefore), "pod should not be recreated on IPAddress creation")

				Consistently(func(g Gomega) {
					v := refreshVM(ctx, f, testVM)
					cond, _ := conditions.GetCondition(vmcondition.TypeAwaitingRestartToApplyConfiguration, v.Status.Conditions)
					g.Expect(cond.Status).NotTo(Equal(metav1.ConditionTrue), "VM should not require restart for watcher-triggered hotplug")
				}).WithTimeout(10 * time.Second).WithPolling(2 * time.Second).Should(Succeed())
			})
		})
	})

	Describe("hotplug static to auto", func() {
		var (
			vdRoot *v1alpha2.VirtualDisk
			testVM *v1alpha2.VirtualMachine
		)

		It("should switch from static to auto without restart (hotplug)", func() {
			By("Create IPAddress (Static) and VM with static ipAddressName", func() {
				ns := f.Namespace().Name
				Expect(util.CreateSDNIPAddress(ctx, f, "hotplug-static", ns,
					v1alpha2.NetworksTypeClusterNetwork, ipamNetworkName, "192.168.200.52")).To(Succeed())

				// Wait for the IPAddress to be allocated.
				Eventually(func(g Gomega) {
					addr, err := util.GetSDNIPAddressAddress(ctx, f, "hotplug-static", ns)
					g.Expect(err).NotTo(HaveOccurred())
					g.Expect(addr).To(Equal("192.168.200.52"))
				}).WithTimeout(framework.LongTimeout).WithPolling(3 * time.Second).Should(Succeed())

				vdRoot = object.NewVDFromCVI("vd-root", ns, object.PrecreatedCVIAlpineUEFIPerf,
					vd.WithSize(ptr.To(resource.MustParse("512Mi"))),
				)
				testVM = buildIPAMVM("vm-hotplug-sa", ns, vdRoot.Name, "hotplug-static")

				Expect(f.CreateWithDeferredDeletion(ctx, vdRoot, testVM)).To(Succeed())
			})

			By("Wait for VM Running and NetworkReady=True", func() {
				util.UntilObjectPhase(ctx, string(v1alpha2.MachineRunning), framework.LongTimeout, testVM)
				util.UntilConditionStatus(ctx, vmcondition.TypeNetworkReady.String(), "True", framework.LongTimeout, testVM)
			})

			By("Verify static IP in status", func() {
				updated := refreshVM(ctx, f, testVM)
				Expect(getVMNetworkIPAddress(updated)).To(Equal("192.168.200.52"))
			})

			By("Remove ipAddressName (switch to auto)", func() {
				podUIDBefore := getVirtLauncherPodUID(ctx, f, testVM.Name, testVM.Namespace)

				updated := refreshVM(ctx, f, testVM)
				updated.Spec.Networks[1].IPAddressName = ""
				Expect(f.Clients.GenericClient().Update(ctx, updated)).To(Succeed())

				By("Verify NetworkReady=True with new auto IP and pod not recreated")
				Eventually(func(g Gomega) {
					v := refreshVM(ctx, f, testVM)
					cond, _ := conditions.GetCondition(vmcondition.TypeNetworkReady, v.Status.Conditions)
					g.Expect(cond.Status).To(Equal(metav1.ConditionTrue))
					ip := getVMNetworkIPAddress(v)
					g.Expect(ip).NotTo(BeEmpty())
					g.Expect(ip).NotTo(Equal("192.168.200.52"), "should be a new auto IP, not the old static")
				}).WithTimeout(framework.LongTimeout).WithPolling(3 * time.Second).Should(Succeed())

				podUIDAfter := getVirtLauncherPodUID(ctx, f, testVM.Name, testVM.Namespace)
				Expect(podUIDAfter).To(Equal(podUIDBefore), "pod should not be recreated on hotplug")

				cond, _ := conditions.GetCondition(vmcondition.TypeAwaitingRestartToApplyConfiguration, refreshVM(ctx, f, testVM).Status.Conditions)
				Expect(cond.Status).NotTo(Equal(metav1.ConditionTrue), "VM should not require restart for network change")
			})
		})
	})

	Describe("hotplug auto to static", func() {
		var (
			vdRoot *v1alpha2.VirtualDisk
			testVM *v1alpha2.VirtualMachine
		)

		It("should switch from auto to static without restart (hotplug)", func() {
			By("Create VM in auto mode", func() {
				ns := f.Namespace().Name
				vdRoot = object.NewVDFromCVI("vd-root", ns, object.PrecreatedCVIAlpineUEFIPerf,
					vd.WithSize(ptr.To(resource.MustParse("512Mi"))),
				)
				testVM = buildIPAMVM("vm-hotplug-as", ns, vdRoot.Name, "")

				Expect(f.CreateWithDeferredDeletion(ctx, vdRoot, testVM)).To(Succeed())
			})

			By("Wait for VM Running and NetworkReady=True", func() {
				util.UntilObjectPhase(ctx, string(v1alpha2.MachineRunning), framework.LongTimeout, testVM)
				util.UntilConditionStatus(ctx, vmcondition.TypeNetworkReady.String(), "True", framework.LongTimeout, testVM)
			})

			By("Create IPAddress (Static) and add ipAddressName (switch to static)", func() {
				podUIDBefore := getVirtLauncherPodUID(ctx, f, testVM.Name, testVM.Namespace)

				Expect(util.CreateSDNIPAddress(ctx, f, "hotplug-as-ip", f.Namespace().Name,
					v1alpha2.NetworksTypeClusterNetwork, ipamNetworkName, "192.168.200.99")).To(Succeed())

				patch := `[{"op":"add","path":"/spec/networks/1/ipAddressName","value":"hotplug-as-ip"}]`
				Expect(f.Clients.GenericClient().Patch(ctx, testVM, crclient.RawPatch(types.JSONPatchType, []byte(patch)))).To(Succeed())

				By("Verify NetworkReady=True with static IP and pod not recreated")
				Eventually(func(g Gomega) {
					v := refreshVM(ctx, f, testVM)
					cond, _ := conditions.GetCondition(vmcondition.TypeNetworkReady, v.Status.Conditions)
					g.Expect(cond.Status).To(Equal(metav1.ConditionTrue))
					g.Expect(getVMNetworkIPAddress(v)).To(Equal("192.168.200.99"))
				}).WithTimeout(framework.LongTimeout).WithPolling(3 * time.Second).Should(Succeed())

				podUIDAfter := getVirtLauncherPodUID(ctx, f, testVM.Name, testVM.Namespace)
				Expect(podUIDAfter).To(Equal(podUIDBefore), "pod should not be recreated on hotplug")

				Consistently(func(g Gomega) {
					v := refreshVM(ctx, f, testVM)
					cond, _ := conditions.GetCondition(vmcondition.TypeAwaitingRestartToApplyConfiguration, v.Status.Conditions)
					g.Expect(cond.Status).NotTo(Equal(metav1.ConditionTrue), "VM should not require restart for ipAddressName change")
				}).WithTimeout(10 * time.Second).WithPolling(2 * time.Second).Should(Succeed())
			})
		})
	})

	Describe("live migration (auto)", func() {
		var (
			vdRoot *v1alpha2.VirtualDisk
			testVM *v1alpha2.VirtualMachine
		)

		It("should preserve auto IP across live migration", func() {
			By("Create VM in auto mode", func() {
				ns := f.Namespace().Name
				vdRoot = object.NewVDFromCVI("vd-root", ns, object.PrecreatedCVIAlpineUEFIPerf,
					vd.WithSize(ptr.To(resource.MustParse("512Mi"))),
				)
				testVM = buildIPAMVM("vm-migrate", ns, vdRoot.Name, "")

				Expect(f.CreateWithDeferredDeletion(ctx, vdRoot, testVM)).To(Succeed())
			})

			By("Wait for VM Running and NetworkReady=True", func() {
				util.UntilObjectPhase(ctx, string(v1alpha2.MachineRunning), framework.LongTimeout, testVM)
				util.UntilConditionStatus(ctx, vmcondition.TypeNetworkReady.String(), "True", framework.LongTimeout, testVM)
			})

			var allocatedIP string
			By("Record the allocated IP", func() {
				updated := refreshVM(ctx, f, testVM)
				allocatedIP = getVMNetworkIPAddress(updated)
				Expect(allocatedIP).NotTo(BeEmpty())
			})

			By("Migrate VM", func() {
				util.MigrateVirtualMachine(f, testVM, vmop.WithGenerateName("vmop-migrate-ipam-"))
			})

			By("Wait for migration to complete", func() {
				util.UntilVMMigrationSucceeded(crclient.ObjectKeyFromObject(testVM), framework.LongTimeout)
			})

			By("Verify the same IP is preserved after migration", func() {
				util.UntilObjectPhase(ctx, string(v1alpha2.MachineRunning), framework.LongTimeout, testVM)
				util.UntilConditionStatus(ctx, vmcondition.TypeNetworkReady.String(), "True", framework.LongTimeout, testVM)

				updated := refreshVM(ctx, f, testVM)
				Expect(getVMNetworkIPAddress(updated)).To(Equal(allocatedIP),
					"auto IP should be preserved across live migration")
			})
		})
	})
})

// cloudInitDHCPAdditionalNetwork returns a cloud-init that enables DHCP on all
// interfaces (eth0 for Main, eth1 for additional). Used for IPAM tests where
// the guest OS must obtain the additional network IP via DHCP from SDN.
func cloudInitDHCPAdditionalNetwork(hasMain bool) string {
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
    lock_passwd: False
    ssh_authorized_keys:
      - ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIFxcXHmwaGnJ8scJaEN5RzklBPZpVSic4GdaAsKjQoeA your_email@example.com
packages:
  - bash
  - iputils-ping
write_files:
  - path: /etc/network/interfaces
    content: |
      auto lo
      iface lo inet loopback
      auto eth0
      iface eth0 inet dhcp
      auto %s
      iface %s inet dhcp
runcmd:
  - rc-update add sshd && rc-service sshd start
  - rc-update add networking boot && rc-service networking restart
`, ifaceName, ifaceName)
}

// getVMNetworkIPAddress returns the ipAddress from status.networks for the IPAM network (cn-4006).
func getVMNetworkIPAddress(vm *v1alpha2.VirtualMachine) string {
	for _, n := range vm.Status.Networks {
		if n.Name == ipamNetworkName {
			return n.IPAddress
		}
	}
	return ""
}

// getActiveVirtLauncherPod returns the active (Running) virt-launcher pod for the VM.
func getActiveVirtLauncherPod(ctx context.Context, f *framework.Framework, vmName, namespace string) *corev1.Pod {
	podList := &corev1.PodList{}
	Expect(f.Clients.GenericClient().List(ctx, podList,
		crclient.InNamespace(namespace),
		crclient.MatchingLabels{
			"kubevirt.internal.virtualization.deckhouse.io":         "virt-launcher",
			"vm.kubevirt.internal.virtualization.deckhouse.io/name": vmName,
		},
	)).To(Succeed())
	for i := range podList.Items {
		if podList.Items[i].Status.Phase == corev1.PodRunning {
			return &podList.Items[i]
		}
	}
	return nil
}

// getVirtLauncherPodUID returns the UID of the active virt-launcher pod for the VM.
func getVirtLauncherPodUID(ctx context.Context, f *framework.Framework, vmName, namespace string) string {
	pod := getActiveVirtLauncherPod(ctx, f, vmName, namespace)
	Expect(pod).NotTo(BeNil(), "active virt-launcher pod should exist")
	return string(pod.UID)
}

// getPodNetworksSpec returns the networks-spec annotation from the active virt-launcher pod.
func getPodNetworksSpec(ctx context.Context, f *framework.Framework, vmName, namespace string) string {
	pod := getActiveVirtLauncherPod(ctx, f, vmName, namespace)
	Expect(pod).NotTo(BeNil(), "active virt-launcher pod should exist")
	return pod.Annotations[annotations.AnnNetworksSpec]
}

// networkInPodSpec checks if the given network name appears in the networks-spec annotation.
func networkInPodSpec(spec, networkName string) bool {
	return strings.Contains(spec, networkName)
}

// refreshVM fetches the latest VM state from the cluster.
func refreshVM(ctx context.Context, f *framework.Framework, vm *v1alpha2.VirtualMachine) *v1alpha2.VirtualMachine {
	updated := &v1alpha2.VirtualMachine{}
	Expect(f.Clients.GenericClient().Get(ctx, crclient.ObjectKeyFromObject(vm), updated)).To(Succeed())
	return updated
}

// buildIPAMVM creates a VM with Main + ClusterNetwork (cn-4006) for IPAM tests.
// If ipAddressName is empty, auto mode is used; otherwise static mode.
func buildIPAMVM(name, ns, vdRootName, ipAddressName string, extraOpts ...vm.Option) *v1alpha2.VirtualMachine {
	opts := []vm.Option{
		vm.WithName(name),
		vm.WithNamespace(ns),
		vm.WithBootloader(v1alpha2.EFI),
		vm.WithCPU(1, ptr.To("50%")),
		vm.WithMemory(resource.MustParse("256Mi")),
		vm.WithRestartApprovalMode(v1alpha2.Manual),
		vm.WithVirtualMachineClass(object.DefaultVMClass),
		vm.WithLiveMigrationPolicy(v1alpha2.PreferSafeMigrationPolicy),
		vm.WithProvisioningUserData(cloudInitDHCPAdditionalNetwork(true)),
		vm.WithBlockDeviceRefs(v1alpha2.BlockDeviceSpecRef{
			Kind: v1alpha2.VirtualDiskKind,
			Name: vdRootName,
		}),
		vm.WithNetwork(v1alpha2.NetworksSpec{Type: v1alpha2.NetworksTypeMain}),
	}
	netSpec := v1alpha2.NetworksSpec{
		Type: v1alpha2.NetworksTypeClusterNetwork,
		Name: ipamNetworkName,
	}
	if ipAddressName != "" {
		netSpec.IPAddressName = ipAddressName
	}
	opts = append(opts, vm.WithNetwork(netSpec))
	opts = append(opts, extraOpts...)
	return vm.New(opts...)
}
