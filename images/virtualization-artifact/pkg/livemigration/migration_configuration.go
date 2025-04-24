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

package livemigration

import (
	"encoding/json"
	"fmt"
	"reflect"

	virtv1 "kubevirt.io/api/core/v1"

	"github.com/deckhouse/virtualization-controller/pkg/config"
)

// Live migration defaults from kubevirt

const (
	ParallelMigrationsPerClusterDefault uint32 = 5
	NodeDrainTaintDefaultKey            string = "kubevirt.io/drain"
	MigrationProgressTimeout            int64  = 150
	MigrationCompletionTimeoutPerGiB    int64  = 800
	DefaultUnsafeMigrationOverride      bool   = false
	MigrationAllowPostCopy              bool   = false
)

func NewMigrationConfiguration(moduleSettings config.LiveMigrationSettings, allowAutoConverge bool) *virtv1.MigrationConfiguration {
	parallelMigrationsPerClusterDefault := ParallelMigrationsPerClusterDefault
	parallelOutboundMigrationsPerNode := uint32(moduleSettings.MaxMigrationsPerNode)
	nodeDrainTaintDefaultKey := NodeDrainTaintDefaultKey
	bandwidthPerMigrationDefault := moduleSettings.BandwidthPerNode.DeepCopy()
	progressTimeout := MigrationProgressTimeout
	completionTimeoutPerGiB := MigrationCompletionTimeoutPerGiB
	defaultUnsafeMigrationOverride := DefaultUnsafeMigrationOverride
	allowPostCopy := MigrationAllowPostCopy

	return &virtv1.MigrationConfiguration{
		ParallelMigrationsPerCluster:      &parallelMigrationsPerClusterDefault,
		ParallelOutboundMigrationsPerNode: &parallelOutboundMigrationsPerNode,
		NodeDrainTaintKey:                 &nodeDrainTaintDefaultKey,
		BandwidthPerMigration:             &bandwidthPerMigrationDefault,
		ProgressTimeout:                   &progressTimeout,
		CompletionTimeoutPerGiB:           &completionTimeoutPerGiB,
		UnsafeMigrationOverride:           &defaultUnsafeMigrationOverride,
		AllowAutoConverge:                 &allowAutoConverge,
		AllowPostCopy:                     &allowPostCopy,
		DisableTLS:                        nil,
		Network:                           nil,
		MatchSELinuxLevelOnMigration:      nil,
	}
}

func DumpKVVMIMigrationConfiguration(kvvmi *virtv1.VirtualMachineInstance) string {
	if kvvmi.Status.MigrationState == nil {
		return "status.migrationState is null"
	}
	if kvvmi.Status.MigrationState.MigrationConfiguration == nil {
		return "status.migrationState.migrationConfiguration is null"
	}

	out, err := json.Marshal(kvvmi.Status.MigrationState.MigrationConfiguration)
	if err != nil {
		return fmt.Sprintf("(json err %v, fmt dump)%#v", err, kvvmi.Status.MigrationState.MigrationConfiguration)
	}
	return string(out)
}

// IsMigrationConfigurationChanged detects if MigrationConfiguration was changed.
func IsMigrationConfigurationChanged(current, changed *virtv1.VirtualMachineInstance) bool {
	// true if migrationConfiguration was added.
	if current.Status.MigrationState != nil && current.Status.MigrationState.MigrationConfiguration == nil &&
		changed.Status.MigrationState != nil && changed.Status.MigrationState.MigrationConfiguration != nil {
		return true
	}

	// Compare MigrationConfiguration. Handler may change options.
	if current.Status.MigrationState != nil && current.Status.MigrationState.MigrationConfiguration != nil &&
		changed.Status.MigrationState != nil && changed.Status.MigrationState.MigrationConfiguration != nil {
		return !reflect.DeepEqual(
			current.Status.MigrationState.MigrationConfiguration,
			changed.Status.MigrationState.MigrationConfiguration,
		)
	}

	return false
}
