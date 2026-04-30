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

package livemigration

import (
	"testing"

	"github.com/stretchr/testify/require"
	"k8s.io/utils/ptr"
	virtv1 "kubevirt.io/api/core/v1"
)

func TestNewMigrationConfiguration_ParallelSyncDefault(t *testing.T) {
	kv := virtv1.KubeVirt{}
	cfg := NewMigrationConfiguration(false, kv)

	require.NotNil(t, cfg.ParallelOutboundMigrationsPerNode)
	require.Equal(t, ParallelOutboundMigrationsPerNodeDefault, *cfg.ParallelOutboundMigrationsPerNode)
	require.NotNil(t, cfg.ParallelSyncMigrationsPerNode)
	require.Equal(t, ParallelSyncMigrationsPerNodeDefault, *cfg.ParallelSyncMigrationsPerNode)
}

func TestNewMigrationConfiguration_ParallelSyncPassthrough(t *testing.T) {
	kv := virtv1.KubeVirt{
		Spec: virtv1.KubeVirtSpec{
			Configuration: virtv1.KubeVirtConfiguration{
				MigrationConfiguration: &virtv1.MigrationConfiguration{
					ParallelOutboundMigrationsPerNode: ptr.To[uint32](4),
					ParallelSyncMigrationsPerNode:     ptr.To[uint32](2),
				},
			},
		},
	}
	cfg := NewMigrationConfiguration(false, kv)
	require.Equal(t, uint32(4), *cfg.ParallelOutboundMigrationsPerNode)
	require.Equal(t, uint32(2), *cfg.ParallelSyncMigrationsPerNode)
}

func TestNewMigrationConfiguration_ParallelSyncClampedToOutbound(t *testing.T) {
	kv := virtv1.KubeVirt{
		Spec: virtv1.KubeVirtSpec{
			Configuration: virtv1.KubeVirtConfiguration{
				MigrationConfiguration: &virtv1.MigrationConfiguration{
					ParallelOutboundMigrationsPerNode: ptr.To[uint32](1),
					ParallelSyncMigrationsPerNode:     ptr.To[uint32](5),
				},
			},
		},
	}
	cfg := NewMigrationConfiguration(false, kv)
	require.Equal(t, uint32(1), *cfg.ParallelOutboundMigrationsPerNode)
	require.Equal(t, uint32(1), *cfg.ParallelSyncMigrationsPerNode,
		"sync cap must be clamped to outbound cap")
}
