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
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"
	crclient "sigs.k8s.io/controller-runtime/pkg/client"

	vmbuilder "github.com/deckhouse/virtualization-controller/pkg/builder/vm"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/nodeusbdevicecondition"
	"github.com/deckhouse/virtualization/test/e2e/internal/framework"
	"github.com/deckhouse/virtualization/test/e2e/internal/object"
	"github.com/deckhouse/virtualization/test/e2e/internal/util"
)

var _ = Describe("VirtualMachineUSB", func() {
	var (
		f *framework.Framework
		t *VMUSBTest
	)

	BeforeEach(func() {
		f = framework.NewFramework("vm-usb")
		DeferCleanup(func() {
			t.unassignNodeUSB()
			f.After()
		})

		f.Before()
		t = NewVMUSBTest(f)
	})

	It("should write data to USB device and preserve after reconnection", func() {
		By("Environment preparation", func() {
			// TODO: Move all preflight checks to the `SynchronizedBeforeSuite` to ensure they are executed in a synchronized context.
			if !t.checkDummyHCDConfigured() {
				Skip("dummy_hcd is not configured. Run generate_dummy_hcd_ngc.sh first.")
			}

			t.GenerateEnvironmentResources()
			err := f.CreateWithDeferredDeletion(context.Background(), t.VD)
			Expect(err).NotTo(HaveOccurred())

			t.assignNodeUSB()
		})

		By("Verifying NodeUSBDevice is not attached before VM attachment", func() {
			Eventually(func(g Gomega) {
				nodeUSBDevice, err := t.Framework.VirtClient().NodeUSBDevices().Get(t.ctx, t.NodeUSBDevice.Name, metav1.GetOptions{})
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(nodeUSBAttachedCondition(nodeUSBDevice)).NotTo(BeNil())
				g.Expect(nodeUSBAttachedCondition(nodeUSBDevice).Status).To(Equal(metav1.ConditionFalse))
			}).WithTimeout(framework.MaxTimeout).WithPolling(time.Second).Should(Succeed())
		})

		By("Creating VM with USB device", func() {
			err := f.CreateWithDeferredDeletion(context.Background(), t.VM)
			Expect(err).NotTo(HaveOccurred())

			util.UntilObjectPhase(string(v1alpha2.MachineRunning), framework.LongTimeout, t.VM)
			util.UntilVMAgentReady(crclient.ObjectKeyFromObject(t.VM), framework.LongTimeout)
			util.UntilSSHReady(f, t.VM, framework.MiddleTimeout)
		})

		By("Waiting for USB device to be attached and ready", func() {
			Eventually(func() error {
				vm, err := t.Framework.VirtClient().VirtualMachines(t.VM.Namespace).Get(t.ctx, t.VM.Name, metav1.GetOptions{})
				Expect(err).NotTo(HaveOccurred())

				for _, dev := range vm.Status.USBDevices {
					if dev.Name == t.NodeUSBDevice.Name && dev.Attached && dev.Ready {
						return nil
					}
				}

				return fmt.Errorf("USB device %s not attached or not ready", t.NodeUSBDevice.Name)
			}).WithTimeout(framework.MaxTimeout).WithPolling(time.Second).Should(Succeed())
		})

		By("Verifying NodeUSBDevice is attached", func() {
			Eventually(func(g Gomega) {
				nodeUSBDevice, err := t.Framework.VirtClient().NodeUSBDevices().Get(t.ctx, t.NodeUSBDevice.Name, metav1.GetOptions{})
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(nodeUSBAttachedCondition(nodeUSBDevice)).NotTo(BeNil())
				g.Expect(nodeUSBAttachedCondition(nodeUSBDevice).Status).To(Equal(metav1.ConditionTrue))
			}).WithTimeout(framework.MaxTimeout).WithPolling(time.Second).Should(Succeed())
		})

		By("Mounting USB device", func() {
			t.mountUSB()
		})

		By("Writing data to USB device", func() {
			result, err := t.Framework.SSHCommand(t.VM.Name, t.VM.Namespace, fmt.Sprintf("echo \"%s\" | sudo tee %s && sudo sync && sudo umount /mnt/usb", t.testContent, t.testFile))

			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(ContainSubstring(t.testContent))
		})

		By("Migrating VM", func() {
			util.MigrateVirtualMachine(f, t.VM)
			util.UntilVMMigrationSucceeded(crclient.ObjectKeyFromObject(t.VM), framework.LongTimeout)

			util.UntilObjectPhase(string(v1alpha2.MachineRunning), framework.ShortTimeout, t.VM)
			util.UntilSSHReady(f, t.VM, framework.ShortTimeout)
		})

		By("Waiting for USB device to be ready after migration", func() {
			Eventually(func() error {
				vm, err := t.Framework.VirtClient().VirtualMachines(t.VM.Namespace).Get(t.ctx, t.VM.Name, metav1.GetOptions{})
				Expect(err).NotTo(HaveOccurred())

				for _, dev := range vm.Status.USBDevices {
					if dev.Name == t.NodeUSBDevice.Name && dev.Attached && dev.Ready {
						return nil
					}
				}

				return fmt.Errorf("USB device %s not ready after migration", t.NodeUSBDevice.Name)
			}).WithTimeout(framework.MaxTimeout).WithPolling(time.Second).Should(Succeed())
		})

		By("Verifying NodeUSBDevice is attached after migration", func() {
			Eventually(func(g Gomega) {
				nodeUSBDevice, err := t.Framework.VirtClient().NodeUSBDevices().Get(t.ctx, t.NodeUSBDevice.Name, metav1.GetOptions{})
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(nodeUSBAttachedCondition(nodeUSBDevice)).NotTo(BeNil())
				g.Expect(nodeUSBAttachedCondition(nodeUSBDevice).Status).To(Equal(metav1.ConditionTrue))
			}).WithTimeout(framework.MaxTimeout).WithPolling(time.Second).Should(Succeed())
		})

		By("Remounting USB device after migration", func() {
			t.mountUSB()
		})

		By("Verifying data persists after migration", func() {
			result, err := t.Framework.SSHCommand(t.VM.Name, t.VM.Namespace, fmt.Sprintf("cat %s", t.testFile))
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(ContainSubstring(t.testContent))
		})
	})
})

type VMUSBTest struct {
	ctx       context.Context
	Framework *framework.Framework

	VM            *v1alpha2.VirtualMachine
	VD            *v1alpha2.VirtualDisk
	NodeUSBDevice *v1alpha2.NodeUSBDevice

	testFile    string
	testContent string
}

func NewVMUSBTest(f *framework.Framework) *VMUSBTest {
	return &VMUSBTest{
		Framework:   f,
		ctx:         context.Background(),
		testFile:    "/mnt/usb/testfile.txt",
		testContent: "Hello USB " + time.Now().Format(time.RFC3339),
	}
}

func (t *VMUSBTest) checkDummyHCDConfigured() bool {
	ctx := context.Background()
	virtClient := t.Framework.VirtClient()

	nodeUSBList, err := virtClient.NodeUSBDevices().List(ctx, metav1.ListOptions{})
	if err != nil {
		return false
	}

	if len(nodeUSBList.Items) == 0 {
		return false
	}

	for _, nodeUSB := range nodeUSBList.Items {
		if nodeUSB.Status.Attributes.VendorID == "1d6b" && nodeUSB.Status.Attributes.ProductID == "0104" {
			return true
		}
	}

	return false
}

func (t *VMUSBTest) GenerateEnvironmentResources() {
	ctx := context.Background()
	virtClient := t.Framework.VirtClient()

	nodeUSBList, err := virtClient.NodeUSBDevices().List(ctx, metav1.ListOptions{})
	Expect(err).NotTo(HaveOccurred())

	var freeUSB *v1alpha2.NodeUSBDevice
	for i := range nodeUSBList.Items {
		if nodeUSBList.Items[i].Status.Attributes.VendorID == "1d6b" && nodeUSBList.Items[i].Status.Attributes.ProductID == "0104" && nodeUSBList.Items[i].Spec.AssignedNamespace == "" {
			freeUSB = &nodeUSBList.Items[i]
			break
		}
	}
	Expect(freeUSB).NotTo(BeNil(), "no free USB devices available")

	t.NodeUSBDevice = freeUSB

	usbNodeName := t.NodeUSBDevice.Status.NodeName
	Expect(usbNodeName).NotTo(BeEmpty(), "USB device must have a node assigned")

	t.VD = object.NewVDFromCVI("vd-usb-test", t.Framework.Namespace().Name, object.PrecreatedCVIAlpineBIOS)

	t.VM = vmbuilder.New(
		vmbuilder.WithName("vm-usb-test"),
		vmbuilder.WithNamespace(t.Framework.Namespace().Name),
		vmbuilder.WithCPU(1, ptr.To("100%")),
		vmbuilder.WithMemory(resource.MustParse("512Mi")),
		vmbuilder.WithVirtualMachineClass(object.DefaultVMClass),
		vmbuilder.WithProvisioningUserData(object.AlpineCloudInit),
		vmbuilder.WithLiveMigrationPolicy(v1alpha2.AlwaysSafeMigrationPolicy),
		vmbuilder.WithBlockDeviceRefs(v1alpha2.BlockDeviceSpecRef{Kind: v1alpha2.DiskDevice, Name: t.VD.Name}),
		vmbuilder.WithUSBDevices([]v1alpha2.USBDeviceSpecRef{{Name: t.NodeUSBDevice.Name}}),
	)
}

func (t *VMUSBTest) assignNodeUSB() {
	nodeUSBCopy := t.NodeUSBDevice.DeepCopy()
	nodeUSBCopy.Spec.AssignedNamespace = t.Framework.Namespace().Name
	_, err := t.Framework.VirtClient().NodeUSBDevices().Update(t.ctx, nodeUSBCopy, metav1.UpdateOptions{})
	Expect(err).NotTo(HaveOccurred())

	Eventually(func() error {
		_, err := t.Framework.VirtClient().USBDevices(t.Framework.Namespace().Name).Get(t.ctx, t.NodeUSBDevice.Name, metav1.GetOptions{})
		if err != nil {
			return err
		}

		return nil
	}).WithTimeout(framework.MaxTimeout).WithPolling(time.Second).Should(Succeed())
}

func (t *VMUSBTest) mountUSB() {
	serial := t.NodeUSBDevice.Status.Attributes.Serial
	Expect(serial).NotTo(BeEmpty(), "USB device serial must be set")

	mountCmd := fmt.Sprintf(`
		usb_serial=%q
		: > /tmp/usb-mount.err
		for serial_file in /sys/bus/usb/devices/*/serial; do
			if [ -f "$serial_file" ] && [ "$(cat "$serial_file")" = "$usb_serial" ]; then
				usb_sys_device="$(dirname "$serial_file")"
				break
			fi
		done
		[ -n "$usb_sys_device" ] || { echo "USB device with serial $usb_serial not found" >/tmp/usb-mount.err; exit 1; }

		for block_path in /sys/block/*; do
			[ -e "$block_path" ] || continue
			device_realpath="$(readlink -f "$block_path/device" 2>/dev/null || true)"
			case "$device_realpath" in
				"$usb_sys_device"/*)
					block_name="$(basename "$block_path")"
					if [ -b "/dev/$block_name" ]; then
						mount_device="/dev/$block_name"
						break
					fi
					;;
			esac
			done
		[ -n "$mount_device" ] || {
			echo "USB block device for serial $usb_serial not found" >>/tmp/usb-mount.err
			echo "usb_sys_device=$usb_sys_device" >>/tmp/usb-mount.err
			find "$usb_sys_device" -maxdepth 8 -print >>/tmp/usb-mount.err 2>&1 || true
			for block_path in /sys/block/*; do
				[ -e "$block_path" ] || continue
				echo "$(basename "$block_path") -> $(readlink -f "$block_path/device" 2>/dev/null || true)" >>/tmp/usb-mount.err
			done
			exit 1
		}

		for partition in /sys/block/$block_name/${block_name}[0-9]* /sys/block/$block_name/${block_name}p[0-9]*; do
			if [ -e "$partition" ] && [ -b "/dev/$(basename "$partition")" ]; then
				mount_device="/dev/$(basename "$partition")"
				break
			fi
		done

		sudo mkdir -p /mnt/usb && \
		(sudo mountpoint -q /mnt/usb || sudo mount "$mount_device" /mnt/usb 2>>/tmp/usb-mount.err || sudo mount -o rw "$mount_device" /mnt/usb 2>>/tmp/usb-mount.err) && \
		ls -la /mnt/usb
	`, serial)

	Eventually(func() error {
		_, err := t.Framework.SSHCommand(t.VM.Name, t.VM.Namespace, mountCmd, framework.WithSSHTimeout(framework.MiddleTimeout))
		return err
	}).WithTimeout(framework.MiddleTimeout).WithPolling(time.Second).Should(Succeed(), t.usbDiagnostics())
}

func (t *VMUSBTest) usbDiagnostics() string {
	diagnosticsCmd := `
		echo "mount error:" && cat /tmp/usb-mount.err 2>/dev/null || true
		echo "mount:" && mount || true
		echo "usb serials:" && for serial_file in /sys/bus/usb/devices/*/serial; do [ -f "$serial_file" ] && echo "$serial_file=$(cat "$serial_file")"; done || true
		echo "usb sysfs:" && find /sys/bus/usb/devices -maxdepth 3 -print || true
		echo "lsblk:" && lsblk -a -o NAME,PATH,TYPE,TRAN,RM,SERIAL,MODEL || true
		echo "lsusb:" && lsusb || true
		echo "dmesg:" && sudo dmesg | tail -n 100 || true
	`

	result, err := t.Framework.SSHCommand(t.VM.Name, t.VM.Namespace, diagnosticsCmd, framework.WithSSHTimeout(framework.MiddleTimeout))
	if err != nil {
		return fmt.Sprintf("failed to collect USB diagnostics: %v", err)
	}

	return result
}

func nodeUSBAttachedCondition(nodeUSBDevice *v1alpha2.NodeUSBDevice) *metav1.Condition {
	if nodeUSBDevice == nil {
		return nil
	}

	return meta.FindStatusCondition(nodeUSBDevice.Status.Conditions, string(nodeusbdevicecondition.AttachedType))
}

func (t *VMUSBTest) unassignNodeUSB() {
	GinkgoHelper()

	if t.NodeUSBDevice == nil {
		return
	}

	nodeUSBDevice, err := t.Framework.VirtClient().NodeUSBDevices().Get(t.ctx, t.NodeUSBDevice.Name, metav1.GetOptions{})
	Expect(err).NotTo(HaveOccurred())

	nodeUSBDevice.Spec.AssignedNamespace = ""
	_, err = t.Framework.VirtClient().NodeUSBDevices().Update(t.ctx, nodeUSBDevice, metav1.UpdateOptions{})
	if err != nil {
		fmt.Printf("Failed to unassign NodeUSBDevice: %v\n", err)
	}

	Eventually(func() error {
		_, err := t.Framework.VirtClient().USBDevices(t.Framework.Namespace().Name).Get(t.ctx, t.NodeUSBDevice.Name, metav1.GetOptions{})
		return err
	}).WithTimeout(framework.MaxTimeout).WithPolling(time.Second).Should(HaveOccurred())
}
