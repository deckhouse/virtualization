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

package kvbuilder

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"k8s.io/utils/ptr"
	virtv1 "kubevirt.io/api/core/v1"

	"github.com/deckhouse/virtualization/api/core/v1alpha2"
)

// The full device matrix: every osType against both paravirtualization modes,
// for a static and for a hot-plugged device. Written out as a table rather than
// derived from the presets, so a preset edit that moves a bus fails here instead
// of agreeing with itself. The ide bus and the rtl8139 adapter belong to Legacy
// alone — no other osType may produce them in any cell.
//
//	osType   paravirt | static disk | static cdrom | hotplug disk | hotplug cdrom | adapter
//	---------------------------------------------------------------------------------------
//	<unset>  true     | scsi        | scsi         | scsi         | scsi          | virtio
//	<unset>  false    | sata        | sata         | scsi         | scsi          | e1000
//	Generic  true     | scsi        | scsi         | scsi         | scsi          | virtio
//	Generic  false    | sata        | sata         | scsi         | scsi          | e1000
//	Windows  true     | scsi        | scsi         | scsi         | scsi          | virtio
//	Windows  false    | sata        | sata         | scsi         | scsi          | e1000
//	Legacy   true     | virtio      | ide          | scsi         | scsi          | virtio
//	Legacy   false    | ide         | ide          | ide          | ide           | rtl8139
//
// Not covered here, on purpose: applyBlockDeviceRefs, which decides whether a disk
// is hot-plugged in the first place — this matrix is told the answer — and the bus
// of the provisioning disk, which SetProvisioning routes through SetDisk.
var _ = Describe("device presets", func() {
	const (
		scsi   = virtv1.DiskBusSCSI
		sata   = virtv1.DiskBusSATA
		virtio = virtv1.DiskBusVirtio
		ide    = virtv1.DiskBusIDE
	)

	type buses struct {
		disk  virtv1.DiskBus
		cdrom virtv1.DiskBus
	}

	busOf := func(b *KVVM, name string) virtv1.DiskBus {
		for _, d := range b.Resource.Spec.Template.Spec.Domain.Devices.Disks {
			if d.Name != name {
				continue
			}
			if d.CDRom != nil {
				return d.CDRom.Bus
			}
			if d.Disk != nil {
				return d.Disk.Bus
			}
		}
		return ""
	}

	DescribeTable("assigns the buses and the network adapter",
		func(osType v1alpha2.OsType, paravirt bool, static, hotplug buses, nic string) {
			for _, hotplugged := range []bool{false, true} {
				because := func(what string) string {
					return what + ", osType=" + string(osType) + ", hotplug=" + map[bool]string{true: "true", false: "false"}[hotplugged]
				}

				b := NewEmptyKVVM(namespacedName("test-vm", "test-ns"), KVVMOptions{
					EnableParavirtualization: paravirt,
					OsType:                   osType,
				})
				Expect(b.SetDisk("disk", SetDiskOptions{ContainerDisk: ptr.To("img"), IsHotplugged: hotplugged})).To(Succeed())
				Expect(b.SetDisk("cdrom", SetDiskOptions{IsCdrom: true, ContainerDisk: ptr.To("img"), IsHotplugged: hotplugged})).To(Succeed())

				want := static
				if hotplugged {
					want = hotplug
				}
				Expect(busOf(b, "disk")).To(Equal(want.disk), because("disk bus"))
				Expect(busOf(b, "cdrom")).To(Equal(want.cdrom), because("cdrom bus"))

				// The adapter model comes from the same preset as the buses, so it
				// belongs in the same matrix. It does not depend on hotplug.
				b.SetNetworkInterface("default", "", 0)
				Expect(b.Resource.Spec.Template.Spec.Domain.Devices.Interfaces[0].Model).To(Equal(nic), because("network adapter"))

				if osType != v1alpha2.LegacyOs {
					Expect(busOf(b, "disk")).NotTo(Equal(ide), because("the ide bus leaked from the Legacy osType"))
					Expect(busOf(b, "cdrom")).NotTo(Equal(ide), because("the ide bus leaked from the Legacy osType"))
					Expect(b.Resource.Spec.Template.Spec.Domain.Devices.Interfaces[0].Model).NotTo(Equal(nicModelRTL8139), because("the rtl8139 adapter leaked from the Legacy osType"))
				}
			}
		},
		// An unset osType is what KVVMOptions carries before the VM spec is applied;
		// it must behave exactly like Generic and never see ide.
		Entry("unset osType, paravirtualized", v1alpha2.OsType(""), true, buses{scsi, scsi}, buses{scsi, scsi}, nicModelVirtio),
		Entry("unset osType, emulated", v1alpha2.OsType(""), false, buses{sata, sata}, buses{scsi, scsi}, nicModelE1000),

		Entry("Generic, paravirtualized", v1alpha2.GenericOs, true, buses{scsi, scsi}, buses{scsi, scsi}, nicModelVirtio),
		Entry("Generic, emulated", v1alpha2.GenericOs, false, buses{sata, sata}, buses{scsi, scsi}, nicModelE1000),

		Entry("Windows, paravirtualized", v1alpha2.Windows, true, buses{scsi, scsi}, buses{scsi, scsi}, nicModelVirtio),
		Entry("Windows, emulated", v1alpha2.Windows, false, buses{sata, sata}, buses{scsi, scsi}, nicModelE1000),

		// virtio-scsi is not an option for Legacy: the archived virtio-win that still
		// carries these guests has no virtio-scsi build older than Windows 7. It does
		// carry virtio-blk for Windows XP and Server 2003. The cdrom stays on ide
		// because virtio-blk has no cdrom. A hot-plugged volume follows the common
		// rule and lands on scsi: whoever enabled paravirtualization stated the guest
		// has virtio drivers.
		Entry("Legacy, paravirtualized", v1alpha2.LegacyOs, true, buses{virtio, ide}, buses{scsi, scsi}, nicModelVirtio),
		// With paravirtualization off the ide bus wins even for a volume marked
		// hotpluggable: that bus cannot be hot-plugged, so the scsi fallback would
		// silently move the disk onto a controller the guest has no driver for. The
		// adapter is rtl8139, the one these guests have a driver for; e1000 is already
		// too new for them.
		Entry("Legacy, emulated", v1alpha2.LegacyOs, false, buses{ide, ide}, buses{ide, ide}, nicModelRTL8139),
	)
})
