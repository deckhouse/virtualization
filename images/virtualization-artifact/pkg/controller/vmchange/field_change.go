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

type FieldChange struct {
	Operation    ChangeOperation `json:"operation,omitempty"`
	Path         string          `json:"path,omitempty"`
	CurrentValue interface{}     `json:"currentValue,omitempty"`
	DesiredValue interface{}     `json:"desiredValue,omitempty"`

	ActionRequired ActionType `json:"-"`
}

func HasChanges(changes []FieldChange) bool {
	for _, change := range changes {
		if change.Operation != ChangeNone {
			return true
		}
	}
	return false
}
