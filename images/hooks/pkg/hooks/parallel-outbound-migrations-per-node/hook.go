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

package parallel_outbound_migrations_per_node

import (
	"context"
	"fmt"
	"hooks/pkg/settings"
	"strconv"

	"k8s.io/utils/ptr"

	"github.com/deckhouse/module-sdk/pkg"
	"github.com/deckhouse/module-sdk/pkg/registry"
)

const (
	snapshotModuleConfig           = "module-config"
	moduleConfigJQFilter           = `.metadata.annotations`
	migrationsPerNodeAnnotationKey = "virtualization.deckhouse.io/parallel-outbound-migrations-per-node"
	migrationsPerNodeValuesPath    = "virtualization.internal.virtConfig.parallelOutboundMigrationsPerNode"
	defaultMigrationsPerNode       = 1
)

var _ = registry.RegisterFunc(config, reconcile)

var config = &pkg.HookConfig{
	OnBeforeHelm: &pkg.OrderedConfig{Order: 10},
	Kubernetes: []pkg.KubernetesConfig{
		{
			Name:       snapshotModuleConfig,
			APIVersion: "deckhouse.io/v1alpha1",
			Kind:       "ModuleConfig",
			NameSelector: &pkg.NameSelector{
				MatchNames: []string{settings.ModuleName},
			},
			ExecuteHookOnSynchronization: ptr.To(true),
			ExecuteHookOnEvents:          ptr.To(true),
			JqFilter:                     moduleConfigJQFilter,
		},
	},

	Queue: fmt.Sprintf("modules/%s", settings.ModuleName),
}

func reconcile(_ context.Context, input *pkg.HookInput) error {
	parallelOutboundMigrationsPerNode, err := parallelOutboundMigrationsPerNodeFromSnapshot(input)
	if err != nil {
		return err
	}
	current := int(input.Values.Get(migrationsPerNodeValuesPath).Int())
	if current != parallelOutboundMigrationsPerNode {
		input.Values.Set(migrationsPerNodeValuesPath, parallelOutboundMigrationsPerNode)
	}
	return nil
}

func parallelOutboundMigrationsPerNodeFromSnapshot(input *pkg.HookInput) (int, error) {
	snap := input.Snapshots.Get(snapshotModuleConfig)
	if len(snap) < 1 {
		return -1, fmt.Errorf("moduleConfig is missing, something wrong with Deckhouse configuration")
	}

	var annos map[string]string
	err := snap[0].UnmarshalTo(&annos)
	if err != nil {
		return -1, fmt.Errorf("failed to unmarshal moduleConfig annotations: %w", err)
	}

	if val, ok := annos[migrationsPerNodeAnnotationKey]; ok {
		valInt, err := strconv.Atoi(val)
		if err != nil {
			return -1, fmt.Errorf("failed to parse %q annotation: %w", migrationsPerNodeAnnotationKey, err)
		}
		return valInt, nil
	}

	return defaultMigrationsPerNode, nil
}
