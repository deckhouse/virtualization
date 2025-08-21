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

package common

import (
	"errors"
	"fmt"
)

var (
	ErrVirtualMachineAlreadyExists      = errors.New("VirtualMachine already exists")
	ErrVirtualDiskAlreadyExists         = errors.New("VirtualDisk already exists")
	ErrVirtualDiskAttachedToDifferentVM = errors.New("VirtualDisk is attached to different VirtualMachine")
	ErrVMBDAAlreadyExists               = errors.New("VirtualMachineBlockDeviceAttachment already exists")
	ErrVMBDAAttachedToDifferentVM       = errors.New("VirtualMachineBlockDeviceAttachment is attached to different VirtualMachine")
	ErrVMIPAttachedToDifferentVM        = errors.New("VirtualMachineIPAddress is attached to different VirtualMachine")
	ErrImageResourceNotFound            = errors.New("image resource is used by VirtualMachine but absent in cluster")
	ErrSecretContentDifferent           = errors.New("secret content is different from that in the snapshot")
)

func FormatVirtualMachineConflictError(vmName string) error {
	return fmt.Errorf("VirtualMachine with name %s already exists", vmName)
}

func FormatVirtualDiskConflictError(vdName string) error {
	return fmt.Errorf("VirtualDisk with name %s already exists", vdName)
}

func FormatVirtualDiskAttachedError(vdName, attachedVM string) error {
	return fmt.Errorf("VirtualDisk with name %s attached to VirtualMachine %s", vdName, attachedVM)
}

func FormatVMBDAConflictError(vmbdaName string) error {
	return fmt.Errorf("VirtualMachineBlockDeviceAttachment with name %s already exists", vmbdaName)
}

func FormatVMBDAAttachedError(vmbdaName, attachedVM string) error {
	return fmt.Errorf("VirtualMachineBlockDeviceAttachment with name %s already exists and attached to VirtualMachine %s", vmbdaName, attachedVM)
}

func FormatVMIPAttachedError(vmipName, attachedVM string) error {
	return fmt.Errorf("VirtualMachineIPAddress with name %s already exists and attached to VirtualMachine %s", vmipName, attachedVM)
}

func FormatImageResourceNotFoundError(resourceType, resourceName, vmName string) error {
	return fmt.Errorf("%s %s is used by VirtualMachine %s but absent in cluster", resourceType, resourceName, vmName)
}

func FormatSecretContentDifferentError(secretName string) error {
	return fmt.Errorf("content of the secret %s is different from that in the snapshot", secretName)
}
