/*
Copyright 2024 Flant JSC

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
		// CD-ROMs use the USB bus (usb-storage) so guests without a virtio-scsi
		// driver (e.g. Windows WinPE) can enumerate them via the inbox USB Mass
		// Storage driver. The xhci controller is always present (tablet input).
		CdromBus:       virtv1.DiskBusUSB,
		InterfaceModel: "virtio",
	},
	{
		EnableParavirtualization: false,
		DiskBus:                  virtv1.DiskBusSATA,
		CdromBus:                 virtv1.DiskBusUSB,
		InterfaceModel:           "e1000",
	},
}
