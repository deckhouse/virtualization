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

type Manager struct {
	conds      []metav1.Condition
	indexConds map[string]int
}

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

func (m *Manager) Get(name string) (metav1.Condition, bool) {
	if i, found := m.indexConds[name]; found {
		return m.conds[i], true
	}
	return metav1.Condition{}, false
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
	if i, found := m.indexConds[c.Type]; found {
		if !equalConditions(c, m.conds[i]) {
			m.conds[i] = c
		}
		return
	}
	m.conds = append(m.conds, c)
	m.indexConds[c.Type] = len(m.conds) - 1
}

func (m *Manager) Generate() []metav1.Condition {
	return slices.Clone(m.conds)
}

func equalConditions(c1, c2 metav1.Condition) bool {
	if c1.Type != c2.Type {
		return false
	}
	if c1.Status != c2.Status {
		return false
	}
	if c1.Reason != c2.Reason {
		return false
	}
	if c1.Message != c2.Message {
		return false
	}
	if c1.ObservedGeneration != c2.ObservedGeneration {
		return false
	}
	return true
}
