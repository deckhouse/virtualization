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
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/vmopcondition"
)

func SetPhaseCloneConditionToFailed(cb *conditions.ConditionBuilder, phase *v1alpha2.VMOPPhase, err error) {
	*phase = v1alpha2.VMOPPhaseFailed
	cb.
		Status(metav1.ConditionFalse).
		Reason(vmopcondition.ReasonCloneOperationFailed).
		Message(service.CapitalizeFirstLetter(err.Error()) + ".")
}
