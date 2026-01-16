/*
Copyright 2025 Flant JSC

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

package readiness

import (
	"context"
	"fmt"
	"hooks/pkg/settings"

	"github.com/deckhouse/module-sdk/pkg"
	"github.com/deckhouse/module-sdk/pkg/app"
)

var ReadinessConfig = app.ReadinessConfig{
	ProbeFunc: checkModuleReadiness,
}

func checkModuleReadiness(ctx context.Context, input *pkg.HookInput) error {
	validationObj := input.Values.Get(settings.InternalValuesConfigValidationPath)
	if validationObj.IsObject() {
		validationErr := validationObj.Get("error")
		if validationErr.Exists() {
			return fmt.Errorf("%s", validationErr.String())
		}
		// moduleConfigValidation is present, but no errors. Something wrong.
		return fmt.Errorf("module is not ready yet")
	}
	// No module validation problems.
	// TODO add readiness checks for various hidden resources later: cdi config, kubevirt config, etc.
	return nil
}
