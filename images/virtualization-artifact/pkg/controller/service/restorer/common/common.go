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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/deckhouse/virtualization-controller/pkg/controller/conditions"
	"github.com/deckhouse/virtualization-controller/pkg/controller/service"
	virtv2 "github.com/deckhouse/virtualization/api/core/v1alpha2"
	vmrestorecondition "github.com/deckhouse/virtualization/api/core/v1alpha2/vm-restore-condition"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/vmopcondition"
)

func SetPhaseConditionToFailed(cb *conditions.ConditionBuilder, phase *virtv2.VMOPPhase, err error) {
	*phase = virtv2.VMOPPhaseFailed
	cb.
		Status(metav1.ConditionFalse).
		Reason(vmrestorecondition.VirtualMachineRestoreFailed).
		Message(service.CapitalizeFirstLetter(err.Error()) + ".")
}

func SetPhaseConditionToPending(cb *conditions.ConditionBuilder, phase *virtv2.VMOPPhase, reason vmopcondition.ReasonCompleted, msg string) {
	*phase = virtv2.VMOPPhasePending
	cb.
		Status(metav1.ConditionFalse).
		Reason(reason).
		Message(service.CapitalizeFirstLetter(msg) + ".")
}

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
