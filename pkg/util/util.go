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
