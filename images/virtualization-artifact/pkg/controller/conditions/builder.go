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
	meta.SetStatusCondition(conditions, c.Condition())
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
	return &ConditionBuilder{conditionType: conditionType.String()}
}

func (c *ConditionBuilder) FromCondition(condition metav1.Condition) *ConditionBuilder {
	if condition.Type != "" {
		c.conditionType = condition.Type
	}
	if condition.Status != "" {
		c.status = condition.Status
	} else {
		c.status = metav1.ConditionUnknown
	}
	c.reason = condition.Reason
	c.message = condition.Message
	c.generation = condition.ObservedGeneration
	return c
}

type ConditionBuilder struct {
	status        metav1.ConditionStatus
	conditionType string
	reason        string
	message       string
	generation    int64
}

func (c *ConditionBuilder) Condition() metav1.Condition {
	return metav1.Condition{
		Type:               c.conditionType,
		Status:             c.status,
		Reason:             c.reason,
		LastTransitionTime: metav1.Now(),
		Message:            c.message,
		ObservedGeneration: c.generation,
	}
}

func (c *ConditionBuilder) Status(status metav1.ConditionStatus) *ConditionBuilder {
	c.status = status
	return c
}

func (c *ConditionBuilder) Reason(reason Stringer) *ConditionBuilder {
	c.reason = reason.String()
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

type stringerMock struct {
	Value string
}

func (m stringerMock) String() string {
	return m.Value
}

func (c *ConditionBuilder) GetType() Stringer {
	return stringerMock{Value: c.conditionType}
}
