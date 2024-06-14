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

package rewriter

import (
	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
)

// RewriteResourceOrList is a helper to transform a single resource or a list of resources.
func RewriteResourceOrList(payload []byte, listKind string, transformFn func(singleObj []byte) ([]byte, error)) ([]byte, error) {
	kind := gjson.GetBytes(payload, "kind").String()

	// Not a list, transform a single resource.
	if kind != listKind {
		return transformFn(payload)
	}

	return RewriteArray(payload, "items", transformFn)
}

func RewriteArray(obj []byte, arrayPath string, transformFn func(item []byte) ([]byte, error)) ([]byte, error) {
	// Transform each item in list. Put back original items if transformFn returns nil bytes.
	items := gjson.GetBytes(obj, arrayPath).Array()
	if len(items) == 0 {
		return obj, nil
	}
	rwrItems := []byte(`[]`)
	for _, item := range items {
		rwrItem, err := transformFn([]byte(item.Raw))
		if err != nil {
			return nil, err
		}
		// Put original item back.
		if rwrItem == nil {
			rwrItem = []byte(item.Raw)
		}
		rwrItems, err = sjson.SetRawBytes(rwrItems, "-1", rwrItem)
		if err != nil {
			return nil, err
		}
	}

	return sjson.SetRawBytes(obj, arrayPath, rwrItems)
}
