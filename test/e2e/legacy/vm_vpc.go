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

package legacy

import (
	"github.com/deckhouse/virtualization/test/e2e/internal/framework"
)

// isSdnModuleEnabled is used by legacy restore tests (vm_restore_safe, vm_restore_force).
func isSdnModuleEnabled() (bool, error) {
	sdnModule, err := framework.NewFramework("").GetModuleConfig("sdn")
	if err != nil {
		return false, err
	}
	enabled := sdnModule.Spec.Enabled

	return enabled != nil && *enabled, nil
}
