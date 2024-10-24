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

package internal

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/deckhouse/virtualization-controller/pkg/controller/conditions"
	virtv2 "github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/vmclasscondition"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/vmcondition"
)

func isDeletion(class *virtv2.VirtualMachineClass) bool {
	return class == nil || !class.GetDeletionTimestamp().IsZero()
}

func addAllUnknown(vm *virtv2.VirtualMachineClass, conds ...vmclasscondition.Type) (update bool) {
	for _, cond := range conds {
		if conditions.HasCondition(cond, vm.Status.Conditions) {
			continue
		}
		cb := conditions.NewConditionBuilder(cond).
			Generation(vm.GetGeneration()).
			Reason(vmcondition.ReasonUnknown).
			Status(metav1.ConditionUnknown)
		conditions.SetCondition(cb, &vm.Status.Conditions)
		update = true
	}
	return
}
