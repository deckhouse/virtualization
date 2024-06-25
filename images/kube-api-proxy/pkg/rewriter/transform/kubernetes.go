package transform

import (
	"strings"

	"github.com/tidwall/gjson"
)

// ResourceOrList is a helper to transform a single resource or a list of resources.
// It assumes that obj has a "kind" field and a list kind has "List" suffix.
func ResourceOrList(obj []byte, transformFn func(singleObj []byte) ([]byte, error)) ([]byte, error) {
	kind := gjson.GetBytes(obj, "kind").String()
	if kind == "" {
		return obj, nil
	}
	if !strings.HasSuffix(kind, "List") {
		return transformFn(obj)
	}
	return Array(obj, "items", transformFn)
}

// Patch treats obj as a JSON patch or Merge patch and calls
// a corresponding transformFn.
func Patch(
	obj []byte,
	transformMerge func(mergePatch []byte) ([]byte, error),
	transformJSON func(jsonPatch []byte) ([]byte, error)) ([]byte, error) {
	if len(obj) == 0 {
		return obj, nil
	}
	// Merge patch for Kubernetes resource is always starts with the curly bracket.
	if string(obj[0]) == "{" && transformMerge != nil {
		return transformMerge(obj)
	}

	// JSON patch should start with the square bracket.
	if string(obj[0]) == "[" && transformJSON != nil {
		return Array(obj, Root, transformJSON)
	}

	// Return patch as-is in other cases.
	return obj, nil
}
