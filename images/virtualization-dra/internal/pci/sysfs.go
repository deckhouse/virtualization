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
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

const (
	defaultSysfsRoot = "/sys"
	defaultProcRoot  = "/proc"

	vfioDriverName = "vfio-pci"
)

// sysfs reads and mutates the PCI subsystem state under configurable roots,
// which keeps every operation testable against a fake directory tree.
type sysfs struct {
	root string
	proc string
}

func newSysfs(root string) sysfs {
	if root == "" {
		root = defaultSysfsRoot
	}
	return sysfs{root: root, proc: defaultProcRoot}
}

func (s sysfs) devicesDir() string {
	return filepath.Join(s.root, "bus", "pci", "devices")
}

func (s sysfs) deviceDir(address string) string {
	return filepath.Join(s.devicesDir(), address)
}

func (s sysfs) readDeviceFile(address, name string) (string, error) {
	data, err := os.ReadFile(filepath.Join(s.deviceDir(address), name))
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(data)), nil
}

// readDeviceHex reads a sysfs value like "0x10ee" and returns it without the prefix.
func (s sysfs) readDeviceHex(address, name string) (string, error) {
	value, err := s.readDeviceFile(address, name)
	if err != nil {
		return "", err
	}
	return strings.ToLower(strings.TrimPrefix(value, "0x")), nil
}

// driver returns the name of the driver bound to the device, or "" when unbound.
func (s sysfs) driver(address string) (string, error) {
	target, err := os.Readlink(filepath.Join(s.deviceDir(address), "driver"))
	if err != nil {
		if os.IsNotExist(err) {
			return "", nil
		}
		return "", err
	}
	return filepath.Base(target), nil
}

// iommuGroup returns the IOMMU group number of the device, or -1 when the
// device is not in any group (IOMMU disabled or not covered).
func (s sysfs) iommuGroup(address string) (int, error) {
	target, err := os.Readlink(filepath.Join(s.deviceDir(address), "iommu_group"))
	if err != nil {
		if os.IsNotExist(err) {
			return -1, nil
		}
		return -1, err
	}
	group, err := strconv.Atoi(filepath.Base(target))
	if err != nil {
		return -1, fmt.Errorf("unexpected iommu_group link %q for device %s: %w", target, address, err)
	}
	return group, nil
}

func (s sysfs) numaNode(address string) int64 {
	value, err := s.readDeviceFile(address, "numa_node")
	if err != nil {
		return -1
	}
	node, err := strconv.ParseInt(value, 10, 64)
	if err != nil {
		return -1
	}
	return node
}

// iommuGroupDevices lists PCI addresses of all devices in the IOMMU group.
func (s sysfs) iommuGroupDevices(group int) ([]string, error) {
	entries, err := os.ReadDir(filepath.Join(s.root, "kernel", "iommu_groups", strconv.Itoa(group), "devices"))
	if err != nil {
		return nil, err
	}
	addresses := make([]string, 0, len(entries))
	for _, entry := range entries {
		addresses = append(addresses, entry.Name())
	}
	return addresses, nil
}

// hasActiveNetInterface reports whether any network interface backed by the
// device is administratively up. Interfaces are looked up through
// /sys/class/net because a netdev may sit below a child bus device
// (virtio-net exposes it as <device>/virtio*/net/<name>).
func (s sysfs) hasActiveNetInterface(address string) (bool, error) {
	deviceDir, err := filepath.EvalSymlinks(s.deviceDir(address))
	if err != nil {
		return false, err
	}

	entries, err := os.ReadDir(filepath.Join(s.root, "class", "net"))
	if err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, err
	}
	for _, entry := range entries {
		netDir := filepath.Join(s.root, "class", "net", entry.Name())
		realPath, err := filepath.EvalSymlinks(netDir)
		if err != nil || !strings.HasPrefix(realPath, deviceDir+string(filepath.Separator)) {
			continue
		}
		data, err := os.ReadFile(filepath.Join(netDir, "flags"))
		if err != nil {
			continue
		}
		flags, err := strconv.ParseInt(strings.TrimPrefix(strings.TrimSpace(string(data)), "0x"), 16, 64)
		if err != nil {
			continue
		}
		const iffUp = 0x1
		if flags&iffUp != 0 {
			return true, nil
		}
	}
	return false, nil
}

// blockDevices returns the names of block devices (including partitions)
// backed by the PCI device: every /sys/class/block entry whose real path
// lives under the device directory. This covers both NVMe namespaces and
// disks behind SCSI/SATA controllers.
func (s sysfs) blockDevices(address string) ([]string, error) {
	deviceDir, err := filepath.EvalSymlinks(s.deviceDir(address))
	if err != nil {
		return nil, err
	}

	entries, err := os.ReadDir(filepath.Join(s.root, "class", "block"))
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	var names []string
	for _, entry := range entries {
		realPath, err := filepath.EvalSymlinks(filepath.Join(s.root, "class", "block", entry.Name()))
		if err != nil {
			continue
		}
		if strings.HasPrefix(realPath, deviceDir+string(filepath.Separator)) {
			names = append(names, entry.Name())
		}
	}
	return names, nil
}

// hasBusyBlockDevice reports whether any block device of the PCI device is in
// use by the host: stacked upon (LVM/RAID/dm holders), mounted, or used as swap.
func (s sysfs) hasBusyBlockDevice(address string) (bool, error) {
	names, err := s.blockDevices(address)
	if err != nil {
		return false, err
	}
	if len(names) == 0 {
		return false, nil
	}

	for _, name := range names {
		holders, err := os.ReadDir(filepath.Join(s.root, "class", "block", name, "holders"))
		if err != nil && !os.IsNotExist(err) {
			return false, err
		}
		if len(holders) > 0 {
			return true, nil
		}
	}

	used, err := s.usedBlockDeviceSources()
	if err != nil {
		return false, err
	}
	for _, name := range names {
		if _, busy := used["/dev/"+name]; busy {
			return true, nil
		}
	}
	return false, nil
}

// usedBlockDeviceSources collects device paths currently mounted or used as swap.
func (s sysfs) usedBlockDeviceSources() (map[string]struct{}, error) {
	used := make(map[string]struct{})

	for _, file := range []string{"mounts", "swaps"} {
		data, err := os.ReadFile(filepath.Join(s.proc, file))
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return nil, err
		}
		for _, line := range strings.Split(string(data), "\n") {
			fields := strings.Fields(line)
			if len(fields) > 0 && strings.HasPrefix(fields[0], "/dev/") {
				used[fields[0]] = struct{}{}
			}
		}
	}
	return used, nil
}

func (s sysfs) writeFile(path, value string) error {
	f, err := os.OpenFile(path, os.O_WRONLY, 0)
	if err != nil {
		return err
	}
	defer f.Close()

	if _, err := f.WriteString(value); err != nil {
		return fmt.Errorf("failed to write %q to %s: %w", value, path, err)
	}
	return nil
}

func (s sysfs) setDriverOverride(address, driver string) error {
	if driver == "" {
		driver = "\n"
	}
	return s.writeFile(filepath.Join(s.deviceDir(address), "driver_override"), driver)
}

func (s sysfs) unbind(address string) error {
	driver, err := s.driver(address)
	if err != nil {
		return err
	}
	if driver == "" {
		return nil
	}
	return s.writeFile(filepath.Join(s.deviceDir(address), "driver", "unbind"), address)
}

func (s sysfs) probe(address string) error {
	return s.writeFile(filepath.Join(s.root, "bus", "pci", "drivers_probe"), address)
}
