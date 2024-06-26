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
	"unicode"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	cdiv1 "kubevirt.io/containerized-data-importer-api/pkg/apis/core/v1beta1"

	"github.com/deckhouse/virtualization/api/core/v1alpha2/cvicondition"
)

func GetCondition(condType cvicondition.Type, conds []metav1.Condition) (metav1.Condition, bool) {
	for _, cond := range conds {
		if cond.Type == condType {
			return cond, true
		}
	}

	return metav1.Condition{}, false
}

func SetCondition(cond metav1.Condition, conditions *[]metav1.Condition) {
	if conditions == nil {
		return
	}

	for i := range *conditions {
		if (*conditions)[i].Type == cond.Type {
			(*conditions)[i] = cond
			return
		}
	}

	*conditions = append(*conditions, cond)
}

func CapitalizeFirstLetter(s string) string {
	if s == "" {
		return ""
	}

	// Convert the first rune to uppercase and append the rest of the string.
	runes := []rune(s)
	runes[0] = unicode.ToUpper(runes[0])

	return string(runes)
}

func GetDataVolumeCondition(conditionType cdiv1.DataVolumeConditionType, conditions []cdiv1.DataVolumeCondition) *cdiv1.DataVolumeCondition {
	for i, condition := range conditions {
		if condition.Type == conditionType {
			return &conditions[i]
		}
	}
	return nil
}
