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
	"slices"

	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// Deprecated: use direct SetCondition instead.
type Manager struct {
	conds []metav1.Condition
}

// Deprecated: use direct SetCondition instead.
func NewManager(conditions []metav1.Condition) *Manager {
	return &Manager{
		conds: slices.Clone(conditions),
	}
}

func (m *Manager) Add(c metav1.Condition) (addedCondition bool) {
	findCond := meta.FindStatusCondition(m.conds, c.Type)
	if findCond != nil {
		return false
	}
	return meta.SetStatusCondition(&m.conds, c)
}

func (m *Manager) Update(c metav1.Condition) {
	meta.SetStatusCondition(&m.conds, c)
}

func (m *Manager) Generate() []metav1.Condition {
	return slices.Clone(m.conds)
}
