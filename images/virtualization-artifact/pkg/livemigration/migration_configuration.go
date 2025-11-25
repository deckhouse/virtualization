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

	"k8s.io/apimachinery/pkg/api/equality"
	"k8s.io/apimachinery/pkg/api/resource"
	virtv1 "kubevirt.io/api/core/v1"

	"github.com/deckhouse/virtualization-controller/pkg/common/patch"
)

// Live migration defaults from kubevirt

const (
	ParallelOutboundMigrationsPerNodeDefault uint32 = 2
	ParallelMigrationsPerClusterDefault      uint32 = 5
	BandwidthPerMigrationDefault                    = "0Mi"
	NodeDrainTaintDefaultKey                 string = "kubevirt.io/drain"
	MigrationProgressTimeout                 int64  = 150
	MigrationCompletionTimeoutPerGiB         int64  = 800
	DefaultUnsafeMigrationOverride           bool   = false
	MigrationAllowPostCopy                   bool   = false
	MigrationAllowWorkloadDisruption         bool   = false
)

func NewMigrationConfiguration(allowAutoConverge bool, kvconfig virtv1.KubeVirt) *virtv1.MigrationConfiguration {
	// TODO rework below section after proper implementation of liveMigration settings in ModuleConfig.
	parallelMigrationsPerCluster := ParallelMigrationsPerClusterDefault
	if kvconfig.Spec.Configuration.MigrationConfiguration != nil && kvconfig.Spec.Configuration.MigrationConfiguration.ParallelMigrationsPerCluster != nil {
		parallelMigrationsPerCluster = *kvconfig.Spec.Configuration.MigrationConfiguration.ParallelMigrationsPerCluster
	}
	// Reuse default value of MaxMigrationsPerNode as parallelOutboundMigrationsPerNode.
	parallelOutboundMigrationsPerNode := ParallelOutboundMigrationsPerNodeDefault
	if kvconfig.Spec.Configuration.MigrationConfiguration != nil && kvconfig.Spec.Configuration.MigrationConfiguration.ParallelOutboundMigrationsPerNode != nil {
		parallelOutboundMigrationsPerNode = *kvconfig.Spec.Configuration.MigrationConfiguration.ParallelOutboundMigrationsPerNode
	}
	// Reuse default value of BandwidthPerNode as bandwidthPerMigration.
	bandwidthPerMigration := resource.MustParse(BandwidthPerMigrationDefault)
	if kvconfig.Spec.Configuration.MigrationConfiguration != nil && kvconfig.Spec.Configuration.MigrationConfiguration.BandwidthPerMigration != nil {
		bandwidthPerMigration = kvconfig.Spec.Configuration.MigrationConfiguration.BandwidthPerMigration.DeepCopy()
	}

	// Just use defaults from KubeVirt.
	nodeDrainTaintDefaultKey := NodeDrainTaintDefaultKey
	progressTimeout := MigrationProgressTimeout
	completionTimeoutPerGiB := MigrationCompletionTimeoutPerGiB
	defaultUnsafeMigrationOverride := DefaultUnsafeMigrationOverride
	allowPostCopy := MigrationAllowPostCopy
	// test comment
	allowWorkloadDisruption := MigrationAllowWorkloadDisruption

	return &virtv1.MigrationConfiguration{
		ParallelMigrationsPerCluster:      &parallelMigrationsPerCluster,
		ParallelOutboundMigrationsPerNode: &parallelOutboundMigrationsPerNode,
		NodeDrainTaintKey:                 &nodeDrainTaintDefaultKey,
		BandwidthPerMigration:             &bandwidthPerMigration,
		ProgressTimeout:                   &progressTimeout,
		CompletionTimeoutPerGiB:           &completionTimeoutPerGiB,
		UnsafeMigrationOverride:           &defaultUnsafeMigrationOverride,
		AllowAutoConverge:                 &allowAutoConverge,
		AllowPostCopy:                     &allowPostCopy,
		AllowWorkloadDisruption:           &allowWorkloadDisruption,
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

func GenerateMigrationConfigurationPatch(current, changed *virtv1.VirtualMachineInstance) ([]byte, error) {
	if current.Status.MigrationState == nil || changed.Status.MigrationState == nil {
		return nil, nil
	}
	currentConf := current.Status.MigrationState.MigrationConfiguration
	changedConf := changed.Status.MigrationState.MigrationConfiguration

	if equality.Semantic.DeepEqual(currentConf, changedConf) {
		return nil, nil
	}

	op := patch.PatchReplaceOp
	if currentConf == nil {
		op = patch.PatchAddOp
	}

	return patch.NewJSONPatch(patch.NewJSONPatchOperation(op, "/status/migrationState/migrationConfiguration", changedConf)).Bytes()
}
