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
