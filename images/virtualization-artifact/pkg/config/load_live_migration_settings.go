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

package config

import (
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
)

// TODO(future) live migration settings will be here. Now just a place for the default policy.

const (
	DefaultLiveMigrationPolicy = v1alpha2.PreferSafeMigrationPolicy
)

var systemMigrationPolicyOverride v1alpha2.LiveMigrationPolicy

func SetSystemMigrationPolicyOverride(rawPolicy string) bool {
	policy := v1alpha2.LiveMigrationPolicy(rawPolicy)
	if !isValidLiveMigrationPolicy(policy) {
		systemMigrationPolicyOverride = ""
		return false
	}
	systemMigrationPolicyOverride = policy
	return true
}

func GetSystemMigrationPolicyOverride() (v1alpha2.LiveMigrationPolicy, bool) {
	if systemMigrationPolicyOverride == "" {
		return "", false
	}
	return systemMigrationPolicyOverride, true
}

func ResetSystemMigrationPolicyOverride() {
	systemMigrationPolicyOverride = ""
}

func isValidLiveMigrationPolicy(policy v1alpha2.LiveMigrationPolicy) bool {
	switch policy {
	case v1alpha2.ManualMigrationPolicy,
		v1alpha2.NeverMigrationPolicy,
		v1alpha2.AlwaysSafeMigrationPolicy,
		v1alpha2.PreferSafeMigrationPolicy,
		v1alpha2.AlwaysForcedMigrationPolicy,
		v1alpha2.PreferForcedMigrationPolicy:
		return true
	default:
		return false
	}
}
