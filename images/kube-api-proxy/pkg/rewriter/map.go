package rewriter

import (
	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
)

func RewriteMapOfStrings(obj []byte, mapPath string, transformFn func(map[string]string) map[string]string) ([]byte, error) {
	m := gjson.GetBytes(obj, mapPath).Map()
	if len(m) == 0 {
		return obj, nil
	}
	newMap := make(map[string]string, len(m))
	for k, v := range m {
		newMap[k] = v.String()
	}
	newMap = transformFn(newMap)

	return sjson.SetBytes(obj, mapPath, newMap)
}

func RewriteMap(obj []byte, mapPath string, transformFn func(map[string]gjson.Result) interface{}) ([]byte, error) {
	m := gjson.GetBytes(obj, mapPath).Map()
	if len(m) == 0 {
		return obj, nil
	}
	newMap := transformFn(m)
	return sjson.SetBytes(obj, mapPath, newMap)
}
