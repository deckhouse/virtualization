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

package common

import (
	"errors"
	"strings"
)

var (
	// ErrUnknownValue is a variable of type `error` that represents an error message indicating an unknown value.
	ErrUnknownValue = errors.New("unknown value")
	// ErrUnknownType is a variable of type `error` that represents an error message indicating an unknown type.
	ErrUnknownType = errors.New("unknown type")
)

// ErrQuotaExceeded checked is the error is of exceeded quota
func ErrQuotaExceeded(err error) bool {
	return strings.Contains(err.Error(), "exceeded quota:")
}

func BoolFloat64(b bool) float64 {
	if b {
		return 1
	}
	return 0
}
