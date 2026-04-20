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

package tls_certificates_dvcr

import (
	"testing"

	"github.com/tidwall/gjson"

	"github.com/deckhouse/module-sdk/pkg"
	"github.com/deckhouse/module-sdk/testing/mock"
	"github.com/deckhouse/virtualization/hooks/pkg/settings"
)

func TestBeforeHookCheckSkipsWithoutModuleConfig(t *testing.T) {
	values := mock.NewPatchableValuesCollectorMock(t)
	values.GetMock.When(settings.InternalValuesConfigCopyPath).Then(gjson.Result{})

	input := &pkg.HookInput{Values: values}
	if settings.HasModuleConfig(input) {
		t.Fatalf("expected copied module config to be absent")
	}
}
