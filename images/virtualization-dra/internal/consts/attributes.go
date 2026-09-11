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

package consts

const (
	AttrName         = "name"
	AttrPath         = "path"
	AttrBusID        = "busID"
	AttrManufacturer = "manufacturer"
	AttrProduct      = "product"
	AttrVendorID     = "vendorID"
	AttrProductID    = "productID"
	AttrBCD          = "bcd"
	AttrBus          = "bus"
	AttrDeviceNumber = "deviceNumber"
	AttrMajor        = "major"
	AttrMinor        = "minor"
	AttrSpeed        = "speed"
	AttrSerial       = "serial"
	AttrDevicePath   = "devicePath"
	AttrUsbAddress   = "usbAddress"
)

const (
	AttrDeviceID   = "deviceID"
	AttrClass      = "class"
	AttrIOMMUGroup = "iommuGroup"
	AttrDriver     = "driver"
)

// Standard Kubernetes device attributes (k8s.io/dynamic-resource-allocation/deviceattribute).
// pciBusID and numaNode constants are not yet available in the vendored library version.
const (
	AttrStandardPCIBusID = "resource.kubernetes.io/pciBusID"
	AttrStandardNUMANode = "resource.kubernetes.io/numaNode"
)
