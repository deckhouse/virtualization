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

package supersede

import (
	"k8s.io/utils/ptr"

	"github.com/deckhouse/virtualization/api/core/v1alpha2"
)

func CanSupersede(oldVMOP, newVMOP *v1alpha2.VirtualMachineOperation) bool {
	if oldVMOP == nil || newVMOP == nil {
		return false
	}
	if oldVMOP.Spec.VirtualMachine != newVMOP.Spec.VirtualMachine {
		return false
	}

	newForce := ptr.Deref(newVMOP.Spec.Force, false)

	switch oldVMOP.Spec.Type {
	case v1alpha2.VMOPTypeStart:
		return newVMOP.Spec.Type == v1alpha2.VMOPTypeStop || newVMOP.Spec.Type == v1alpha2.VMOPTypeRestart
	case v1alpha2.VMOPTypeStop:
		oldForce := ptr.Deref(oldVMOP.Spec.Force, false)
		return !oldForce && newVMOP.Spec.Type == v1alpha2.VMOPTypeStop && newForce
	case v1alpha2.VMOPTypeMigrate, v1alpha2.VMOPTypeEvict:
		return newVMOP.Spec.Type == v1alpha2.VMOPTypeStop
	case v1alpha2.VMOPTypeRestart:
		return newVMOP.Spec.Type == v1alpha2.VMOPTypeStop
	default:
		return false
	}
}
