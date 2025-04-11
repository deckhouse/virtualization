package service

import (
	"encoding/json"
	"fmt"

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
	MigrationAllowAutoConverge          bool   = false
	MigrationAllowPostCopy              bool   = false
)

func NewMigrationConfiguration(moduleSettings config.LiveMigrationSettings) *virtv1.MigrationConfiguration {
	parallelMigrationsPerClusterDefault := ParallelMigrationsPerClusterDefault
	parallelOutboundMigrationsPerNode := uint32(moduleSettings.MaxMigrationsPerNode)
	nodeDrainTaintDefaultKey := NodeDrainTaintDefaultKey
	bandwidthPerMigrationDefault := moduleSettings.BandwidthPerNode.DeepCopy()
	progressTimeout := MigrationProgressTimeout
	completionTimeoutPerGiB := MigrationCompletionTimeoutPerGiB
	defaultUnsafeMigrationOverride := DefaultUnsafeMigrationOverride
	allowAutoConverge := MigrationAllowAutoConverge
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

func DumpMigrationConfiguration(conf *virtv1.MigrationConfiguration) string {
	out, err := json.Marshal(conf)
	if err != nil {
		return fmt.Sprintf("(json err %v, fmt dump)%#v", err, conf)
	}
	return string(out)
}
