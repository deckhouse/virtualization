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
	"math/rand"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"

	vdbuilder "github.com/deckhouse/virtualization-controller/pkg/builder/vd"
	vmbuilder "github.com/deckhouse/virtualization-controller/pkg/builder/vm"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/test/e2e/eventually"
	"github.com/deckhouse/virtualization/test/e2e/internal/framework"
	"github.com/deckhouse/virtualization/test/e2e/internal/label"
	"github.com/deckhouse/virtualization/test/e2e/internal/object"
	"github.com/deckhouse/virtualization/test/e2e/internal/observer"
	nodeusbobs "github.com/deckhouse/virtualization/test/e2e/internal/observer/nodeusbdevice"
	usbdevobs "github.com/deckhouse/virtualization/test/e2e/internal/observer/usbdevice"
	vmobs "github.com/deckhouse/virtualization/test/e2e/internal/observer/vm"
	"github.com/deckhouse/virtualization/test/e2e/internal/precheck"
	"github.com/deckhouse/virtualization/test/e2e/internal/util"
)

var _ = Describe("VirtualMachineUSB", Label(label.SIGCompute, precheck.PrecheckUSB), func() {
	var (
		f   *framework.Framework
		t   *VMUSBTest
		ctx context.Context
	)

	BeforeEach(func() {
		ctx = context.Background()
		f = framework.NewFramework("vm-usb")
		DeferCleanup(func(ctx context.Context) {
			t.unassignNodeUSB()
			f.After(ctx)
		})

		f.Before()
		t = NewVMUSBTest(ctx, f)
	})

	It("should write data to USB device and preserve after reconnection", func() {
		// TODO(e2e-flaky-parallel): flaky under parallel load on the 3-node cluster (migration of hotplugged USB disk). Re-enable once stabilized.
		Skip("flaky under parallel load: hotplugged-USB-disk migration")
		By("Environment preparation", func() {
			// TODO: Move all preflight checks to the `SynchronizedBeforeSuite` to ensure they are executed in a synchronized context.
			if !t.checkDummyHCDConfigured(ctx) {
				Skip("dummy_hcd is not configured. Run generate_dummy_hcd_ngc.sh first.")
			}

			t.GenerateEnvironmentResources(ctx)
			err := f.CreateWithDeferredDeletion(ctx, t.VD)
			Expect(err).NotTo(HaveOccurred())

			t.assignNodeUSB()
		})

		By("Verifying NodeUSBDevice is not attached before VM attachment", func() {
			t.waitForNodeUSBAttached(metav1.ConditionFalse)
		})

		By("Creating VM with USB device", func() {
			err := f.CreateWithDeferredDeletion(ctx, t.VM)
			Expect(err).NotTo(HaveOccurred())

			t.vmObs = vmobs.StartObserver(ctx, f, t.VM)
			t.vmObs.Never(vmobs.BeFailed())
			err = t.vmObs.WaitFor(vmobs.BeRunning(), framework.LongTimeout)
			Expect(err).NotTo(HaveOccurred())
			// Running only means qemu has started; wait for the guest agent so the
			// guest is fully booted before the short SSH readiness window below.
			err = t.vmObs.WaitFor(vmobs.BeAgentReady(), framework.LongTimeout)
			Expect(err).NotTo(HaveOccurred())
			// mkfs.vfat and lsblk are baked into the custom image, so there is no
			// cloud-init package installation to wait for.
			eventually.SSHReadyAsRoot(f, t.VM, framework.MiddleTimeout)
		})

		By("Waiting for USB device to be attached and ready", func() {
			t.waitForVMUSBReady("USB device %s not attached or not ready")
		})

		By("Verifying NodeUSBDevice is attached", func() {
			t.waitForNodeUSBAttached(metav1.ConditionTrue)
		})

		By("Mounting USB device", func() {
			GinkgoWriter.Println("Finding USB device")
			mountDevice := t.findUSBMountDevice()
			GinkgoWriter.Println("Found USB device:", mountDevice)

			GinkgoWriter.Println("Formatting USB device")
			t.formatUSBDevice(mountDevice)

			GinkgoWriter.Println("Mounting USB device")
			t.mountUSBDevice(mountDevice)
		})

		By("Writing data to USB device", func() {
			t.writeUSBTestData()
		})

		By("Migrating VM", func() {
			vmop := util.MigrateVirtualMachine(f, t.VM)
			// The wait consults the known kubevirt migration flakes (e.g. the
			// hotplugged disk "Operation not permitted" race) and skips instead
			// of failing when one of them hits.
			util.UntilVMOPMigrationSucceeded(ctx, vmop, framework.MaxTimeout)
			err := t.vmObs.WaitFor(vmobs.HaveMigrationSucceeded(), framework.MaxTimeout)
			if err != nil {
				// TODO: remove temporary migration skip logic when both known issues are
				// fixed: kubevirt "client socket is closed" and Volume(s)UpdateError.
				util.SkipIfKnownMigrationFailureWithContext(ctx, t.VM)
			}
			Expect(err).NotTo(HaveOccurred())

			err = t.vmObs.WaitFor(vmobs.BeRunning(), framework.ShortTimeout)
			Expect(err).NotTo(HaveOccurred())
			eventually.SSHReadyAsRoot(f, t.VM, framework.ShortTimeout)
		})

		By("Waiting for USB device to be ready after migration", func() {
			t.waitForVMUSBReady("USB device %s not ready after migration")
		})

		By("Verifying NodeUSBDevice is attached after migration", func() {
			t.waitForNodeUSBAttached(metav1.ConditionTrue)
		})

		By("Remounting USB device after migration", func() {
			GinkgoWriter.Println("Finding USB device")
			mountDevice := t.findUSBMountDevice()
			GinkgoWriter.Println("Found USB device:", mountDevice)

			GinkgoWriter.Println("Remounting USB device")
			t.mountUSBDevice(mountDevice)
		})

		By("Verifying data persists after migration", func() {
			t.verifyUSBTestData()
		})
	})
})

type VMUSBTest struct {
	ctx       context.Context
	Framework *framework.Framework

	VM            *v1alpha2.VirtualMachine
	VD            *v1alpha2.VirtualDisk
	NodeUSBDevice *v1alpha2.NodeUSBDevice

	vmObs      vmobs.Observer
	nodeUSBObs nodeusbobs.Observer

	testFile    string
	testContent string
}

func NewVMUSBTest(ctx context.Context, f *framework.Framework) *VMUSBTest {
	return &VMUSBTest{
		Framework:   f,
		ctx:         ctx,
		testFile:    "/mnt/usb/testfile.txt",
		testContent: "Hello USB " + time.Now().Format(time.RFC3339),
	}
}

func (t *VMUSBTest) checkDummyHCDConfigured(ctx context.Context) bool {
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

func (t *VMUSBTest) GenerateEnvironmentResources(ctx context.Context) {
	virtClient := t.Framework.VirtClient()

	nodeUSBList, err := virtClient.NodeUSBDevices().List(ctx, metav1.ListOptions{})
	Expect(err).NotTo(HaveOccurred())

	var freeUSBs []*v1alpha2.NodeUSBDevice
	for i := range nodeUSBList.Items {
		if nodeUSBList.Items[i].Status.Attributes.VendorID == "1d6b" && nodeUSBList.Items[i].Status.Attributes.ProductID == "0104" && nodeUSBList.Items[i].Spec.AssignedNamespace == "" {
			freeUSBs = append(freeUSBs, &nodeUSBList.Items[i])
		}
	}
	Expect(freeUSBs).NotTo(BeEmpty(), "no free USB devices available")

	freeUSB := freeUSBs[rand.New(rand.NewSource(time.Now().UnixNano())).Intn(len(freeUSBs))]

	GinkgoWriter.Println("Found free USB device:", freeUSB.Name)

	t.NodeUSBDevice = freeUSB

	usbNodeName := t.NodeUSBDevice.Status.NodeName
	Expect(usbNodeName).NotTo(BeEmpty(), "USB device must have a node assigned")

	// The custom image bakes in the USB drivers (usb-storage/uas are
	// compiled into the monolithic kernel), mkfs.vfat and lsblk; device nodes
	// appear via devtmpfs, so the guest needs neither udev nor sudo — commands
	// run as root over the baked SSH key.
	t.VD = object.NewVDFromCVI("vd-usb-test", t.Framework.Namespace().Name, object.PrecreatedCVICustomBIOS, vdbuilder.WithSize(ptr.To(resource.MustParse(vdCustomImageSize))))

	t.VM = vmbuilder.New(
		vmbuilder.WithName("vm-usb-test"),
		vmbuilder.WithNamespace(t.Framework.Namespace().Name),
		vmbuilder.WithCPU(1, ptr.To(object.CustomImageVMCoreFraction)),
		vmbuilder.WithMemory(resource.MustParse(object.CustomImageVMMemory)),
		vmbuilder.WithVirtualMachineClass(object.DefaultVMClass),
		// The custom image has no cloud-init; the guest agent is baked in.
		vmbuilder.WithLiveMigrationPolicy(v1alpha2.AlwaysSafeMigrationPolicy),
		vmbuilder.WithBlockDeviceRefs(v1alpha2.BlockDeviceSpecRef{Kind: v1alpha2.DiskDevice, Name: t.VD.Name}),
		vmbuilder.WithUSBDevices([]v1alpha2.USBDeviceSpecRef{{Name: t.NodeUSBDevice.Name}}),
	)
}

func (t *VMUSBTest) assignNodeUSB() {
	// Both observers are armed before the assignment so the resulting events
	// (USBDevice creation, NodeUSBDevice status transitions) are captured.
	t.nodeUSBObs = nodeusbobs.StartObserver(t.ctx, t.Framework, t.NodeUSBDevice.Name)
	usbDeviceObs := usbdevobs.StartObserver(t.ctx, t.Framework, t.NodeUSBDevice.Name, t.Framework.Namespace().Name)

	nodeUSBCopy := t.NodeUSBDevice.DeepCopy()
	nodeUSBCopy.Spec.AssignedNamespace = t.Framework.Namespace().Name
	_, err := t.Framework.VirtClient().NodeUSBDevices().Update(t.ctx, nodeUSBCopy, metav1.UpdateOptions{})
	Expect(err).NotTo(HaveOccurred())

	err = usbDeviceObs.WaitFor(usbdevobs.Exist(), framework.MaxTimeout)
	Expect(err).NotTo(HaveOccurred(),
		"USBDevice %s/%s should be created for the assigned NodeUSBDevice", t.Framework.Namespace().Name, t.NodeUSBDevice.Name)
}

func (t *VMUSBTest) waitForNodeUSBAttached(status metav1.ConditionStatus) {
	GinkgoHelper()
	err := t.nodeUSBObs.WaitFor(nodeusbobs.HaveAttachedCondition(status), framework.MaxTimeout)
	Expect(err).NotTo(HaveOccurred(),
		"NodeUSBDevice %s Attached condition should become %s", t.NodeUSBDevice.Name, status)
}

func (t *VMUSBTest) waitForVMUSBReady(message string) {
	GinkgoHelper()

	err := t.vmObs.WaitFor(haveUSBDeviceReady(t.NodeUSBDevice.Name), framework.MaxTimeout)
	Expect(err).NotTo(HaveOccurred(), fmt.Sprintf(message, t.NodeUSBDevice.Name))
}

// haveUSBDeviceReady reports the named USB device is attached to the VM and
// ready.
func haveUSBDeviceReady(name string) vmobs.Predicate {
	return func(vm *v1alpha2.VirtualMachine) (bool, error) {
		for _, dev := range vm.Status.USBDevices {
			if dev.Name == name && dev.Attached && dev.Ready {
				return true, nil
			}
		}
		return false, nil
	}
}

func (t *VMUSBTest) writeUSBTestData() {
	result, err := t.Framework.SSHCommand(t.VM.Name, t.VM.Namespace, fmt.Sprintf("echo \"%s\" | tee %s && sync && umount /mnt/usb", t.testContent, t.testFile), framework.WithSSHUser("root"))
	Expect(err).NotTo(HaveOccurred())
	Expect(result).To(ContainSubstring(t.testContent))
}

func (t *VMUSBTest) verifyUSBTestData() {
	result, err := t.Framework.SSHCommand(t.VM.Name, t.VM.Namespace, fmt.Sprintf("cat %s", t.testFile), framework.WithSSHUser("root"))
	Expect(err).NotTo(HaveOccurred())
	Expect(result).To(ContainSubstring(t.testContent))
}

func (t *VMUSBTest) findUSBMountDevice() string {
	serial := t.NodeUSBDevice.Status.Attributes.Serial
	Expect(serial).NotTo(BeEmpty(), "USB device serial must be set")

	findDeviceCmd := fmt.Sprintf(`
		usb_serial=%q
		: > /tmp/usb-mount.err
		for serial_file in /sys/bus/usb/devices/*/serial; do
			if [ -f "$serial_file" ] && [ "$(cat "$serial_file")" = "$usb_serial" ]; then
				usb_present=1
				break
			fi
		done
		[ -n "$usb_present" ] || { echo "USB device with serial $usb_serial not found" >/tmp/usb-mount.err; exit 1; }

		for host in /sys/class/scsi_host/host*; do
			echo "- - -" > "$host/scan" || true
		done

		for dev in /dev/sd*; do
			[ -b "$dev" ] || continue
			if lsblk -dno TRAN,RM "$dev" 2>/dev/null | grep -Eq '^usb[[:space:]]+1$'; then
				mount_device="$dev"
				break
			fi
		done
		[ -n "$mount_device" ] || {
			echo "USB block device not found for serial $usb_serial" >>/tmp/usb-mount.err
			lsblk -a -o NAME,PATH,TYPE,TRAN,RM,SERIAL,MODEL >>/tmp/usb-mount.err 2>&1 || true
			exit 1
		}

		echo "$mount_device"
	`, serial)

	var mountDevice string

	// EXCEPTION: guest-side wait (USB rescan and device discovery over SSH),
	// not a Kubernetes resource — nothing to observe via an Observer.
	eventually.Until(func() error {
		result, err := t.Framework.SSHCommand(
			t.VM.Name,
			t.VM.Namespace,
			findDeviceCmd,
			framework.WithSSHUser("root"),
			framework.WithSSHTimeout(framework.ShortTimeout),
		)
		if err != nil {
			return err
		}
		mountDevice = strings.TrimSpace(result)
		if mountDevice == "" {
			return fmt.Errorf("empty mount device output")
		}

		return nil
	}, framework.MiddleTimeout, eventually.WithExplanation(t.usbDiagnostics))

	return mountDevice
}

func (t *VMUSBTest) formatUSBDevice(mountDevice string) {
	formatCmd := fmt.Sprintf(`
		: > /tmp/usb-mount.err
		mkfs.vfat -I %q 2>>/tmp/usb-mount.err
	`, mountDevice)

	// EXCEPTION: guest-side action retried over SSH (mkfs on the USB stick),
	// not a Kubernetes resource — nothing to observe via an Observer.
	eventually.Until(func() error {
		_, err := t.Framework.SSHCommand(
			t.VM.Name,
			t.VM.Namespace,
			formatCmd,
			framework.WithSSHUser("root"),
			framework.WithSSHTimeout(framework.ShortTimeout),
		)
		return err
	}, framework.MiddleTimeout, eventually.WithExplanation(t.usbDiagnostics))
}

func (t *VMUSBTest) mountUSBDevice(mountDevice string) {
	mountCmd := fmt.Sprintf(`
		: > /tmp/usb-mount.err
		mkdir -p /mnt/usb
		if mountpoint -q /mnt/usb; then
			umount /mnt/usb || true
		fi
		mount %q /mnt/usb 2>>/tmp/usb-mount.err
		ls -la /mnt/usb
	`, mountDevice)

	// EXCEPTION: guest-side action retried over SSH (mounting the USB stick),
	// not a Kubernetes resource — nothing to observe via an Observer.
	eventually.Until(func() error {
		_, err := t.Framework.SSHCommand(
			t.VM.Name,
			t.VM.Namespace,
			mountCmd,
			framework.WithSSHUser("root"),
			framework.WithSSHTimeout(framework.MiddleTimeout),
		)
		return err
	}, framework.LongTimeout, eventually.WithExplanation(t.usbDiagnostics))
}

func (t *VMUSBTest) usbDiagnostics() string {
	diagnosticsCmd := `
		echo "mount error:" && cat /tmp/usb-mount.err 2>/dev/null || true
		echo "mount:" && mount || true
		echo "usb serials:" && for serial_file in /sys/bus/usb/devices/*/serial; do [ -f "$serial_file" ] && echo "$serial_file=$(cat "$serial_file")"; done || true
		echo "usb sysfs:" && find /sys/bus/usb/devices -maxdepth 3 -print || true
		echo "lsblk:" && lsblk -a -o NAME,PATH,TYPE,TRAN,RM,SERIAL,MODEL || true
		echo "disks:" && for dev in /dev/sd*; do [ -b "$dev" ] && echo "== $dev ==" && lsblk -dno NAME,PATH,TRAN,RM,SERIAL,MODEL "$dev"; done || true
		echo "lsusb:" && lsusb || true
		echo "fstype:" && blkid /dev/sd* || true
		echo "dmesg:" && dmesg | tail -n 100 || true
	`

	result, err := t.Framework.SSHCommand(t.VM.Name, t.VM.Namespace, diagnosticsCmd, framework.WithSSHUser("root"), framework.WithSSHTimeout(framework.MiddleTimeout))
	if err != nil {
		return fmt.Sprintf("failed to collect USB diagnostics: %v", err)
	}

	return result
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

	namespace := t.Framework.Namespace().Name
	err = observer.WaitForDeleted(t.ctx, t.Framework.VirtClient().USBDevices(namespace), t.NodeUSBDevice.Name, namespace, framework.MaxTimeout,
		func(ctx context.Context) (bool, error) {
			_, err := t.Framework.VirtClient().USBDevices(namespace).Get(ctx, t.NodeUSBDevice.Name, metav1.GetOptions{})
			if k8serrors.IsNotFound(err) {
				return true, nil
			}
			return false, err
		})
	Expect(err).NotTo(HaveOccurred(), "USBDevice %s/%s should be removed after unassignment", namespace, t.NodeUSBDevice.Name)
}
