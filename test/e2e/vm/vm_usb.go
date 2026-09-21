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
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"

	vdbuilder "github.com/deckhouse/virtualization-controller/pkg/builder/vd"
	vmbuilder "github.com/deckhouse/virtualization-controller/pkg/builder/vm"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/vmcondition"
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

const (
	// dummyHCDVendorID and dummyHCDProductID identify the dummy_hcd virtual USB
	// sticks the suite is allowed to consume; real devices on the nodes are
	// never touched.
	dummyHCDVendorID  = "1d6b"
	dummyHCDProductID = "0104"
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

	It("should write data to USB device and preserve after migration", func() {
		By("Environment preparation", func() {
			t.assignFreeNodeUSB()
			t.GenerateEnvironmentResources(ctx, true)
			err := f.CreateWithDeferredDeletion(ctx, t.VD)
			Expect(err).NotTo(HaveOccurred())
		})

		By("Verifying NodeUSBDevice is not attached before VM attachment", func() {
			t.waitForNodeUSBAttached(metav1.ConditionFalse)
		})

		By("Creating VM with USB device", func() {
			t.createVMAndWaitForGuest()
		})

		By("Waiting for USB device to be attached and ready", func() {
			t.waitForVMUSBReady("USB device %s not attached or not ready")
		})

		By("Verifying NodeUSBDevice is attached", func() {
			t.waitForNodeUSBAttached(metav1.ConditionTrue)
		})

		By("Mounting USB device", func() {
			t.formatAndMountUSBDevice()
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
			t.mountUSBDevice(t.findUSBMountDevice())
		})

		By("Verifying data persists after migration", func() {
			t.verifyUSBTestData()
		})
	})

	It("should hotplug and unplug a USB device on a running VM without a restart", func() {
		By("Environment preparation", func() {
			t.assignFreeNodeUSB()
			t.GenerateEnvironmentResources(ctx, false)
			err := f.CreateWithDeferredDeletion(ctx, t.VD)
			Expect(err).NotTo(HaveOccurred())
		})

		By("Creating VM without USB devices", func() {
			t.createVMAndWaitForGuest()
		})

		podBeforeHotplug := t.activePodName()

		By("Attaching USB device to the running VM", func() {
			t.setVMUSBDevices(t.NodeUSBDevice.Name)
			t.waitForVMUSBReady("USB device %s not hotplugged")
			t.waitForNodeUSBAttached(metav1.ConditionTrue)
		})

		By("Writing data to the hotplugged USB device", func() {
			t.formatAndMountUSBDevice()
			t.writeUSBTestData()
		})

		By("Detaching USB device from the running VM", func() {
			t.setVMUSBDevices()
			err := t.vmObs.WaitFor(haveNoUSBDevice(t.NodeUSBDevice.Name), framework.MaxTimeout)
			Expect(err).NotTo(HaveOccurred(), "USB device %s should disappear from the VM status", t.NodeUSBDevice.Name)
			t.waitForNodeUSBAttached(metav1.ConditionFalse)
		})

		By("Verifying the guest no longer sees the USB device", func() {
			t.waitForUSBDeviceGoneFromGuest()
		})

		By("Verifying the VM was not restarted", func() {
			Expect(t.activePodName()).To(Equal(podBeforeHotplug), "hotplug must not recreate the virt-launcher pod")
		})
	})

	It("should reattach the USB device after a VM restart and keep the data", func() {
		By("Environment preparation", func() {
			t.assignFreeNodeUSB()
			t.GenerateEnvironmentResources(ctx, true)
			err := f.CreateWithDeferredDeletion(ctx, t.VD)
			Expect(err).NotTo(HaveOccurred())
		})

		By("Creating VM with USB device", func() {
			t.createVMAndWaitForGuest()
			t.waitForVMUSBReady("USB device %s not attached or not ready")
		})

		By("Writing data to USB device", func() {
			t.formatAndMountUSBDevice()
			t.writeUSBTestData()
		})

		By("Restarting VM", func() {
			runningSince := t.runningConditionTransitionTime()
			util.RebootVirtualMachineByVMOP(f, t.VM)
			err := t.vmObs.WaitFor(vmobs.BeRebootedAfter(runningSince), framework.LongTimeout)
			Expect(err).NotTo(HaveOccurred())
			err = t.vmObs.WaitFor(vmobs.BeAgentReady(), framework.LongTimeout)
			Expect(err).NotTo(HaveOccurred())
			eventually.SSHReadyAsRoot(f, t.VM, framework.MiddleTimeout)
		})

		By("Waiting for USB device to be attached after restart", func() {
			t.waitForVMUSBReady("USB device %s not attached after restart")
			t.waitForNodeUSBAttached(metav1.ConditionTrue)
		})

		By("Verifying data persists after restart", func() {
			t.mountUSBDevice(t.findUSBMountDevice())
			t.verifyUSBTestData()
		})
	})

	It("should detach the USB device when its namespace assignment is revoked and reattach it once assigned again", func() {
		By("Environment preparation", func() {
			t.assignFreeNodeUSB()
			t.GenerateEnvironmentResources(ctx, true)
			err := f.CreateWithDeferredDeletion(ctx, t.VD)
			Expect(err).NotTo(HaveOccurred())
		})

		By("Creating VM with USB device", func() {
			t.createVMAndWaitForGuest()
			t.waitForVMUSBReady("USB device %s not attached or not ready")
		})

		By("Writing data to USB device", func() {
			t.formatAndMountUSBDevice()
			t.writeUSBTestData()
		})

		podBeforeRevoke := t.activePodName()

		By("Revoking the namespace assignment", func() {
			t.setNodeUSBAssignedNamespace("")
			t.waitForUSBDeviceDeleted()
			err := t.vmObs.WaitFor(haveUSBDeviceDetached(t.NodeUSBDevice.Name), framework.MaxTimeout)
			Expect(err).NotTo(HaveOccurred(), "USB device %s should be reported as detached", t.NodeUSBDevice.Name)
			t.waitForNodeUSBAttached(metav1.ConditionFalse)
		})

		By("Verifying the guest no longer sees the USB device and the VM keeps running", func() {
			t.waitForUSBDeviceGoneFromGuest()
			Expect(t.activePodName()).To(Equal(podBeforeRevoke), "revoking the device must not restart the VM")
		})

		By("Assigning the namespace again", func() {
			t.setNodeUSBAssignedNamespace(f.Namespace().Name)
			t.waitForVMUSBReady("USB device %s not reattached after the namespace was assigned again")
			t.waitForNodeUSBAttached(metav1.ConditionTrue)
		})

		By("Verifying data persists after reattachment", func() {
			t.mountUSBDevice(t.findUSBMountDevice())
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

// GenerateEnvironmentResources builds the root disk and the VM. The USB device
// is referenced from the VM spec only when withUSB is set; otherwise the spec
// attaches it later to exercise the hotplug path.
func (t *VMUSBTest) GenerateEnvironmentResources(ctx context.Context, withUSB bool) {
	Expect(t.NodeUSBDevice).NotTo(BeNil(), "a NodeUSBDevice must be assigned first")

	// The custom image bakes in the USB drivers (usb-storage/uas are
	// compiled into the monolithic kernel), mkfs.vfat and lsblk; device nodes
	// appear via devtmpfs, so the guest needs neither udev nor sudo — commands
	// run as root over the baked SSH key.
	t.VD = object.NewVDFromCVI("vd-usb-test", t.Framework.Namespace().Name, object.PrecreatedCVICustomBIOS, vdbuilder.WithSize(ptr.To(resource.MustParse(vdCustomImageSize))))

	opts := []vmbuilder.Option{
		vmbuilder.WithName("vm-usb-test"),
		vmbuilder.WithNamespace(t.Framework.Namespace().Name),
		vmbuilder.WithCPU(1, ptr.To(object.CustomImageVMCoreFraction)),
		vmbuilder.WithMemory(resource.MustParse(object.CustomImageVMMemory)),
		vmbuilder.WithVirtualMachineClass(object.DefaultVMClass),
		// The custom image has no cloud-init; the guest agent is baked in.
		vmbuilder.WithLiveMigrationPolicy(v1alpha2.AlwaysSafeMigrationPolicy),
		vmbuilder.WithBlockDeviceRefs(v1alpha2.BlockDeviceSpecRef{Kind: v1alpha2.DiskDevice, Name: t.VD.Name}),
	}
	if withUSB {
		opts = append(opts, vmbuilder.WithUSBDevices([]v1alpha2.USBDeviceSpecRef{{Name: t.NodeUSBDevice.Name}}))
	}
	t.VM = vmbuilder.New(opts...)
}

// assignFreeNodeUSB picks a random unassigned dummy_hcd device and assigns it
// to the test namespace. Specs run in parallel and race for the same pool, so
// a lost update (another process assigned the same device first) just picks
// again instead of failing the spec.
func (t *VMUSBTest) assignFreeNodeUSB() {
	GinkgoHelper()

	virtClient := t.Framework.VirtClient()
	namespace := t.Framework.Namespace().Name

	eventually.Until(func() error {
		nodeUSBList, err := virtClient.NodeUSBDevices().List(t.ctx, metav1.ListOptions{})
		if err != nil {
			return err
		}

		var freeUSBs []*v1alpha2.NodeUSBDevice
		for i := range nodeUSBList.Items {
			item := &nodeUSBList.Items[i]
			if item.Status.Attributes.VendorID == dummyHCDVendorID && item.Status.Attributes.ProductID == dummyHCDProductID && item.Spec.AssignedNamespace == "" && item.Status.NodeName != "" {
				freeUSBs = append(freeUSBs, item)
			}
		}
		if len(freeUSBs) == 0 {
			return fmt.Errorf("no free dummy_hcd USB devices available")
		}

		candidate := freeUSBs[rand.New(rand.NewSource(time.Now().UnixNano())).Intn(len(freeUSBs))]
		candidate.Spec.AssignedNamespace = namespace
		assigned, err := virtClient.NodeUSBDevices().Update(t.ctx, candidate, metav1.UpdateOptions{})
		if err != nil {
			return fmt.Errorf("assign NodeUSBDevice %s: %w", candidate.Name, err)
		}

		t.NodeUSBDevice = assigned
		return nil
	}, framework.MiddleTimeout)

	GinkgoWriter.Println("Assigned USB device:", t.NodeUSBDevice.Name, "on node", t.NodeUSBDevice.Status.NodeName)

	// A watch without a resourceVersion replays the current state first, so
	// observers started after the assignment still see the USBDevice created
	// by the controller.
	t.nodeUSBObs = nodeusbobs.StartObserver(t.ctx, t.Framework, t.NodeUSBDevice.Name)
	t.waitForUSBDeviceExists()
}

// setNodeUSBAssignedNamespace updates the assigned namespace of the test's
// NodeUSBDevice, retrying on conflicts with the controller's own updates.
func (t *VMUSBTest) setNodeUSBAssignedNamespace(namespace string) {
	GinkgoHelper()

	virtClient := t.Framework.VirtClient()
	eventually.Until(func() error {
		nodeUSBDevice, err := virtClient.NodeUSBDevices().Get(t.ctx, t.NodeUSBDevice.Name, metav1.GetOptions{})
		if err != nil {
			return err
		}
		nodeUSBDevice.Spec.AssignedNamespace = namespace
		_, err = virtClient.NodeUSBDevices().Update(t.ctx, nodeUSBDevice, metav1.UpdateOptions{})
		return err
	}, framework.ShortTimeout)

	if namespace != "" {
		t.waitForUSBDeviceExists()
	}
}

func (t *VMUSBTest) waitForUSBDeviceExists() {
	GinkgoHelper()

	namespace := t.Framework.Namespace().Name
	usbDeviceObs := usbdevobs.StartObserver(t.ctx, t.Framework, t.NodeUSBDevice.Name, namespace)
	err := usbDeviceObs.WaitFor(usbdevobs.Exist(), framework.MaxTimeout)
	Expect(err).NotTo(HaveOccurred(),
		"USBDevice %s/%s should be created for the assigned NodeUSBDevice", namespace, t.NodeUSBDevice.Name)
}

func (t *VMUSBTest) waitForUSBDeviceDeleted() {
	GinkgoHelper()

	namespace := t.Framework.Namespace().Name
	usbDevices := t.Framework.VirtClient().USBDevices(namespace)
	err := observer.WaitForDeleted(t.ctx, usbDevices, t.NodeUSBDevice.Name, namespace, framework.MaxTimeout,
		func(ctx context.Context) (bool, error) {
			_, err := usbDevices.Get(ctx, t.NodeUSBDevice.Name, metav1.GetOptions{})
			if k8serrors.IsNotFound(err) {
				return true, nil
			}
			return false, err
		})
	Expect(err).NotTo(HaveOccurred(), "USBDevice %s/%s should be removed after unassignment", namespace, t.NodeUSBDevice.Name)
}

func (t *VMUSBTest) createVMAndWaitForGuest() {
	GinkgoHelper()

	err := t.Framework.CreateWithDeferredDeletion(t.ctx, t.VM)
	Expect(err).NotTo(HaveOccurred())

	t.vmObs = vmobs.StartObserver(t.ctx, t.Framework, t.VM)
	t.vmObs.Never(vmobs.BeFailed())
	err = t.vmObs.WaitFor(vmobs.BeRunning(), framework.LongTimeout)
	Expect(err).NotTo(HaveOccurred())
	// Running only means qemu has started; wait for the guest agent so the
	// guest is fully booted before the short SSH readiness window below.
	err = t.vmObs.WaitFor(vmobs.BeAgentReady(), framework.LongTimeout)
	Expect(err).NotTo(HaveOccurred())
	// mkfs.vfat and lsblk are baked into the custom image, so there is no
	// cloud-init package installation to wait for.
	eventually.SSHReadyAsRoot(t.Framework, t.VM, framework.MiddleTimeout)
}

// setVMUSBDevices replaces spec.usbDevices of the running VM with the given
// devices; an empty list detaches everything.
func (t *VMUSBTest) setVMUSBDevices(names ...string) {
	GinkgoHelper()

	refs := make([]v1alpha2.USBDeviceSpecRef, 0, len(names))
	for _, name := range names {
		refs = append(refs, v1alpha2.USBDeviceSpecRef{Name: name})
	}
	updateVMSpec(t.ctx, t.Framework, t.VM.Name, func(vm *v1alpha2.VirtualMachine) {
		vm.Spec.USBDevices = refs
	})
}

func (t *VMUSBTest) activePodName() string {
	GinkgoHelper()

	vm := getVirtualMachine(t.ctx, t.Framework, t.VM.Name)
	podName, err := util.GetActivePodName(vm)
	Expect(err).NotTo(HaveOccurred())
	return podName
}

func (t *VMUSBTest) runningConditionTransitionTime() time.Time {
	GinkgoHelper()

	vm := getVirtualMachine(t.ctx, t.Framework, t.VM.Name)
	cond := meta.FindStatusCondition(vm.Status.Conditions, vmcondition.TypeRunning.String())
	Expect(cond).NotTo(BeNil(), "VirtualMachine %s/%s has no Running condition", vm.Namespace, vm.Name)
	return cond.LastTransitionTime.Time
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

// haveUSBDeviceDetached reports the named USB device is still referenced by
// the VM but no longer attached.
func haveUSBDeviceDetached(name string) vmobs.Predicate {
	return func(vm *v1alpha2.VirtualMachine) (bool, error) {
		for _, dev := range vm.Status.USBDevices {
			if dev.Name == name {
				return !dev.Attached, nil
			}
		}
		return false, nil
	}
}

// haveNoUSBDevice reports the named USB device is gone from the VM status.
func haveNoUSBDevice(name string) vmobs.Predicate {
	return func(vm *v1alpha2.VirtualMachine) (bool, error) {
		for _, dev := range vm.Status.USBDevices {
			if dev.Name == name {
				return false, nil
			}
		}
		return true, nil
	}
}

// formatAndMountUSBDevice locates the stick in the guest by its serial,
// formats it and mounts it at /mnt/usb.
func (t *VMUSBTest) formatAndMountUSBDevice() {
	GinkgoHelper()

	mountDevice := t.findUSBMountDevice()
	GinkgoWriter.Println("Found USB device:", mountDevice)
	t.formatUSBDevice(mountDevice)
	t.mountUSBDevice(mountDevice)
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

// usbSerialPresentFn renders a shell function usb_present that succeeds when
// a USB device with the test's serial is visible to the guest.
func (t *VMUSBTest) usbSerialPresentFn() string {
	serial := t.NodeUSBDevice.Status.Attributes.Serial
	Expect(serial).NotTo(BeEmpty(), "USB device serial must be set")

	return fmt.Sprintf(`
		usb_present() {
			for serial_file in /sys/bus/usb/devices/*/serial; do
				if [ -f "$serial_file" ] && [ "$(cat "$serial_file")" = %q ]; then
					return 0
				fi
			done
			return 1
		}
	`, serial)
}

func (t *VMUSBTest) findUSBMountDevice() string {
	GinkgoHelper()

	serial := t.NodeUSBDevice.Status.Attributes.Serial
	findDeviceCmd := t.usbSerialPresentFn() + fmt.Sprintf(`
		: > /tmp/usb-mount.err
		usb_present || { echo "USB device with serial %s not found" >/tmp/usb-mount.err; exit 1; }

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
			echo "USB block device not found for serial %s" >>/tmp/usb-mount.err
			lsblk -a -o NAME,PATH,TYPE,TRAN,RM,SERIAL,MODEL >>/tmp/usb-mount.err 2>&1 || true
			exit 1
		}

		echo "$mount_device"
	`, serial, serial)

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

// waitForUSBDeviceGoneFromGuest waits until the guest no longer lists a USB
// device with the test's serial.
func (t *VMUSBTest) waitForUSBDeviceGoneFromGuest() {
	GinkgoHelper()

	// EXCEPTION: guest-side wait (device removal seen over SSH), not a
	// Kubernetes resource — nothing to observe via an Observer.
	eventually.Until(func() error {
		_, err := t.Framework.SSHCommand(
			t.VM.Name,
			t.VM.Namespace,
			t.usbSerialPresentFn()+"\n\t\tusb_present",
			framework.WithSSHUser("root"),
			framework.WithSSHTimeout(framework.ShortTimeout),
		)
		if err == nil {
			return fmt.Errorf("USB device with serial %s is still visible in the guest", t.NodeUSBDevice.Status.Attributes.Serial)
		}
		return nil
	}, framework.MiddleTimeout, eventually.WithExplanation(t.usbDiagnostics))
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

	t.waitForUSBDeviceDeleted()
}
