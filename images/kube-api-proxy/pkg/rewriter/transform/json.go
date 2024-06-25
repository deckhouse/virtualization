package transform

import (
	"encoding/json"

	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
)

// Helpers for traversing JSON objects with support for root path.
// gjson supports @this, but sjson don't, so unique alias is used.

const Root = "@ROOT"

func GetBytes(obj []byte, path string) gjson.Result {
	if path == Root {
		return gjson.ParseBytes(obj)
	}
	return gjson.GetBytes(obj, path)
}

func SetBytes(obj []byte, path string, value interface{}) ([]byte, error) {
	if path == Root {
		return json.Marshal(value)
	}
	return sjson.SetBytes(obj, path, value)
}

func SetRawBytes(obj []byte, path string, value []byte) ([]byte, error) {
	if path == Root {
		return value, nil
	}
	return sjson.SetRawBytes(obj, path, value)
}
