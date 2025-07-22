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

package restorer

import "errors"

var (
	ErrAlreadyExists           = errors.New("already exists")
	ErrAlreadyExistsAndHasDiff = errors.New("already exists and does not have the same data content")
	ErrAlreadyInUse            = errors.New("already in use")
	ErrRestoring               = errors.New("will be restored")
	ErrUpdating                = errors.New("will be updated")
	ErrIncomplete              = errors.New("still incomplete")
)
