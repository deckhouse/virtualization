/*
Copyright 2025 Flant JSC

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

package libusb

type USBDeviceSpeed uint32

const (
	USBDeviceSpeedUnknown   USBDeviceSpeed = iota // enumerating
	USBDeviceSpeedLow                             // usb 1.1
	USBDeviceSpeedFull                            // usb 1.1
	USBDeviceSpeedHigh                            // usb 2.0
	USBDeviceSpeedSuper                           // usb 3.0
	USBDeviceSpeedSuperPlus                       // usb 3.1
)

func (s USBDeviceSpeed) Speeds() []uint32 {
	switch s {
	case USBDeviceSpeedLow:
		return []uint32{1}
	case USBDeviceSpeedFull:
		return []uint32{12, 15}
	case USBDeviceSpeedHigh:
		return []uint32{480}
	case USBDeviceSpeedSuper:
		return []uint32{5000}
	case USBDeviceSpeedSuperPlus:
		return []uint32{10000, 20000}
	default:
		return nil
	}
}

// https://mjmwired.net/kernel/Documentation/ABI/testing/sysfs-bus-usb#502
func ResolveDeviceSpeed(speed uint32) USBDeviceSpeed {
	switch speed {
	case 1:
		return USBDeviceSpeedLow
	case 12, 15:
		return USBDeviceSpeedFull
	case 480:
		return USBDeviceSpeedHigh
	case 5000:
		return USBDeviceSpeedSuper
	case 10000, 20000:
		return USBDeviceSpeedSuperPlus
	default:
		return USBDeviceSpeedUnknown
	}
}
