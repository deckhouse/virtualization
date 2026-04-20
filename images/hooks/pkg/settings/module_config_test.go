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

package settings

import (
	"testing"

	"github.com/tidwall/gjson"

	"github.com/deckhouse/module-sdk/pkg"
	"github.com/deckhouse/module-sdk/testing/mock"
)

func TestHasModuleConfig(t *testing.T) {
	newInput := func(values *mock.OutputPatchableValuesCollectorMock) *pkg.HookInput {
		return &pkg.HookInput{Values: values}
	}

	t.Run("returns false when moduleConfig is absent", func(t *testing.T) {
		values := mock.NewPatchableValuesCollectorMock(t)
		values.GetMock.When(InternalValuesConfigCopyPath).Then(gjson.Result{})

		if HasModuleConfig(newInput(values)) {
			t.Fatalf("expected HasModuleConfig to return false")
		}
	})

	t.Run("returns false when moduleConfig is not an object", func(t *testing.T) {
		values := mock.NewPatchableValuesCollectorMock(t)
		values.GetMock.When(InternalValuesConfigCopyPath).Then(gjson.Result{Type: gjson.String, Str: "value"})

		if HasModuleConfig(newInput(values)) {
			t.Fatalf("expected HasModuleConfig to return false")
		}
	})

	t.Run("returns false when moduleConfig is an empty object", func(t *testing.T) {
		values := mock.NewPatchableValuesCollectorMock(t)
		values.GetMock.When(InternalValuesConfigCopyPath).Then(gjson.Parse(`{}`))

		if HasModuleConfig(newInput(values)) {
			t.Fatalf("expected HasModuleConfig to return false")
		}
	})

	t.Run("returns true when moduleConfig is a non-empty object", func(t *testing.T) {
		values := mock.NewPatchableValuesCollectorMock(t)
		values.GetMock.When(InternalValuesConfigCopyPath).Then(gjson.Parse(`{"dvcr":{},"virtualMachineCIDRs":["10.0.0.0/24"]}`))

		if !HasModuleConfig(newInput(values)) {
			t.Fatalf("expected HasModuleConfig to return true")
		}
	})
}
