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

package service

import (
	"errors"
	"fmt"
)

var (
	ErrStorageClassNotFound               = errors.New("storage class not found")
	ErrStorageProfileNotFound             = errors.New("storage profile not found")
	ErrDefaultStorageClassNotFound        = errors.New("default storage class not found")
	ErrStorageClassNotAllowed             = errors.New("storage class not allowed")
	ErrDataVolumeNotRunning               = errors.New("pvc importer is not running")
	ErrDataVolumeProvisionerUnschedulable = errors.New("provisioner unschedulable")
)

var (
	ErrIPAddressAlreadyExist = errors.New("the IP address is already allocated")
	ErrIPAddressOutOfRange   = errors.New("the IP address is out of range")
)

type VirtualDiskUsedByImageError struct {
	vdName string
}

func (e VirtualDiskUsedByImageError) Error() string {
	return fmt.Sprintf("the virtual disk %q already used by creating image", e.vdName)
}

func NewVirtualDiskUsedByImageError(name string) error {
	return VirtualDiskUsedByImageError{
		vdName: name,
	}
}

type VirtualDiskUsedByVirtualMachineError struct {
	vdName string
}

func (e VirtualDiskUsedByVirtualMachineError) Error() string {
	return fmt.Sprintf("the virtual disk %q already used by running virtual machine", e.vdName)
}

func NewVirtualDiskUsedByVirtualMachineError(name string) error {
	return VirtualDiskUsedByVirtualMachineError{
		vdName: name,
	}
}
