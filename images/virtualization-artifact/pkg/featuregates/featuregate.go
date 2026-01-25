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

package featuregates

import (
	"github.com/spf13/pflag"
	"k8s.io/component-base/featuregate"

	"github.com/deckhouse/virtualization-controller/pkg/version"
)

const (
	SDN                                 featuregate.Feature = "SDN"
	AutoMigrationIfNodePlacementChanged featuregate.Feature = "AutoMigrationIfNodePlacementChanged"
	VolumeMigration                     featuregate.Feature = "VolumeMigration"
	TargetMigration                     featuregate.Feature = "TargetMigration"
)

var featureSpecs = map[featuregate.Feature]featuregate.FeatureSpec{
	SDN: {
		Default:    false,
		PreRelease: featuregate.Alpha,
	},
	AutoMigrationIfNodePlacementChanged: {
		Default:       version.GetEdition() == version.EditionEE,
		LockToDefault: true,
		PreRelease:    featuregate.Alpha,
	},
	VolumeMigration: {
		Default:       version.GetEdition() == version.EditionEE,
		LockToDefault: true,
		PreRelease:    featuregate.Alpha,
	},
	TargetMigration: {
		Default:       version.GetEdition() == version.EditionEE,
		LockToDefault: true,
		PreRelease:    featuregate.Alpha,
	},
}

var (
	instance featuregate.FeatureGate
	addFlags func(fs *pflag.FlagSet)
)

func init() {
	gate, gateAddFlags, _, err := New()
	if err != nil {
		panic(err)
	}
	instance = gate
	addFlags = gateAddFlags
}

func AddFlags(fs *pflag.FlagSet) {
	addFlags(fs)
}

func Default() featuregate.FeatureGate {
	return instance
}

type (
	AddFlagsFunc   func(fs *pflag.FlagSet)
	SetFromMapFunc func(m map[string]bool) error
)

func New() (featuregate.FeatureGate, AddFlagsFunc, SetFromMapFunc, error) {
	gate := featuregate.NewFeatureGate()
	if err := gate.Add(featureSpecs); err != nil {
		return nil, nil, nil, err
	}
	return gate, gate.AddFlag, gate.SetFromMap, nil
}
