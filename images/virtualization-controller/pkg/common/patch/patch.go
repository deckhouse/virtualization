package patch

import (
	"encoding/json"
	"fmt"
)

const (
	PatchReplaceOp = "replace"
	PatchAddOp     = "add"
	PatchRemoveOp  = "remove"
)

type JsonPatch struct {
	operations []JsonPatchOperation
}

type JsonPatchOperation struct {
	Op    string      `json:"op"`
	Path  string      `json:"path"`
	Value interface{} `json:"value"`
}

func NewJsonPatch(patches ...JsonPatchOperation) *JsonPatch {
	return &JsonPatch{
		operations: patches,
	}
}

func NewJsonPatchOperation(op, path string, value interface{}) JsonPatchOperation {
	return JsonPatchOperation{
		Op:    op,
		Path:  path,
		Value: value,
	}
}

func (jp *JsonPatch) Operations() []JsonPatchOperation {
	return jp.operations
}

func (jp *JsonPatch) Append(patches ...JsonPatchOperation) {
	jp.operations = append(jp.operations, patches...)
}

func (jp *JsonPatch) Delete(op, path string) {
	var idx int
	var found bool
	for i, o := range jp.operations {
		if o.Op == op && o.Path == path {
			idx = i
			found = true
			break
		}
	}
	if found {
		jp.operations = append(jp.operations[:idx], jp.operations[idx+1:]...)
	}
}

func (jp *JsonPatch) Len() int {
	return len(jp.operations)
}

func (jp *JsonPatch) String() (string, error) {
	bytes, err := jp.Bytes()
	if err != nil {
		return "", err
	}
	return string(bytes), nil
}

func (jp *JsonPatch) Bytes() ([]byte, error) {
	if jp.Len() == 0 {
		return nil, fmt.Errorf("list of patches is empty")
	}
	return json.Marshal(jp.operations)
}
