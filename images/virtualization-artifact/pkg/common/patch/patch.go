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

package patch

import (
	"encoding/json"
	"fmt"
	"strings"
)

const (
	PatchReplaceOp = "replace"
	PatchAddOp     = "add"
	PatchRemoveOp  = "remove"
	PatchTestOp    = "test"
)

type JsonPatch struct {
	operations []JsonPatchOperation
}

type JsonPatchOperation struct {
	Op    string      `json:"op"`
	Path  string      `json:"path"`
	Value interface{} `json:"value,omitempty"`
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

func WithAdd(path string, value interface{}) JsonPatchOperation {
	return NewJsonPatchOperation(PatchAddOp, path, value)
}

func WithRemove(path string) JsonPatchOperation {
	return NewJsonPatchOperation(PatchRemoveOp, path, nil)
}

func WithReplace(path string, value interface{}) JsonPatchOperation {
	return NewJsonPatchOperation(PatchReplaceOp, path, value)
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

func EscapeJSONPointer(path string) string {
	path = strings.ReplaceAll(path, "~", "~0")
	path = strings.ReplaceAll(path, "/", "~1")
	return path
}
