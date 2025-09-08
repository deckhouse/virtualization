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

	virtv2 "github.com/deckhouse/virtualization/api/core/v1alpha2"
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
