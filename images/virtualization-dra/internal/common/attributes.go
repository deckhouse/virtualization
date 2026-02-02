package common

import "k8s.io/dynamic-resource-allocation/deviceattribute"

const (
	AttrName         = "name"
	AttrPath         = "path"
	AttrBusID        = "busID"
	AttrManufacturer = "manufacturer"
	AttrProduct      = "product"
	AttrVendorId     = "vendorID"
	AttrProductId    = "productID"
	AttrBCD          = "bcd"
	AttrBus          = "bus"
	AttrDeviceNumber = "deviceNumber"
	AttrMajor        = "major"
	AttrMinor        = "minor"
	AttrSerial       = "serial"
	AttrDevicePath   = "devicePath"

	// https://github.com/kubernetes/kubernetes/tree/4c5746c0bc529439f78af458f8131b5def4dbe5d/staging/src/k8s.io/dynamic-resource-allocation/deviceattribute
	StandardDeviceAttrUsbAddressBus          = deviceattribute.StandardDeviceAttributePrefix + "usbAddressBus"
	StandardDeviceAttrUsbAddressDeviceNumber = deviceattribute.StandardDeviceAttributePrefix + "usbAddressDeviceNumber"
)
