/*
Copyright 2026 Flant JSC

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
	"slices"
	"strconv"
	"strings"
)

const (
	PatchReplaceOp = "replace"
	PatchAddOp     = "add"
	PatchRemoveOp  = "remove"
	PatchTestOp    = "test"
)

type JSONPatch struct {
	operations []JSONPatchOperation
}

type JSONPatchOperation struct {
	Op    string      `json:"op"`
	Path  string      `json:"path"`
	Value interface{} `json:"value,omitempty"`
}

func NewJSONPatch(patches ...JSONPatchOperation) *JSONPatch {
	return &JSONPatch{
		operations: patches,
	}
}

func NewJSONPatchOperation(op, path string, value interface{}) JSONPatchOperation {
	return JSONPatchOperation{
		Op:    op,
		Path:  path,
		Value: value,
	}
}

func WithAdd(path string, value interface{}) JSONPatchOperation {
	return NewJSONPatchOperation(PatchAddOp, path, value)
}

func WithRemove(path string) JSONPatchOperation {
	return NewJSONPatchOperation(PatchRemoveOp, path, nil)
}

func WithReplace(path string, value interface{}) JSONPatchOperation {
	return NewJSONPatchOperation(PatchReplaceOp, path, value)
}

func WithTest(path string, value interface{}) JSONPatchOperation {
	return NewJSONPatchOperation(PatchTestOp, path, value)
}

func (jp *JSONPatch) Operations() []JSONPatchOperation {
	return jp.operations
}

func (jp *JSONPatch) Append(patches ...JSONPatchOperation) {
	jp.operations = append(jp.operations, patches...)
}

func (jp *JSONPatch) Delete(op, path string) {
	jp.operations = slices.DeleteFunc(jp.operations, func(o JSONPatchOperation) bool {
		return o.Op == op && o.Path == path
	})
}

func (jp *JSONPatch) Len() int {
	return len(jp.operations)
}

func (jp *JSONPatch) String() (string, error) {
	bytes, err := jp.Bytes()
	if err != nil {
		return "", err
	}
	return string(bytes), nil
}

func (jp *JSONPatch) Bytes() ([]byte, error) {
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

type AsJsonString struct {
	Data interface{}
}

func (a AsJsonString) MarshalJSON() ([]byte, error) {
	b, err := json.Marshal(a.Data)
	if err != nil {
		return nil, err
	}
	return []byte(strconv.Quote(string(b))), nil
}
