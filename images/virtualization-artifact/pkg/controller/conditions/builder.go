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

package conditions

import (
	"time"

	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type Conder interface {
	Condition() metav1.Condition
}

func HasCondition(conditionType Stringer, conditions []metav1.Condition) bool {
	for _, condition := range conditions {
		if condition.Type == conditionType.String() {
			return true
		}
	}

	return false
}

func SetCondition(c Conder, conditions *[]metav1.Condition) {
	newCondition := c.Condition()
	if conditions == nil {
		return
	}
	existingCondition := FindStatusCondition(*conditions, newCondition.Type)
	if existingCondition == nil {
		if newCondition.LastTransitionTime.IsZero() {
			newCondition.LastTransitionTime = metav1.NewTime(time.Now())
		}
		*conditions = append(*conditions, newCondition)
		return
	}

	if existingCondition.Status != newCondition.Status {
		existingCondition.Status = newCondition.Status
		if !newCondition.LastTransitionTime.IsZero() {
			existingCondition.LastTransitionTime = newCondition.LastTransitionTime
		} else {
			existingCondition.LastTransitionTime = metav1.NewTime(time.Now())
		}
	}

	if existingCondition.Reason != newCondition.Reason {
		existingCondition.Reason = newCondition.Reason
	}
	if existingCondition.Message != newCondition.Message {
		existingCondition.Message = newCondition.Message
	}
	if existingCondition.ObservedGeneration != newCondition.ObservedGeneration {
		existingCondition.ObservedGeneration = newCondition.ObservedGeneration
	}
}

func FindStatusCondition(conditions []metav1.Condition, conditionType string) *metav1.Condition {
	for i := range conditions {
		if conditions[i].Type == conditionType {
			return &conditions[i]
		}
	}

	return nil
}

func RemoveCondition(conditionType Stringer, conditions *[]metav1.Condition) {
	meta.RemoveStatusCondition(conditions, conditionType.String())
}

func GetCondition(condType Stringer, conditions []metav1.Condition) (metav1.Condition, bool) {
	for _, condition := range conditions {
		if condition.Type == condType.String() {
			return condition, true
		}
	}

	return metav1.Condition{}, false
}

func NewConditionBuilder(conditionType Stringer) *ConditionBuilder {
	return &ConditionBuilder{
		status:        metav1.ConditionUnknown,
		reason:        ReasonUnknown.String(),
		conditionType: conditionType,
	}
}

type ConditionBuilder struct {
	status        metav1.ConditionStatus
	conditionType Stringer
	reason        string
	message       string
	generation    int64
}

func (c *ConditionBuilder) Condition() metav1.Condition {
	return metav1.Condition{
		Type:               c.conditionType.String(),
		Status:             c.status,
		Reason:             c.reason,
		Message:            c.message,
		ObservedGeneration: c.generation,
	}
}

func (c *ConditionBuilder) Status(status metav1.ConditionStatus) *ConditionBuilder {
	if status != "" {
		c.status = status
	}
	return c
}

func (c *ConditionBuilder) Reason(reason Stringer) *ConditionBuilder {
	if reason.String() != "" {
		c.reason = reason.String()
	}
	return c
}

func (c *ConditionBuilder) Message(msg string) *ConditionBuilder {
	c.message = msg
	return c
}

func (c *ConditionBuilder) Generation(generation int64) *ConditionBuilder {
	c.generation = generation
	return c
}

func (c *ConditionBuilder) Clone() *ConditionBuilder {
	var out *ConditionBuilder
	*out = *c
	return out
}

func (c *ConditionBuilder) GetType() Stringer {
	return c.conditionType
}
