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

	"github.com/deckhouse/virtualization-controller/pkg/common/validate"
	virtv2 "github.com/deckhouse/virtualization/api/core/v1alpha2"
)

const (
	// MaxKubernetesResourceNameLength specifies the maximum allowable length for Kubernetes resource names.
	MaxKubernetesResourceNameLength = 253
)

var (
	ErrAlreadyInUse                = errors.New("already in use")
	ErrRestoring                   = errors.New("will be restored")
	ErrUpdating                    = errors.New("will be updated")
	ErrWaitingForDeletion          = errors.New("waiting for deletion to complete")
	ErrVMNotInMaintenance          = errors.New("the virtual machine is not in maintenance mode")
	ErrVMMaintenanceCondNotFound   = errors.New("the virtual machine maintenance condition is not found")
	ErrVirtualImageNotFound        = errors.New("the virtual image is not found")
	ErrVirtualDiskSnapshotNotFound = errors.New("not found")
	ErrClusterVirtualImageNotFound = errors.New("the virtual image is not found")
	ErrSecretHasDifferentData      = errors.New("the secret has different data")
	ErrResourceNameTooLong         = errors.New("resource name exceeds maximum allowed length")

	ErrVirtualMachineAlreadyExists      = errors.New("VirtualMachine already exists")
	ErrVirtualDiskAlreadyExists         = errors.New("VirtualDisk already exists")
	ErrVirtualDiskAttachedToDifferentVM = errors.New("VirtualDisk is attached to different VirtualMachine")
	ErrVMBDAAlreadyExists               = errors.New("VirtualMachineBlockDeviceAttachment already exists")
	ErrVMBDAAttachedToDifferentVM       = errors.New("VirtualMachineBlockDeviceAttachment is attached to different VirtualMachine")
	ErrVMIPAttachedToDifferentVM        = errors.New("VirtualMachineIPAddress is attached to different VirtualMachine")
	ErrVMMACAttachedToDifferentVM       = errors.New("VirtualMachineMACAddress is attached to different VirtualMachine")
	ErrImageResourceNotFound            = errors.New("image resource is used by VirtualMachine but absent in cluster")
	ErrSecretContentDifferent           = errors.New("secret content is different from that in the snapshot")
)

// OverrideName overrides the name of the resource with the given rules
func OverrideName(kind, name string, rules []virtv2.NameReplacement) string {
	if name == "" {
		return ""
	}

	for _, rule := range rules {
		if rule.From.Kind != "" && rule.From.Kind != kind {
			continue
		}

		if rule.From.Name == name {
			return rule.To
		}
	}

	return name
}

// ValidateResourceNameLength checks if the given resource name exceeds
// the maximum allowed length for the specified Kubernetes resource kind.
// By default, the maximum length is set to MaxKubernetesResourceNameLength,
// but for VirtualMachine and VirtualDisk resources, it uses
// MaxVirtualMachineNameLen and MaxDiskNameLen respectively.
func ValidateResourceNameLength(resourceName, kind string) error {
	maxLength := MaxKubernetesResourceNameLength
	switch kind {
	case virtv2.VirtualMachineKind:
		maxLength = validate.MaxVirtualMachineNameLen
	case virtv2.VirtualDiskKind:
		maxLength = validate.MaxDiskNameLen
	}
	if len(resourceName) > maxLength {
		return fmt.Errorf("name %q too long (%d > %d): %w",
			resourceName, len(resourceName), maxLength, ErrResourceNameTooLong)
	}
	return nil
}

// ApplyNameCustomization applies prefix and suffix to a resource name for cloning operations
func ApplyNameCustomization(name, prefix, suffix string) string {
	if name == "" {
		return ""
	}
	return prefix + name + suffix
}
