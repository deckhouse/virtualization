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

package vmchange

import "cmp"

type ChangeOperation string

const (
	ChangeNone    ChangeOperation = "none"
	ChangeAdd     ChangeOperation = "add"
	ChangeRemove  ChangeOperation = "remove"
	ChangeReplace ChangeOperation = "replace"
)

type ActionType string

const (
	ActionNone           ActionType = ""
	ActionRestart        ActionType = "Restart"
	ActionApplyImmediate ActionType = "ApplyImmediate"
)

var actionDisruptionOrder = map[ActionType]int{
	ActionNone:           0,
	ActionApplyImmediate: 1,
	ActionRestart:        2,
}

type FieldChange struct {
	Operation    ChangeOperation `json:"operation,omitempty"`
	Path         string          `json:"path,omitempty"`
	CurrentValue interface{}     `json:"currentValue,omitempty"`
	DesiredValue interface{}     `json:"desiredValue,omitempty"`

	ActionRequired ActionType `json:"-"`
	RestartMessage string     `json:"-"`
}

func HasChanges(changes []FieldChange) bool {
	for _, change := range changes {
		if change.Operation != ChangeNone {
			return true
		}
	}
	return false
}

// MostDisruptiveAction returns a most dangerous action from the list.
func MostDisruptiveAction(actions ...ActionType) ActionType {
	result := ActionNone
	for _, action := range actions {
		// Break immediately if 'action' is the most disruptive action.
		if action == ActionRestart {
			return action
		}
		if action.Cmp(result) == 1 {
			result = action
		}
	}
	return result
}

// Cmp returns 0 if the action is equal to 'other', -1 if the action is less harmless than 'other',
// or 1 if the action is more disruptive than 'other'.
func (a ActionType) Cmp(other ActionType) int {
	aOrder, hasA := actionDisruptionOrder[a]
	otherOrder, hasOther := actionDisruptionOrder[other]
	if hasA && hasOther {
		return cmp.Compare(aOrder, otherOrder)
	}

	// Should not reach here, but equal is safe.
	return 0
}
