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

import "k8s.io/apimachinery/pkg/api/resource"

func compareStrings(path, current, desired, defaultValue string, onChange ActionType) []FieldChange {
	currentValue := NewStringValue(current, defaultValue)
	desiredValue := NewStringValue(desired, defaultValue)
	isEqual := current == desired

	return compareValues(path, currentValue, desiredValue, isEqual, onChange)
}

func compareQuantity(path string, current, desired, defaultValue resource.Quantity, onChange ActionType) []FieldChange {
	currentValue := NewQuantityValue(current, defaultValue)
	desiredValue := NewQuantityValue(desired, defaultValue)
	isEqual := current.Equal(desired)

	return compareValues(path, currentValue, desiredValue, isEqual, onChange)
}

func compareInts(path string, current, desired, defaultValue int, onChange ActionType) []FieldChange {
	currentValue := NewIntValue(current, defaultValue)
	desiredValue := NewIntValue(desired, defaultValue)
	isEqual := current == desired
	return compareValues(path, currentValue, desiredValue, isEqual, onChange)
}

func comparePtrInt64(path string, current, desired *int64, defaultValue int64, onChange ActionType) []FieldChange {
	if current == nil && desired == nil {
		return nil
	}

	isEqual := isEqualPtrInt64(current, desired)
	currentValue := NewPtrInt64Value(current, defaultValue)
	desiredValue := NewPtrInt64Value(desired, defaultValue)

	return compareValues(path, currentValue, desiredValue, isEqual, onChange)
}

func compareBools(path string, current, desired, defaultValue bool, onChange ActionType) []FieldChange {
	currentValue := NewBoolValue(current, defaultValue)
	desiredValue := NewBoolValue(desired, defaultValue)
	isEqual := current == desired
	return compareValues(path, currentValue, desiredValue, isEqual, onChange)
}

// compareValues
// current == default, desired == zeroValue => operation remove, no action required
// current == zeroValue, desired == default => operation add, no action required
// current != desired => operation replace, onChange action is required
func compareValues(path string, currentValue, desiredValue Value, isEqual bool, onChange ActionType) []FieldChange {
	changes := compareEmpty(path, currentValue, desiredValue, onChange)
	// Consider operation ChangeNone as a stop. It'll be ignored later.
	if len(changes) > 0 {
		return changes
	}

	if !isEqual {
		return []FieldChange{
			{
				Operation:      ChangeReplace,
				Path:           path,
				CurrentValue:   currentValue.Value,
				DesiredValue:   desiredValue.Value,
				ActionRequired: onChange,
			},
		}
	}

	return nil
}

// compareEmpty returns a remove ar an add change or a none change if both values are empty.
func compareEmpty(path string, currentValue, desiredValue Value, onChange ActionType) []FieldChange {
	if currentValue.IsEmpty && desiredValue.IsEmpty {
		return []FieldChange{{Operation: ChangeNone}}
	}

	if currentValue.IsEmpty && !desiredValue.IsEmpty {
		// Default value changed to an empty value -> no action required.
		if desiredValue.IsDefault {
			onChange = ActionNone
		}
		return []FieldChange{
			{
				Operation:      ChangeAdd,
				Path:           path,
				DesiredValue:   desiredValue.Value,
				ActionRequired: onChange,
			},
		}
	}

	if !currentValue.IsEmpty && desiredValue.IsEmpty {
		// Empty value changed to default value -> no action required.
		if currentValue.IsDefault {
			onChange = ActionNone
		}
		return []FieldChange{
			{
				Operation:      ChangeRemove,
				Path:           path,
				CurrentValue:   currentValue.Value,
				ActionRequired: onChange,
			},
		}
	}

	return nil
}

type Value struct {
	Value     interface{}
	IsEmpty   bool
	IsDefault bool
}

func NewValue(value interface{}, isEmpty, isDefault bool) Value {
	return Value{
		Value:     value,
		IsEmpty:   isEmpty,
		IsDefault: isDefault,
	}
}

func NewStringValue(value, defaultValue string) Value {
	isEmpty := value == ""
	isDefault := !isEmpty && value == defaultValue
	return NewValue(value, isEmpty, isDefault)
}

func NewQuantityValue(value, defaultValue resource.Quantity) Value {
	isEmpty := value.IsZero()
	isDefault := !isEmpty && value.Cmp(defaultValue) == 0
	return NewValue(value, isEmpty, isDefault)
}

func NewIntValue(value, defaultValue int) Value {
	isEmpty := value == 0
	isDefault := !isEmpty && value == defaultValue
	return NewValue(value, isEmpty, isDefault)
}

func NewPtrInt64Value(value *int64, defaultValue int64) Value {
	isEmpty := value == nil
	isDefault := !isEmpty && *value == defaultValue
	return NewValue(value, isEmpty, isDefault)
}

func isEqualPtrInt64(a, b *int64) bool {
	if a == nil && b == nil {
		return true
	}
	if a != nil && b != nil && *a == *b {
		return true
	}
	return false
}

func NewBoolValue(value, defaultValue bool) Value {
	// Consider bool value is not empty.
	isDefault := value == defaultValue
	return NewValue(value, false, isDefault)
}

func NewPtrBoolValue(value *bool, defaultValue bool) Value {
	isEmpty := value == nil
	isDefault := !isEmpty && *value == defaultValue
	return NewValue(value, isEmpty, isDefault)
}

func NewPtrValue(value interface{}, isNil bool) Value {
	return NewValue(value, isNil, false)
}
