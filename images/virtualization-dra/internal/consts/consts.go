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

package consts

const (
	USBGatewayLabel = "virtualization.deckhouse.io/usb-gateway"
)

const (
	VirtualizationDraUSBDriverName = "virtualization-usb"
)

const (
	AnnUSBDeviceAddresses = "usb.virtualization.deckhouse.io/device-addresses"
	AnnUSBDeviceUser      = "usb.virtualization.deckhouse.io/device-user"
	AnnUSBDeviceGroup     = "usb.virtualization.deckhouse.io/device-group"
	AnnUSBIPTotalPorts    = "usb.virtualization.deckhouse.io/usbip-total-ports"
	AnnUSBIPUsedPorts     = "usb.virtualization.deckhouse.io/usbip-used-ports"
	AnnUSBIPAddress       = "usb.virtualization.deckhouse.io/usbip-address"
)
