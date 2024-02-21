package kvbuilder

import (
	"fmt"

	virtv1 "kubevirt.io/api/core/v1"
)

type DeviceOptions struct {
	EnableParavirtualization bool

	DiskBus        virtv1.DiskBus
	CdromBus       virtv1.DiskBus
	InterfaceModel string
}

type DeviceOptionsList []DeviceOptions

func (l DeviceOptionsList) Find(enableParavirtualization bool) DeviceOptions {
	for _, opts := range l {
		if opts.EnableParavirtualization == enableParavirtualization {
			return opts
		}
	}
	panic(fmt.Sprintf("cannot find preset for enableParavirtualization=%v", enableParavirtualization))
}

var DeviceOptionsPresets DeviceOptionsList = []DeviceOptions{
	{
		EnableParavirtualization: true,
		DiskBus:                  virtv1.DiskBusSCSI,
		CdromBus:                 virtv1.DiskBusSATA,
		InterfaceModel:           "virtio",
	},
	{
		EnableParavirtualization: false,
		DiskBus:                  virtv1.DiskBusSATA,
		CdromBus:                 virtv1.DiskBusSATA,
		InterfaceModel:           "e1000",
	},
}
