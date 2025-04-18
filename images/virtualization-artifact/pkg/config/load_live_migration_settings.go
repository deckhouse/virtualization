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
	"fmt"
	"os"
	"strconv"

	"k8s.io/apimachinery/pkg/api/resource"
)

const (
	LiveMigrationBandwidthPerNodeEnv       = "LIVE_MIGRATION_BANDWIDTH_PER_NODE"
	LiveMigrationMaxMigrationsPerNodeEnv   = "LIVE_MIGRATION_MAX_MIGRATIONS_PER_NODE"
	LiveMigrationNetworkEnv                = "LIVE_MIGRATION_NETWORK"
	LiveMigrationDedicatedInterfaceNameEnv = "LIVE_MIGRATION_DEDICATED_INTERFACE_NAME"
)

type LiveMigrationNetwork string

// These constants should be in sync with liveMigration.network field enum in config-values.yaml
const (
	LiveMigrationNetworkShared    LiveMigrationNetwork = "Shared"
	LiveMigrationNetworkDedicated LiveMigrationNetwork = "Dedicated"
)

const (
	DefaultBandwidthPerNode     = "64Mi"
	DefaultMaxMigrationsPerNode = 2
	DefaultNetwork              = LiveMigrationNetworkShared
)

type LiveMigrationSettings struct {
	BandwidthPerNode       resource.Quantity
	MaxMigrationsPerNode   int
	Network                LiveMigrationNetwork
	DedicatedInterfaceName string
}

func LoadLiveMigrationSettings() (LiveMigrationSettings, error) {
	settings := NewDefaultLiveMigrationSettings()

	if val, ok := os.LookupEnv(LiveMigrationBandwidthPerNodeEnv); ok && val != "" {
		bandwidth, err := resource.ParseQuantity(val)
		if err != nil {
			return settings, fmt.Errorf("parse %s: %s should be bandwidth quantity: %w", LiveMigrationBandwidthPerNodeEnv, val, err)
		}
		settings.BandwidthPerNode = bandwidth
	}

	if val, ok := os.LookupEnv(LiveMigrationMaxMigrationsPerNodeEnv); ok && val != "" {
		maxPerNode, err := strconv.ParseUint(val, 10, 32)
		if err != nil {
			return settings, fmt.Errorf("parse %s: %s should be numberic value: %w", LiveMigrationMaxMigrationsPerNodeEnv, val, err)
		}
		settings.MaxMigrationsPerNode = int(maxPerNode)
	}

	if val, ok := os.LookupEnv(LiveMigrationNetworkEnv); ok && val != "" {
		switch val {
		case string(LiveMigrationNetworkShared):
			settings.Network = LiveMigrationNetworkShared
		case string(LiveMigrationNetworkDedicated):
			settings.Network = LiveMigrationNetworkDedicated
		default:
			return settings, fmt.Errorf("parse %s: %s is unsupported value, should be one of [%s, %s]",
				LiveMigrationNetworkEnv, val,
				LiveMigrationNetworkShared, LiveMigrationNetworkDedicated,
			)
		}
	}

	if val, ok := os.LookupEnv(LiveMigrationDedicatedInterfaceNameEnv); ok && val != "" {
		settings.DedicatedInterfaceName = val
	}

	return settings, nil
}

// defaultBandwidthPerNode is an error eater for hardcoded quantity.
// Parsing of DefaultBandwidthPerNode should be tested.
func defaultBandwidthPerNode() resource.Quantity {
	bandwidth, _ := resource.ParseQuantity(DefaultBandwidthPerNode)
	return bandwidth
}

func NewDefaultLiveMigrationSettings() LiveMigrationSettings {
	return LiveMigrationSettings{
		BandwidthPerNode:     defaultBandwidthPerNode(),
		MaxMigrationsPerNode: DefaultMaxMigrationsPerNode,
		Network:              DefaultNetwork,
	}
}
