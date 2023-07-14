package kvbuilder

import (
	"fmt"

	virtv1 "kubevirt.io/api/core/v1"

	virtv2 "github.com/deckhouse/virtualization-controller/api/v2alpha1"
)

type DeviceOptions struct {
	EnableParavirtualization bool
	OsType                   virtv2.OsType

	DiskBus             virtv1.DiskBus
	CdromBus            virtv1.DiskBus
	InterfaceModel      string
	EnableTabletPointer bool
	FixTimers           bool
	EnableTPM           bool
}

type DeviceOptionsList []DeviceOptions

func (l DeviceOptionsList) Find(enableParavirtualization bool, osType virtv2.OsType) DeviceOptions {
	for _, opts := range l {
		if opts.EnableParavirtualization == enableParavirtualization && opts.OsType == osType {
			return opts
		}
	}
	panic(fmt.Sprintf("cannot find preset for enableParavirtualization=%v osType=%v", enableParavirtualization, osType))
}

var DeviceOptionsPresets DeviceOptionsList = []DeviceOptions{
	{
		EnableParavirtualization: true,
		OsType:                   virtv2.Windows,
		DiskBus:                  virtv1.DiskBusSCSI,
		CdromBus:                 virtv1.DiskBusSATA,
		InterfaceModel:           "virtio",
		EnableTabletPointer:      true,
		EnableTPM:                true,
	},
	{
		EnableParavirtualization: true,
		OsType:                   virtv2.LegacyWindows,
		DiskBus:                  virtv1.DiskBusSCSI,
		CdromBus:                 virtv1.DiskBusSATA,
		InterfaceModel:           "virtio",
		EnableTabletPointer:      true,
		EnableTPM:                true,
		FixTimers:                true,
	},
	{
		EnableParavirtualization: true,
		OsType:                   virtv2.GenericOs,
		DiskBus:                  virtv1.DiskBusSCSI,
		CdromBus:                 virtv1.DiskBusSATA,
		InterfaceModel:           "virtio",
		EnableTPM:                true,
	},
	{
		EnableParavirtualization: false,
		OsType:                   virtv2.Windows,
		DiskBus:                  virtv1.DiskBusSATA,
		CdromBus:                 virtv1.DiskBusSATA,
		InterfaceModel:           "e1000",
		EnableTabletPointer:      true,
		EnableTPM:                true,
	},
	{
		EnableParavirtualization: false,
		OsType:                   virtv2.LegacyWindows,
		DiskBus:                  virtv1.DiskBusSATA,
		CdromBus:                 virtv1.DiskBusSATA,
		InterfaceModel:           "e1000",
		EnableTabletPointer:      true,
		EnableTPM:                true,
		FixTimers:                true,
	},
	{
		EnableParavirtualization: false,
		OsType:                   virtv2.GenericOs,
		DiskBus:                  virtv1.DiskBusSATA,
		CdromBus:                 virtv1.DiskBusSATA,
		InterfaceModel:           "e1000",
		EnableTPM:                true,
	},
}
