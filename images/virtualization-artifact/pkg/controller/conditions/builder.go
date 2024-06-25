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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type Stringer interface {
	String() string
}

func NewConditionBuilder2(name Stringer) *ConditionBuilder {
	return &ConditionBuilder{name: name.String()}
}

func NewConditionBuilder(name string) *ConditionBuilder {
	return &ConditionBuilder{name: name}
}

type ConditionBuilder struct {
	status     metav1.ConditionStatus
	name       string
	reason     string
	msg        string
	generation int64
}

func (c *ConditionBuilder) Condition() metav1.Condition {
	return metav1.Condition{
		Type:               c.name,
		Status:             c.status,
		ObservedGeneration: c.generation,
		LastTransitionTime: metav1.Now(),
		Reason:             c.reason,
		Message:            c.msg,
	}
}

func (c *ConditionBuilder) Status(status metav1.ConditionStatus) *ConditionBuilder {
	c.status = status
	return c
}

func (c *ConditionBuilder) Reason(reason string) *ConditionBuilder {
	c.reason = reason
	return c
}

func (c *ConditionBuilder) Reason2(reason Stringer) *ConditionBuilder {
	c.reason = reason.String()
	return c
}

func (c *ConditionBuilder) Message(msg string) *ConditionBuilder {
	c.msg = msg
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
