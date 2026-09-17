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
	"strings"

	"k8s.io/apimachinery/pkg/api/resource"
)

var (
	ErrDefaultStorageClassNotFound = errors.New("default storage class not found")
	ErrImporterNotRunning          = errors.New("pvc importer is not running")
	ErrProvisionerUnschedulable    = errors.New("provisioner unschedulable")
)

// NewInsufficientRestoreSizeError reports a restore into a disk smaller than the one the snapshot was
// taken from. Both the admission webhook and the provisioning step build the text here, so the user is
// told about the same floor in the same words twice.
func NewInsufficientRestoreSizeError(vdSnapshotName string, requested, sourceSize *resource.Quantity) error {
	return fmt.Errorf(
		"%w: the VirtualDiskSnapshot %q was taken from a %s disk and cannot be restored into %s; increase spec.persistentVolumeClaim.size to at least %s",
		ErrInsufficientPVCSize, vdSnapshotName, sourceSize, requested, sourceSize,
	)
}

// CoreRange is an inclusive CPU core range allowed by a sizing policy.
type CoreRange struct {
	Min int
	Max int
}

type NoSizingPolicyMatchError struct {
	Cores     int
	ClassName string
	Ranges    []CoreRange
}

func NewNoSizingPolicyMatchError(cores int, className string, ranges []CoreRange) *NoSizingPolicyMatchError {
	return &NoSizingPolicyMatchError{
		Cores:     cores,
		ClassName: className,
		Ranges:    ranges,
	}
}

func (e *NoSizingPolicyMatchError) Error() string {
	if len(e.Ranges) == 0 {
		return fmt.Sprintf(
			"does not match any sizing policy of VirtualMachineClass %q: its %d CPU core(s) are not covered by any policy",
			e.ClassName, e.Cores,
		)
	}

	rangeStrs := make([]string, len(e.Ranges))
	for i, r := range e.Ranges {
		rangeStrs[i] = fmt.Sprintf("%d-%d", r.Min, r.Max)
	}

	return fmt.Sprintf(
		"does not match any sizing policy of VirtualMachineClass %q: its %d CPU core(s) fall outside the allowed ranges (%s); set the number of cores (spec.cpu.cores) accordingly",
		e.ClassName, e.Cores, strings.Join(rangeStrs, ", "),
	)
}
