package common

import (
	"slices"
)

type FilterFunc[T any] func(obj *T) (skip bool)

func Filter[T any](objs []T, filters ...FilterFunc[T]) []T {
	if len(filters) == 0 {
		return slices.Clone(objs)
	}
	var filtered []T
loop:
	for _, o := range objs {
		for _, f := range filters {
			if f(&o) {
				continue loop
			}
		}
		filtered = append(filtered, o)
	}
	return filtered
}
