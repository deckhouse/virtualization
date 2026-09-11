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

package pci

import (
	"crypto/sha1"
	"encoding/hex"
	"fmt"

	"github.com/deckhouse/virtualization-dra/internal/consts"
)

const DriverName = consts.VirtualizationDraPCIDriverName

// Device describes a host PCI function suitable for VFIO passthrough.
type Device struct {
	// Address is the PCI address in extended BDF notation, e.g. "0000:3b:00.0".
	Address string
	// VendorID and DeviceID are 4-digit lowercase hex without the "0x" prefix.
	VendorID string
	DeviceID string
	// ClassCode is the 6-digit lowercase hex PCI class code (class/subclass/prog-if).
	ClassCode string
	// Driver is the kernel driver currently bound to the device; empty when unbound.
	Driver string
	// IOMMUGroup is the IOMMU group number the device belongs to.
	IOMMUGroup int
	// NUMANode is the NUMA node of the device; -1 when the platform reports none.
	NUMANode int64
}

func (d *Device) GetName(nodeName string) string {
	unhashed := fmt.Sprintf("%s-%s-%s-%s", d.Address, d.VendorID, d.DeviceID, nodeName)

	hash := sha1.Sum([]byte(unhashed))

	return fmt.Sprintf("pci-%s", hex.EncodeToString(hash[:]))
}

func (d *Device) Validate() error {
	if d.Address == "" {
		return fmt.Errorf("address is required")
	}
	if d.VendorID == "" {
		return fmt.Errorf("vendorID is required")
	}
	if d.DeviceID == "" {
		return fmt.Errorf("deviceID is required")
	}
	if d.ClassCode == "" {
		return fmt.Errorf("class is required")
	}
	if d.IOMMUGroup < 0 {
		return fmt.Errorf("iommuGroup is required")
	}
	return nil
}
