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

package validate_module_config_reporter

import (
	"context"
	"fmt"

	"hooks/pkg/settings"

	"github.com/deckhouse/module-sdk/pkg"
	"github.com/deckhouse/module-sdk/pkg/registry"
	"github.com/deckhouse/virtualization/api/core"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
)

const (
	snapshotModuleConfig = "module-config"
	moduleConfigJQFilter = `{"cidrs": .spec.settings.virtualMachineCIDRs}`

	snapshotNodes = "nodes"
	nodesJQFilter = `{"addresses": .status.addresses}`

	// see https://helm.sh/docs/howto/charts_tips_and_tricks/#tell-helm-not-to-uninstall-a-resource
	helmResourcePolicyKey  = "helm.sh/resource-policy"
	helmResourcePolicyKeep = "keep"
	apiVersion             = core.GroupName + "/" + v1alpha2.Version
)

var _ = registry.RegisterFunc(config, Reconcile)

var config = &pkg.HookConfig{
	Schedule: []pkg.ScheduleConfig{
		{
			Name:    "validate-module-config-reporter",
			Crontab: "*/15 * * * * *",
		},
	},
	Queue: fmt.Sprintf("modules/%s/validate-module-config-reporter", settings.ModuleName),
}

func Reconcile(_ context.Context, input *pkg.HookInput) error {
	readinessObj := input.Values.Get(settings.InternalValuesReadinessPath)
	if !readinessObj.IsObject() {
		return fmt.Errorf("module is not ready yet")
	}
	validationErr := readinessObj.Get("moduleConfigValidationError")
	if validationErr.Exists() {
		return fmt.Errorf(validationErr.String())
	}
	return nil
}
