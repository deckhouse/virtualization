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
	"slices"
)

type FilterFunc[T any] func(obj *T) (keep bool)

// Filter removes an item from the "objs" array if at least one of the "keeps" functions return false for the item.
func Filter[T any](objs []T, keeps ...FilterFunc[T]) []T {
	if len(keeps) == 0 {
		return slices.Clone(objs)
	}
	var filtered []T
loop:
	for _, o := range objs {
		for _, keep := range keeps {
			if !keep(&o) {
				continue loop
			}
		}
		filtered = append(filtered, o)
	}
	return filtered
}
