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

package api

import (
	"encoding/json"
	"fmt"
)

// ModuleSettings is a typed view of spec.settings of the virtualization ModuleConfig; the fields
// mirror openapi/config-values.yaml. Only the options read from Go code are declared here, the
// remaining ones are ignored by LoadModuleSettings.
type ModuleSettings struct {
	FeatureGates []string `json:"featureGates,omitempty"`
}

// LoadModuleSettings converts the untyped spec.settings into ModuleSettings instead of digging
// through the map by hand. Deckhouse validates spec.settings against the module OpenAPI schema
// before storing the ModuleConfig, so a conversion error means the settings do not match the
// schema of this module at all.
func LoadModuleSettings(values SettingsValues) (*ModuleSettings, error) {
	raw, err := json.Marshal(values)
	if err != nil {
		return nil, fmt.Errorf("marshal ModuleConfig settings: %w", err)
	}

	settings := &ModuleSettings{}
	if err = json.Unmarshal(raw, settings); err != nil {
		return nil, fmt.Errorf("unmarshal ModuleConfig settings: %w", err)
	}

	return settings, nil
}
