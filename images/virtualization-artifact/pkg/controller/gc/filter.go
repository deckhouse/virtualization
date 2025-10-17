/*
Copyright 2025 Flant JSC

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

package gc

import (
	"cmp"
	"slices"
	"time"

	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/deckhouse/virtualization-controller/pkg/common/object"
)

type (
	IsCandidate func(obj client.Object) bool
	IndexFunc   func(obj client.Object) string
)

func DefaultFilter(objs []client.Object, isCandidate IsCandidate, ttl time.Duration, indexFunc IndexFunc, maxCount int, now time.Time) []client.Object {
	var (
		expired    []client.Object
		nonExpired []client.Object
	)

	for _, obj := range objs {
		isCandidate := isCandidate(obj)
		if !isCandidate {
			continue
		}

		if object.IsTerminating(obj) {
			continue
		}

		if getAge(obj, now) > ttl {
			expired = append(expired, obj)
			continue
		}

		nonExpired = append(nonExpired, obj)
	}

	result := expired
	result = append(result, KeepLastRemoveOld(nonExpired, indexFunc, maxCount, now)...)

	return result
}

func KeepLastRemoveOld(objs []client.Object, indexFunc IndexFunc, maxCount int, now time.Time) []client.Object {
	if maxCount <= 0 {
		return nil
	}

	slices.SortFunc(objs, func(a, b client.Object) int {
		return cmp.Compare(getAge(a, now), getAge(b, now))
	})

	var result []client.Object

	indexed := make(map[string]int)
	for _, obj := range objs {
		index := indexFunc(obj)
		count := indexed[index]
		if count >= maxCount {
			result = append(result, obj)
		}
		indexed[index]++
	}

	return result
}

func getAge(obj client.Object, now time.Time) time.Duration {
	return now.Sub(obj.GetCreationTimestamp().Time).Truncate(time.Second)
}
