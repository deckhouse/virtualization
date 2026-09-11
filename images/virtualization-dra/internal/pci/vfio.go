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
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"syscall"
	"time"

	"golang.org/x/sys/unix"
)

const (
	defaultDevRoot = "/dev"

	vfioGroupDeviceWait = 5 * time.Second
)

// bindGroupToVFIO binds every endpoint device of the IOMMU group to vfio-pci.
// The kernel exposes /dev/vfio/<group> as usable only when all endpoint
// devices of the group are held by vfio drivers.
func (s sysfs) bindGroupToVFIO(group int) error {
	endpoints, err := s.endpointGroupDevices(group)
	if err != nil {
		return err
	}

	for _, address := range endpoints {
		if err := s.bindDeviceToVFIO(address); err != nil {
			return fmt.Errorf("failed to bind device %s of IOMMU group %d to %s: %w", address, group, vfioDriverName, err)
		}
	}
	return nil
}

func (s sysfs) bindDeviceToVFIO(address string) error {
	driver, err := s.driver(address)
	if err != nil {
		return err
	}
	if driver == vfioDriverName {
		return nil
	}

	if err := s.setDriverOverride(address, vfioDriverName); err != nil {
		return err
	}
	if err := s.unbind(address); err != nil {
		return err
	}
	if err := s.probe(address); err != nil {
		return err
	}

	driver, err = s.driver(address)
	if err != nil {
		return err
	}
	if driver != vfioDriverName {
		return fmt.Errorf("device is bound to %q after probe, expected %q", driver, vfioDriverName)
	}
	return nil
}

// restoreGroupDrivers releases every endpoint device of the IOMMU group from
// vfio-pci and lets the kernel rebind the default driver: clearing
// driver_override and re-probing restores the pre-passthrough binding without
// having to remember the original driver name.
func (s sysfs) restoreGroupDrivers(group int) error {
	endpoints, err := s.endpointGroupDevices(group)
	if err != nil {
		return err
	}

	var errs []error
	for _, address := range endpoints {
		if err := s.restoreDeviceDriver(address); err != nil {
			errs = append(errs, fmt.Errorf("failed to restore driver for device %s of IOMMU group %d: %w", address, group, err))
		}
	}
	return errors.Join(errs...)
}

func (s sysfs) restoreDeviceDriver(address string) error {
	driver, err := s.driver(address)
	if err != nil {
		return err
	}
	if driver != vfioDriverName {
		return nil
	}

	if err := s.setDriverOverride(address, ""); err != nil {
		return err
	}
	if err := s.unbind(address); err != nil {
		return err
	}
	return s.probe(address)
}

// vfioDeviceNode describes a character device to inject into the container.
type vfioDeviceNode struct {
	Path  string
	Major int64
	Minor int64
}

// vfioGroupDeviceNodes returns the /dev/vfio/vfio container node and the
// group node /dev/vfio/<group>. The group node is created by the kernel
// asynchronously after the last endpoint binds, hence the bounded wait.
func vfioGroupDeviceNodes(devRoot string, group int) ([]vfioDeviceNode, error) {
	if devRoot == "" {
		devRoot = defaultDevRoot
	}

	containerNode, err := statDeviceNode(filepath.Join(devRoot, "vfio", "vfio"))
	if err != nil {
		return nil, fmt.Errorf("vfio container device is not available (is the vfio-pci module loaded?): %w", err)
	}

	groupPath := filepath.Join(devRoot, "vfio", strconv.Itoa(group))
	groupNode, err := waitDeviceNode(groupPath, vfioGroupDeviceWait)
	if err != nil {
		return nil, fmt.Errorf("vfio group device %s is not available: %w", groupPath, err)
	}

	return []vfioDeviceNode{containerNode, groupNode}, nil
}

func waitDeviceNode(path string, timeout time.Duration) (vfioDeviceNode, error) {
	deadline := time.Now().Add(timeout)
	for {
		node, err := statDeviceNode(path)
		if err == nil {
			return node, nil
		}
		if !os.IsNotExist(err) || time.Now().After(deadline) {
			return vfioDeviceNode{}, err
		}
		time.Sleep(100 * time.Millisecond)
	}
}

func statDeviceNode(path string) (vfioDeviceNode, error) {
	info, err := os.Stat(path)
	if err != nil {
		return vfioDeviceNode{}, err
	}
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return vfioDeviceNode{}, fmt.Errorf("failed to stat device node %s", path)
	}
	rdev := uint64(stat.Rdev) //nolint:unconvert // Rdev is not uint64 on every platform
	return vfioDeviceNode{
		Path:  path,
		Major: int64(unix.Major(rdev)),
		Minor: int64(unix.Minor(rdev)),
	}, nil
}
