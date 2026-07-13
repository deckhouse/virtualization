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

// Buses returns the disk and cdrom bus for the preset. Hot-plugged devices are
// attached via AddVolume, which always uses the scsi bus regardless of the
// paravirtualization preset. Keeping them on scsi stops a VM with
// enableParavirtualization=false (sata preset) from rewriting an already
// attached device to sata, which is invalid for a hot-plugged disk. Static
// disks follow the preset and change buses on the restart a paravirtualization
// flip already requires.
func (o DeviceOptions) Buses(isHotplugged bool) (diskBus, cdromBus virtv1.DiskBus) {
	if isHotplugged {
		return virtv1.DiskBusSCSI, virtv1.DiskBusSCSI
	}
	return o.DiskBus, o.CdromBus
}

var DeviceOptionsPresets DeviceOptionsList = []DeviceOptions{
	{
		EnableParavirtualization: true,
		DiskBus:                  virtv1.DiskBusSCSI,
		CdromBus:                 virtv1.DiskBusSCSI,
		InterfaceModel:           "virtio",
	},
	{
		EnableParavirtualization: false,
		DiskBus:                  virtv1.DiskBusSATA,
		CdromBus:                 virtv1.DiskBusSATA,
		InterfaceModel:           "e1000",
	},
}
