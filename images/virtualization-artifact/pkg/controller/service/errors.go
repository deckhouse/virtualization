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
	ErrDefaultStorageClassNotFound        = errors.New("default storage class not found")
	ErrDataVolumeNotRunning               = errors.New("pvc importer is not running")
	ErrDataVolumeProvisionerUnschedulable = errors.New("provisioner unschedulable")
)

type NoSizingPolicyMatchError struct {
	VMName    string
	ClassName string
}

func NewNoSizingPolicyMatchError(vmName, className string) *NoSizingPolicyMatchError {
	return &NoSizingPolicyMatchError{
		VMName:    vmName,
		ClassName: className,
	}
}

func (e *NoSizingPolicyMatchError) Error() string {
	return fmt.Sprintf("virtual machine %q resources do not match any sizing policies in class %q", e.VMName, e.ClassName)
}
