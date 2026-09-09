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
	"github.com/deckhouse/virtualization/test/e2e/eventually"
	"github.com/deckhouse/virtualization/test/e2e/internal/framework"
	"github.com/deckhouse/virtualization/test/e2e/internal/label"
	"github.com/deckhouse/virtualization/test/e2e/internal/object"
	vmobs "github.com/deckhouse/virtualization/test/e2e/internal/observer/vm"
	vmopobs "github.com/deckhouse/virtualization/test/e2e/internal/observer/vmop"
	"github.com/deckhouse/virtualization/test/e2e/internal/precheck"
	"github.com/deckhouse/virtualization/test/e2e/internal/util"
)

const (
	ipamNetworkName   = "cn-4006-for-e2e-test"
	noPoolNetworkName = "cn-4007-for-e2e-test"

	// Static IP addresses used across the IPAM e2e tests (from the e2e-ipam-pool
	// 192.168.200.0/24 bound to cn-4006).
	staticIPForStatic        = "192.168.200.50"
	staticIPForWatcher       = "192.168.200.51"
	staticIPForHotplugStatic = "192.168.200.52"
	staticIPForHotplugAuto   = "192.168.200.99"
)

var _ = Describe("VirtualMachineIPAMForAdditionalNetworks", Label(label.SIGCompute, precheck.PrecheckSDN), func() {
	var (
		ctx context.Context
		f   *framework.Framework
	)

	BeforeEach(func() {
		// TODO: Re-enable the suite.
		Skip("skipped as flaky: fix the instability, then remove this skip")

		ctx = context.Background()
		f = framework.NewFramework("vm-ipam-for-additional-networks")
		DeferCleanup(f.After)
		f.Before()
	})

	Describe("auto mode (DHCP)", func() {
		var (
			vdRoot    *v1alpha2.VirtualDisk
			testVM    *v1alpha2.VirtualMachine
			testVMObs vmobs.Observer
		)

		It("should allocate IP from pool, deliver via DHCP, and keep stable across restart", func() {
			By("Create VM with Main + cn-4006 (auto, no ipAddressName)", func() {
				ns := f.Namespace().Name
				vdRoot = object.NewVDFromCVI("vd-root", ns, object.PrecreatedCVICustomBIOS,
					vd.WithSize(ptr.To(resource.MustParse(vdCustomImageSize))),
				)
				testVM = buildIPAMVM("vm-auto", ns, vdRoot.Name, "")

				Expect(f.CreateWithDeferredDeletion(ctx, vdRoot, testVM)).To(Succeed())
				testVMObs = vmobs.StartObserver(ctx, f, testVM)
			})

			By("Wait for VM Running and NetworkReady=True", func() {
				err := testVMObs.WaitFor(vmobs.BeRunning(), framework.LongTimeout)
				Expect(err).NotTo(HaveOccurred())
				err = testVMObs.WaitFor(haveNetworkReady(), framework.LongTimeout)
				Expect(err).NotTo(HaveOccurred())
			})

			var allocatedIP string
			By("Verify status.networks has ipAddress for cn-4006", func() {
				updated := refreshVM(ctx, f, testVM)
				allocatedIP = getVMNetworkIPAddress(updated)
				Expect(allocatedIP).NotTo(BeEmpty(), "auto IPAddress should be allocated in status.networks")
			})

			By("Verify IP is present in guest OS via DHCP", func() {
				eventually.SSHReadyAsRoot(f, testVM, framework.LongTimeout)
				// EXCEPTION: guest-side wait (interface address over SSH), not a
				// Kubernetes resource — nothing to observe via an Observer.
				eventually.UntilAssertion(func(g Gomega) {
					output, err := f.SSHCommand(testVM.Name, testVM.Namespace, "ip -4 addr show eth1", framework.WithSSHUser("root"))
					g.Expect(err).NotTo(HaveOccurred())
					g.Expect(output).To(ContainSubstring(allocatedIP), "eth1 should have the allocated IP %s", allocatedIP)
				}, framework.LongTimeout, eventually.WithPolling(3*time.Second))
			})

			By("Restart VM and verify the same IP is kept (stability)", func() {
				previousRunningTime := time.Now()
				util.RebootVirtualMachineByVMOP(f, testVM)
				err := testVMObs.WaitFor(vmobs.BeRebootedAfter(previousRunningTime), framework.LongTimeout)
				if err != nil {
					util.SkipIfGuestPowerActionStuck(ctx, crclient.ObjectKeyFromObject(testVM))
				}
				Expect(err).NotTo(HaveOccurred())
				err = testVMObs.WaitFor(vmobs.BeRunning(), framework.LongTimeout)
				Expect(err).NotTo(HaveOccurred())
				err = testVMObs.WaitFor(haveNetworkReady(), framework.LongTimeout)
				Expect(err).NotTo(HaveOccurred())

				updated := refreshVM(ctx, f, testVM)
				Expect(getVMNetworkIPAddress(updated)).To(Equal(allocatedIP),
					"auto IP should be stable across restart (ownerRef on VM)")
			})
		})
	})

	Describe("static mode (ipAddressName)", func() {
		var (
			vdRoot    *v1alpha2.VirtualDisk
			testVM    *v1alpha2.VirtualMachine
			testVMObs vmobs.Observer
		)

		It("should use user-provided IPAddress and keep stable across restart", func() {
			By("Create IPAddress (Static) and VM referencing it", func() {
				ns := f.Namespace().Name
				Expect(util.CreateSDNIPAddress(ctx, f, "my-static-ip", ns,
					v1alpha2.NetworksTypeClusterNetwork, ipamNetworkName, staticIPForStatic)).To(Succeed())

				// Wait for the IPAddress to be allocated, observing the SDN CR
				// through a dynamic watch.
				util.UntilSDNIPAddressAllocated(ctx, "my-static-ip", ns, staticIPForStatic, framework.LongTimeout)

				vdRoot = object.NewVDFromCVI("vd-root", ns, object.PrecreatedCVICustomBIOS,
					vd.WithSize(ptr.To(resource.MustParse(vdCustomImageSize))),
				)
				testVM = buildIPAMVM("vm-static", ns, vdRoot.Name, "my-static-ip")

				Expect(f.CreateWithDeferredDeletion(ctx, vdRoot, testVM)).To(Succeed())
				testVMObs = vmobs.StartObserver(ctx, f, testVM)
			})

			By("Wait for VM Running and NetworkReady=True", func() {
				err := testVMObs.WaitFor(vmobs.BeRunning(), framework.LongTimeout)
				Expect(err).NotTo(HaveOccurred())
				err = testVMObs.WaitFor(haveNetworkReady(), framework.LongTimeout)
				Expect(err).NotTo(HaveOccurred())
			})

			By("Verify status.networks has the static IP", func() {
				updated := refreshVM(ctx, f, testVM)
				Expect(getVMNetworkIPAddress(updated)).To(Equal(staticIPForStatic))
			})

			By("Verify IP is present in guest OS", func() {
				eventually.SSHReadyAsRoot(f, testVM, framework.LongTimeout)
				// EXCEPTION: guest-side wait (interface address over SSH), not a
				// Kubernetes resource — nothing to observe via an Observer.
				eventually.UntilAssertion(func(g Gomega) {
					output, err := f.SSHCommand(testVM.Name, testVM.Namespace, "ip -4 addr show eth1", framework.WithSSHUser("root"))
					g.Expect(err).NotTo(HaveOccurred())
					g.Expect(output).To(ContainSubstring(staticIPForStatic))
				}, framework.LongTimeout, eventually.WithPolling(3*time.Second))
			})

			By("Restart VM and verify the same static IP is kept", func() {
				previousRunningTime := time.Now()
				util.RebootVirtualMachineByVMOP(f, testVM)
				err := testVMObs.WaitFor(vmobs.BeRebootedAfter(previousRunningTime), framework.LongTimeout)
				if err != nil {
					util.SkipIfGuestPowerActionStuck(ctx, crclient.ObjectKeyFromObject(testVM))
				}
				Expect(err).NotTo(HaveOccurred())
				err = testVMObs.WaitFor(vmobs.BeRunning(), framework.LongTimeout)
				Expect(err).NotTo(HaveOccurred())
				err = testVMObs.WaitFor(haveNetworkReady(), framework.LongTimeout)
				Expect(err).NotTo(HaveOccurred())

				updated := refreshVM(ctx, f, testVM)
				Expect(getVMNetworkIPAddress(updated)).To(Equal(staticIPForStatic))
			})
		})
	})

	Describe("skip problematic interface", func() {
		var (
			vdRoot    *v1alpha2.VirtualDisk
			testVM    *v1alpha2.VirtualMachine
			testVMObs vmobs.Observer
		)

		It("should start VM without problematic network and report error in NetworkReady", func() {
			By("Create VM with cn-4006 (auto) + cn-4007 (no pool, ipAddressName=nonexistent)", func() {
				ns := f.Namespace().Name
				vdRoot = object.NewVDFromCVI("vd-root", ns, object.PrecreatedCVICustomBIOS,
					vd.WithSize(ptr.To(resource.MustParse(vdCustomImageSize))),
				)
				testVM = buildIPAMVM("vm-skip", ns, vdRoot.Name, "",
					vm.WithNetwork(v1alpha2.NetworksSpec{
						Type:          v1alpha2.NetworksTypeClusterNetwork,
						Name:          noPoolNetworkName,
						IPAddressName: "nonexistent-ip",
					}),
				)

				Expect(f.CreateWithDeferredDeletion(ctx, vdRoot, testVM)).To(Succeed())
				testVMObs = vmobs.StartObserver(ctx, f, testVM)
			})

			By("Wait for VM Running (not blocked by problematic network)", func() {
				err := testVMObs.WaitFor(vmobs.BeRunning(), framework.LongTimeout)
				Expect(err).NotTo(HaveOccurred())
			})

			By("Verify NetworkReady=False with error about cn-4007", func() {
				err := testVMObs.WaitFor(haveNetworkNotReadyMentioning(noPoolNetworkName), framework.LongTimeout)
				Expect(err).NotTo(HaveOccurred())
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
			vdRoot    *v1alpha2.VirtualDisk
			testVM    *v1alpha2.VirtualMachine
			testVMObs vmobs.Observer
		)

		It("should provision interface when IPAddress is created after VM start (watcher)", func() {
			By("Create VM with cn-4006 + ipAddressName=not-yet-created (static, IPAddress absent)", func() {
				ns := f.Namespace().Name
				vdRoot = object.NewVDFromCVI("vd-root", ns, object.PrecreatedCVICustomBIOS,
					vd.WithSize(ptr.To(resource.MustParse(vdCustomImageSize))),
				)
				testVM = buildIPAMVM("vm-watcher", ns, vdRoot.Name, "watcher-ip")

				Expect(f.CreateWithDeferredDeletion(ctx, vdRoot, testVM)).To(Succeed())
				testVMObs = vmobs.StartObserver(ctx, f, testVM)
			})

			By("Wait for VM Running and NetworkReady=False (IPAddress does not exist)", func() {
				err := testVMObs.WaitFor(vmobs.BeRunning(), framework.LongTimeout)
				Expect(err).NotTo(HaveOccurred())

				err = testVMObs.WaitFor(haveNetworkNotReady(), framework.LongTimeout)
				Expect(err).NotTo(HaveOccurred())
			})

			By("Verify cn-4006 is skipped in networks-spec", func() {
				spec := getPodNetworksSpec(ctx, f, testVM.Name, testVM.Namespace)
				Expect(networkInPodSpec(spec, ipamNetworkName)).To(BeFalse(),
					"cn-4006 should be skipped while IPAddress does not exist")
			})

			By("Create the IPAddress (Static) — watcher should trigger reconciliation", func() {
				Expect(util.CreateSDNIPAddress(ctx, f, "watcher-ip", f.Namespace().Name,
					v1alpha2.NetworksTypeClusterNetwork, ipamNetworkName, staticIPForWatcher)).To(Succeed())
			})

			By("Verify NetworkReady=True and cn-4006 is now provisioned (via watcher, no restart)", func() {
				podUIDBefore := getVirtLauncherPodUID(ctx, f, testVM.Name, testVM.Namespace)

				// The watcher-triggered hotplug must never ask for a restart: enforced
				// as an invariant on every VM status update from here to the end of
				// the spec (it replaces the previous bounded Consistently window).
				testVMObs.Never(neverAwaitRestart("VM must not require restart for watcher-triggered hotplug"))

				err := testVMObs.WaitFor(haveNetworkReadyWithIP(staticIPForWatcher), framework.LongTimeout)
				Expect(err).NotTo(HaveOccurred())

				By("Verify pod was not recreated (hotplug via watcher)")
				podUIDAfter := getVirtLauncherPodUID(ctx, f, testVM.Name, testVM.Namespace)
				Expect(podUIDAfter).To(Equal(podUIDBefore), "pod should not be recreated on IPAddress creation")
			})
		})
	})

	Describe("hotplug static to auto", func() {
		var (
			vdRoot    *v1alpha2.VirtualDisk
			testVM    *v1alpha2.VirtualMachine
			testVMObs vmobs.Observer
		)

		It("should switch from static to auto without restart (hotplug)", func() {
			// The switch never completes on the running VM: the controller creates
			// the Auto IPAddress and the SDN allocates a fresh address for it, but
			// the VM's interface and status.networks keep the old static IP
			// indefinitely (reproduced manually: the Auto IPAddress reaches
			// Allocated while status.networks still reports the static IP after
			// 5+ minutes). The data-plane hotplug re-lease is an SDN-side gap the
			// test cannot work around without a restart, which would defeat the
			// spec's purpose. TODO: unskip when the SDN applies the auto-allocated
			// IP to a running VM without a restart.
			Skip("static-to-auto IP switch does not reach the running VM without a restart (SDN-side gap)")

			By("Create IPAddress (Static) and VM with static ipAddressName", func() {
				ns := f.Namespace().Name
				Expect(util.CreateSDNIPAddress(ctx, f, "hotplug-static", ns,
					v1alpha2.NetworksTypeClusterNetwork, ipamNetworkName, staticIPForHotplugStatic)).To(Succeed())

				// Wait for the IPAddress to be allocated, observing the SDN CR
				// through a dynamic watch.
				util.UntilSDNIPAddressAllocated(ctx, "hotplug-static", ns, staticIPForHotplugStatic, framework.LongTimeout)

				vdRoot = object.NewVDFromCVI("vd-root", ns, object.PrecreatedCVICustomBIOS,
					vd.WithSize(ptr.To(resource.MustParse(vdCustomImageSize))),
				)
				testVM = buildIPAMVM("vm-hotplug-sa", ns, vdRoot.Name, "hotplug-static")

				Expect(f.CreateWithDeferredDeletion(ctx, vdRoot, testVM)).To(Succeed())
				testVMObs = vmobs.StartObserver(ctx, f, testVM)
			})

			By("Wait for VM Running and NetworkReady=True", func() {
				err := testVMObs.WaitFor(vmobs.BeRunning(), framework.LongTimeout)
				Expect(err).NotTo(HaveOccurred())
				err = testVMObs.WaitFor(haveNetworkReady(), framework.LongTimeout)
				Expect(err).NotTo(HaveOccurred())
			})

			By("Verify static IP in status", func() {
				updated := refreshVM(ctx, f, testVM)
				Expect(getVMNetworkIPAddress(updated)).To(Equal(staticIPForHotplugStatic))
			})

			By("Remove ipAddressName (switch to auto)", func() {
				podUIDBefore := getVirtLauncherPodUID(ctx, f, testVM.Name, testVM.Namespace)

				// The hotplug must never ask for a restart: enforced as an invariant
				// on every VM status update from here to the end of the spec.
				testVMObs.Never(neverAwaitRestart("VM must not require restart for network change"))

				updated := refreshVM(ctx, f, testVM)
				updated.Spec.Networks[1].IPAddressName = ""
				Expect(f.Clients.GenericClient().Update(ctx, updated)).To(Succeed())

				By("Verify NetworkReady=True with new auto IP and pod not recreated")
				err := testVMObs.WaitFor(haveNetworkReadyWithAutoIPOtherThan(staticIPForHotplugStatic), framework.LongTimeout)
				Expect(err).NotTo(HaveOccurred())

				podUIDAfter := getVirtLauncherPodUID(ctx, f, testVM.Name, testVM.Namespace)
				Expect(podUIDAfter).To(Equal(podUIDBefore), "pod should not be recreated on hotplug")
			})
		})
	})

	Describe("hotplug auto to static", func() {
		var (
			vdRoot    *v1alpha2.VirtualDisk
			testVM    *v1alpha2.VirtualMachine
			testVMObs vmobs.Observer
		)

		It("should switch from auto to static without restart (hotplug)", func() {
			By("Create VM in auto mode", func() {
				ns := f.Namespace().Name
				vdRoot = object.NewVDFromCVI("vd-root", ns, object.PrecreatedCVICustomBIOS,
					vd.WithSize(ptr.To(resource.MustParse(vdCustomImageSize))),
				)
				testVM = buildIPAMVM("vm-hotplug-as", ns, vdRoot.Name, "")

				Expect(f.CreateWithDeferredDeletion(ctx, vdRoot, testVM)).To(Succeed())
				testVMObs = vmobs.StartObserver(ctx, f, testVM)
			})

			By("Wait for VM Running and NetworkReady=True", func() {
				err := testVMObs.WaitFor(vmobs.BeRunning(), framework.LongTimeout)
				Expect(err).NotTo(HaveOccurred())
				err = testVMObs.WaitFor(haveNetworkReady(), framework.LongTimeout)
				Expect(err).NotTo(HaveOccurred())
			})

			By("Create IPAddress (Static) and add ipAddressName (switch to static)", func() {
				podUIDBefore := getVirtLauncherPodUID(ctx, f, testVM.Name, testVM.Namespace)

				// The hotplug must never ask for a restart: enforced as an invariant
				// on every VM status update from here to the end of the spec.
				testVMObs.Never(neverAwaitRestart("VM must not require restart for ipAddressName change"))

				Expect(util.CreateSDNIPAddress(ctx, f, "hotplug-as-ip", f.Namespace().Name,
					v1alpha2.NetworksTypeClusterNetwork, ipamNetworkName, staticIPForHotplugAuto)).To(Succeed())

				patch := `[{"op":"add","path":"/spec/networks/1/ipAddressName","value":"hotplug-as-ip"}]`
				Expect(f.Clients.GenericClient().Patch(ctx, testVM, crclient.RawPatch(types.JSONPatchType, []byte(patch)))).To(Succeed())

				By("Verify NetworkReady=True with static IP and pod not recreated")
				err := testVMObs.WaitFor(haveNetworkReadyWithIP(staticIPForHotplugAuto), framework.LongTimeout)
				Expect(err).NotTo(HaveOccurred())

				podUIDAfter := getVirtLauncherPodUID(ctx, f, testVM.Name, testVM.Namespace)
				Expect(podUIDAfter).To(Equal(podUIDBefore), "pod should not be recreated on hotplug")
			})
		})
	})

	Describe("live migration (auto)", func() {
		var (
			vdRoot    *v1alpha2.VirtualDisk
			testVM    *v1alpha2.VirtualMachine
			testVMObs vmobs.Observer
		)

		It("should preserve auto IP across live migration", func() {
			By("Create VM in auto mode", func() {
				ns := f.Namespace().Name
				vdRoot = object.NewVDFromCVI("vd-root", ns, object.PrecreatedCVICustomBIOS,
					vd.WithSize(ptr.To(resource.MustParse(vdCustomImageSize))),
				)
				testVM = buildIPAMVM("vm-migrate", ns, vdRoot.Name, "")

				Expect(f.CreateWithDeferredDeletion(ctx, vdRoot, testVM)).To(Succeed())
				testVMObs = vmobs.StartObserver(ctx, f, testVM)
			})

			By("Wait for VM Running and NetworkReady=True", func() {
				err := testVMObs.WaitFor(vmobs.BeRunning(), framework.LongTimeout)
				Expect(err).NotTo(HaveOccurred())
				err = testVMObs.WaitFor(haveNetworkReady(), framework.LongTimeout)
				Expect(err).NotTo(HaveOccurred())
			})

			var allocatedIP string
			By("Record the allocated IP", func() {
				updated := refreshVM(ctx, f, testVM)
				allocatedIP = getVMNetworkIPAddress(updated)
				Expect(allocatedIP).NotTo(BeEmpty())
			})

			var migrateVMOPObs vmopobs.Observer
			By("Migrate VM", func() {
				migrateVMOP := util.MigrateVirtualMachine(f, testVM, vmop.WithGenerateName("vmop-migrate-ipam-"))
				migrateVMOPObs = vmopobs.StartObserver(ctx, migrateVMOP)
			})

			By("Wait for migration to complete", func() {
				err := migrateVMOPObs.WaitFor(vmopobs.BeCompleted(), framework.LongTimeout)
				Expect(err).NotTo(HaveOccurred())
				err = testVMObs.WaitFor(vmobs.HaveMigrationSucceeded(), framework.LongTimeout)
				if err != nil {
					// TODO: remove temporary migration skip logic when both known issues
					// are fixed: kubevirt "client socket is closed" and Volume(s)UpdateError.
					util.SkipIfKnownMigrationFailureWithContext(ctx, testVM)
				}
				Expect(err).NotTo(HaveOccurred())
			})

			By("Verify the same IP is preserved after migration", func() {
				err := testVMObs.WaitFor(vmobs.BeRunning(), framework.LongTimeout)
				Expect(err).NotTo(HaveOccurred())
				err = testVMObs.WaitFor(haveNetworkReady(), framework.LongTimeout)
				Expect(err).NotTo(HaveOccurred())

				updated := refreshVM(ctx, f, testVM)
				Expect(getVMNetworkIPAddress(updated)).To(Equal(allocatedIP),
					"auto IP should be preserved across live migration")
			})
		})
	})
})

// neverAwaitRestart returns a predicate that reports an invariant violation
// as soon as the VM's AwaitingRestartToApplyConfiguration condition turns
// True. Intended for use with the VM observer's Never.
func neverAwaitRestart(message string) vmobs.Predicate {
	return func(vm *v1alpha2.VirtualMachine) (bool, error) {
		cond, _ := conditions.GetCondition(vmcondition.TypeAwaitingRestartToApplyConfiguration, vm.Status.Conditions)
		if cond.Status == metav1.ConditionTrue {
			return true, fmt.Errorf("%s", message)
		}
		return false, nil
	}
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
//
// The custom image brings up every non-primary NIC with DHCP at boot
// (S41extranics), which is exactly what the IPAM tests need for eth1; there is
// no cloud-init and guest commands run as root. The BIOS flavor carries the
// same rootfs as the EFI one and boots on a quarter of its memory.
func buildIPAMVM(name, ns, vdRootName, ipAddressName string, extraOpts ...vm.Option) *v1alpha2.VirtualMachine {
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

// haveNetworkNotReady reports the VM's NetworkReady condition is False.
func haveNetworkNotReady() vmobs.Predicate {
	return func(vm *v1alpha2.VirtualMachine) (bool, error) {
		cond, _ := conditions.GetCondition(vmcondition.TypeNetworkReady, vm.Status.Conditions)
		return cond.Status == metav1.ConditionFalse, nil
	}
}

// haveNetworkNotReadyMentioning reports the VM's NetworkReady condition is
// False with the NetworkNotReady reason and a message mentioning name.
func haveNetworkNotReadyMentioning(name string) vmobs.Predicate {
	return func(vm *v1alpha2.VirtualMachine) (bool, error) {
		cond, exists := conditions.GetCondition(vmcondition.TypeNetworkReady, vm.Status.Conditions)
		return exists &&
			cond.Status == metav1.ConditionFalse &&
			cond.Reason == vmcondition.ReasonNetworkNotReady.String() &&
			strings.Contains(cond.Message, name), nil
	}
}

// haveNetworkReadyWithIP reports the VM's NetworkReady condition is True and
// the additional network carries exactly the given IP address.
func haveNetworkReadyWithIP(ip string) vmobs.Predicate {
	return func(vm *v1alpha2.VirtualMachine) (bool, error) {
		cond, _ := conditions.GetCondition(vmcondition.TypeNetworkReady, vm.Status.Conditions)
		return cond.Status == metav1.ConditionTrue && getVMNetworkIPAddress(vm) == ip, nil
	}
}

// haveNetworkReadyWithAutoIPOtherThan reports the VM's NetworkReady condition
// is True and the additional network carries a non-empty IP address different
// from old (i.e. a fresh auto-allocated one).
func haveNetworkReadyWithAutoIPOtherThan(old string) vmobs.Predicate {
	return func(vm *v1alpha2.VirtualMachine) (bool, error) {
		cond, _ := conditions.GetCondition(vmcondition.TypeNetworkReady, vm.Status.Conditions)
		ip := getVMNetworkIPAddress(vm)
		return cond.Status == metav1.ConditionTrue && ip != "" && ip != old, nil
	}
}
