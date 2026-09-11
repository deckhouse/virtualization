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
	"fmt"
	"log/slog"
	"os"
	"strings"
)

// PCI base class codes excluded from passthrough discovery. Conservative by
// design: the cost of publishing a host-critical device is a broken node,
// the cost of not publishing a legitimate one is a follow-up allowlist change.
const (
	classMassStorage      = "01"
	classNetwork          = "02"
	classDisplay          = "03"
	classMemoryController = "05"
	classBridge           = "06"
	classSystemPeripheral = "08"
)

var deniedClasses = map[string]struct{}{
	// Display controllers belong to the GPU module (gpu.deckhouse.io).
	classDisplay:          {},
	classMemoryController: {},
	classBridge:           {},
	classSystemPeripheral: {},
}

// isRootComplexIntegrated reports whether the device sits directly on the
// root bus (bus 00). Such endpoints are integrated into the SoC/PCH (audio,
// SMBus, MEI, PMT and so on), usually lack FLR and are part of the platform:
// rebinding one to vfio-pci can hang the host outright. Passthrough-capable
// hardware lives behind root ports: slot cards and SR-IOV functions.
func isRootComplexIntegrated(address string) bool {
	parts := strings.Split(address, ":")
	return len(parts) == 3 && parts[1] == "00"
}

func baseClass(classCode string) string {
	if len(classCode) < 2 {
		return classCode
	}
	return classCode[:2]
}

func isBridge(classCode string) bool {
	return baseClass(classCode) == classBridge
}

func (s sysfs) loadDevice(address string) (Device, error) {
	vendorID, err := s.readDeviceHex(address, "vendor")
	if err != nil {
		return Device{}, err
	}
	deviceID, err := s.readDeviceHex(address, "device")
	if err != nil {
		return Device{}, err
	}
	classCode, err := s.readDeviceHex(address, "class")
	if err != nil {
		return Device{}, err
	}
	driver, err := s.driver(address)
	if err != nil {
		return Device{}, err
	}
	group, err := s.iommuGroup(address)
	if err != nil {
		return Device{}, err
	}

	return Device{
		Address:    address,
		VendorID:   vendorID,
		DeviceID:   deviceID,
		ClassCode:  classCode,
		Driver:     driver,
		IOMMUGroup: group,
		NUMANode:   s.numaNode(address),
	}, nil
}

// discoverDevices returns passthrough-capable devices keyed by PCI address.
// A device is published only when it passes the class filters and every
// non-bridge device in its IOMMU group passes them too: the group is the
// passthrough unit, so a group holding a host-critical device is unusable.
func (s sysfs) discoverDevices(log *slog.Logger) (map[string]Device, error) {
	entries, err := os.ReadDir(s.devicesDir())
	if err != nil {
		return nil, fmt.Errorf("failed to list PCI devices: %w", err)
	}

	all := make(map[string]Device, len(entries))
	for _, entry := range entries {
		address := entry.Name()
		device, err := s.loadDevice(address)
		if err != nil {
			log.Warn("Failed to load PCI device, skipping", slog.String("address", address), slog.Any("err", err))
			continue
		}
		all[address] = device
	}

	allowed := make(map[string]bool, len(all))
	for address, device := range all {
		ok, err := s.deviceAllowed(&device)
		if err != nil {
			return nil, err
		}
		allowed[address] = ok
	}

	result := make(map[string]Device)
	for address, device := range all {
		if !allowed[address] {
			continue
		}
		clean, err := s.groupClean(&device, all, allowed)
		if err != nil {
			return nil, err
		}
		if !clean {
			log.Debug("Skipping PCI device: IOMMU group contains non-passthrough devices", slog.String("address", address), slog.Int("iommuGroup", device.IOMMUGroup))
			continue
		}
		result[address] = device
	}

	return result, nil
}

func (s sysfs) deviceAllowed(device *Device) (bool, error) {
	if device.IOMMUGroup < 0 {
		return false, nil
	}
	if isRootComplexIntegrated(device.Address) {
		return false, nil
	}
	if _, denied := deniedClasses[baseClass(device.ClassCode)]; denied {
		return false, nil
	}
	if baseClass(device.ClassCode) == classNetwork {
		active, err := s.hasActiveNetInterface(device.Address)
		if err != nil {
			return false, err
		}
		if active {
			return false, nil
		}
	}
	// Storage controllers are eligible (NVMe passthrough is a first-class use
	// case) unless the host uses any of their block devices.
	if baseClass(device.ClassCode) == classMassStorage {
		busy, err := s.hasBusyBlockDevice(device.Address)
		if err != nil {
			return false, err
		}
		if busy {
			return false, nil
		}
	}
	return true, nil
}

func (s sysfs) groupClean(device *Device, all map[string]Device, allowed map[string]bool) (bool, error) {
	members, err := s.iommuGroupDevices(device.IOMMUGroup)
	if err != nil {
		return false, fmt.Errorf("failed to list IOMMU group %d devices: %w", device.IOMMUGroup, err)
	}
	for _, member := range members {
		if member == device.Address {
			continue
		}
		memberDevice, known := all[member]
		if !known {
			return false, nil
		}
		if isBridge(memberDevice.ClassCode) {
			continue
		}
		if !allowed[member] {
			return false, nil
		}
	}
	return true, nil
}

// endpointGroupDevices lists non-bridge devices of the IOMMU group; these are
// the devices that must be bound to vfio-pci for the group to be viable.
func (s sysfs) endpointGroupDevices(group int) ([]string, error) {
	members, err := s.iommuGroupDevices(group)
	if err != nil {
		return nil, err
	}
	endpoints := make([]string, 0, len(members))
	for _, member := range members {
		classCode, err := s.readDeviceHex(member, "class")
		if err != nil {
			return nil, err
		}
		if isBridge(classCode) {
			continue
		}
		endpoints = append(endpoints, member)
	}
	if len(endpoints) == 0 {
		return nil, fmt.Errorf("IOMMU group %d has no endpoint devices", group)
	}
	return endpoints, nil
}
