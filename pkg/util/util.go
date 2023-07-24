package util

func CopyByPointer[T any](objP *T) *T {
	copyObj := *objP
	return &copyObj
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
