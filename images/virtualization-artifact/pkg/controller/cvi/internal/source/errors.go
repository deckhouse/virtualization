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

package source

import (
	"errors"
	"fmt"
)

var ErrSecretNotFound = errors.New("container registry secret not found")

type ImageNotReadyError struct {
	name string
}

func (e ImageNotReadyError) Error() string {
	return fmt.Sprintf("VirtualImage %s not ready", e.name)
}

func NewImageNotReadyError(name string) error {
	return ImageNotReadyError{
		name: name,
	}
}

type ClusterImageNotReadyError struct {
	name string
}

func (e ClusterImageNotReadyError) Error() string {
	return fmt.Sprintf("ClusterVirtualImage %s not ready", e.name)
}

func NewClusterImageNotReadyError(name string) error {
	return ClusterImageNotReadyError{
		name: name,
	}
}

type VirtualDiskNotReadyError struct {
	name string
}

func (e VirtualDiskNotReadyError) Error() string {
	return fmt.Sprintf("VirtualDisk %s not ready", e.name)
}

func NewVirtualDiskNotReadyError(name string) error {
	return VirtualDiskNotReadyError{
		name: name,
	}
}

type VirtualDiskNotReadyForUseError struct {
	name string
}

func (e VirtualDiskNotReadyForUseError) Error() string {
	return fmt.Sprintf("the VirtualDisk %s not ready for use", e.name)
}

func NewVirtualDiskNotReadyForUseError(name string) error {
	return VirtualDiskNotReadyForUseError{
		name: name,
	}
}

type VirtualDiskAttachedToVirtualMachineError struct {
	name string
}

func (e VirtualDiskAttachedToVirtualMachineError) Error() string {
	return fmt.Sprintf("the VirtualDisk %s attached to VirtualMachine", e.name)
}

func NewVirtualDiskAttachedToVirtualMachineError(name string) error {
	return VirtualDiskAttachedToVirtualMachineError{
		name: name,
	}
}

type VirtualDiskSnapshotNotReadyError struct {
	name string
}

func (e VirtualDiskSnapshotNotReadyError) Error() string {
	return fmt.Sprintf("VirtualDiskSnapshot %s not ready", e.name)
}

func NewVirtualDiskSnapshotNotReadyError(name string) error {
	return VirtualDiskSnapshotNotReadyError{
		name: name,
	}
}
