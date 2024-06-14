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

package util

import (
	"fmt"
	"math"
)

func CopyByPointer[T any](objP *T) *T {
	copyObj := *objP
	return &copyObj
}

func ToPointersArray[T any](items []T) (res []*T) {
	for _, item := range items {
		res = append(res, GetPointer(item))
	}
	return
}

func GetPointer[T any](obj T) *T {
	return &obj
}

func IsEmpty[T comparable](v T) bool {
	var empty T
	return v == empty
}

// SetArrayElem performs idempotent insert of new elem or optionally replace if it exists
func SetArrayElem[T any](elems []T, newElem T, matchFunc func(v1, v2 T) bool, replaceExisting bool) (res []T) {
	isFound := false
	for _, elem := range elems {
		if matchFunc(elem, newElem) {
			if replaceExisting {
				res = append(res, newElem)
			} else {
				res = append(res, elem)
			}
			isFound = true
		} else {
			res = append(res, elem)
		}
	}
	if !isFound {
		res = append(res, newElem)
	}
	return
}

func BoolFloat64(b bool) float64 {
	if b {
		return 1
	}
	return 0
}

func HumanizeIBytes(s uint64) string {
	sizes := []string{"B", "Ki", "Mi", "Gi", "Ti", "Pi", "Ei"}
	return humanateBytes(s, 1024, sizes)
}

func humanateBytes(s uint64, base float64, sizes []string) string {
	if s < 10 {
		return fmt.Sprintf("%dB", s)
	}
	e := math.Floor(logn(float64(s), base))

	suffix := sizes[int(e)]
	val := math.Floor(float64(s)/math.Pow(base, e)*10+0.5) / 10
	_, frac := math.Modf(val)
	f := "%.0f%s"

	if frac > 0 {
		f = "%.1f%s"
	}

	return fmt.Sprintf(f, val, suffix)
}

func logn(n, b float64) float64 {
	return math.Log(n) / math.Log(b)
}
