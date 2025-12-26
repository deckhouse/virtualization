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

package usb

type DeviceSpeed uint32

const (
	DeviceSpeedUnknown   DeviceSpeed = iota // enumerating
	DeviceSpeedLow                          // usb 1.1
	DeviceSpeedFull                         // usb 1.1
	DeviceSpeedHigh                         // usb 2.0
	DeviceSpeedSuper                        // usb 3.0
	DeviceSpeedSuperPlus                    // usb 3.1
)

// https://mjmwired.net/kernel/Documentation/ABI/testing/sysfs-bus-usb#502
func ResolveDeviceSpeed(speed uint32) DeviceSpeed {
	switch speed {
	case 1:
		return DeviceSpeedLow
	case 12, 15:
		return DeviceSpeedFull
	case 480:
		return DeviceSpeedHigh
	case 5000:
		return DeviceSpeedSuper
	case 10000, 20000:
		return DeviceSpeedSuperPlus
	default:
		return DeviceSpeedUnknown
	}
}
