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

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// Deprecated: use direct SetCondition instead.
type Manager struct {
	conds      []metav1.Condition
	indexConds map[string]int
}

// Deprecated: use direct SetCondition instead.
func NewManager(conditions []metav1.Condition) *Manager {
	conds := make([]metav1.Condition, len(conditions))
	indexConds := make(map[string]int, len(conds))
	for i, c := range conditions {
		conds[i] = c
		indexConds[c.Type] = i
	}
	return &Manager{
		conds:      conds,
		indexConds: indexConds,
	}
}

func (m *Manager) Add(c metav1.Condition) (addedCondition bool) {
	if _, found := m.indexConds[c.Type]; found {
		return false
	}
	m.conds = append(m.conds, c)
	m.indexConds[c.Type] = len(m.conds) - 1
	return true
}

func (m *Manager) Update(c metav1.Condition) {
	ApplyCondition(c, &m.conds)
}

func (m *Manager) Generate() []metav1.Condition {
	return slices.Clone(m.conds)
}
