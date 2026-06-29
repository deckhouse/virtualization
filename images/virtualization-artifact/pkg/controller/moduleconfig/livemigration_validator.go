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

package moduleconfig

import (
	"context"
	"fmt"

	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	mcapi "github.com/deckhouse/virtualization-controller/pkg/controller/moduleconfig/api"
)

const (
	liveMigrationField = "liveMigration"
	outboundField      = "outbound"
	perNodeField       = "perNode"
	syncPerNodeField   = "syncPerNode"
)

type liveMigrationValidator struct{}

func newLiveMigrationValidator() *liveMigrationValidator {
	return &liveMigrationValidator{}
}

func (v liveMigrationValidator) ValidateUpdate(_ context.Context, _, newMC *mcapi.ModuleConfig) (admission.Warnings, error) {
	return nil, validateLiveMigrationOutbound(newMC.Spec.Settings)
}

func validateLiveMigrationOutbound(settings mcapi.SettingsValues) error {
	liveMigration, ok := settings[liveMigrationField].(map[string]interface{})
	if !ok {
		return nil
	}
	outbound, ok := liveMigration[outboundField].(map[string]interface{})
	if !ok {
		return nil
	}

	perNode, perNodeOK := settingsInt64(outbound[perNodeField])
	syncPerNode, syncOK := settingsInt64(outbound[syncPerNodeField])
	if !perNodeOK || !syncOK {
		return nil
	}

	if syncPerNode > perNode {
		return fmt.Errorf(
			"liveMigration.outbound.syncPerNode (%d) must not exceed liveMigration.outbound.perNode (%d)",
			syncPerNode, perNode,
		)
	}

	return nil
}

func settingsInt64(v interface{}) (int64, bool) {
	switch n := v.(type) {
	case int64:
		return n, true
	case int:
		return int64(n), true
	case float64:
		return int64(n), true
	default:
		return 0, false
	}
}
