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

package version

import (
	_ "embed"
	"fmt"

	"gopkg.in/yaml.v3"
)

var firmwareInstance firmwareConfig

//go:embed version_map.yml
var embeddedConfig string

type firmwareConfig struct {
	Version             Version `yaml:"version"`
	MinSupportedVersion Version `yaml:"minSupportedVersion"`
}

func (f firmwareConfig) Validate() error {
	if !f.Version.IsValid() {
		return fmt.Errorf("firmware version is invalid")
	}
	if !f.MinSupportedVersion.IsValid() {
		return fmt.Errorf("firmware minimum supported version is invalid")
	}
	return nil
}

func init() {
	if err := yaml.Unmarshal([]byte(embeddedConfig), &firmwareInstance); err != nil {
		panic("failed to load embedded firmwareConf: " + err.Error())
	}
	if err := firmwareInstance.Validate(); err != nil {
		panic("failed to validate embedded firmwareConf: " + err.Error())
	}
}
