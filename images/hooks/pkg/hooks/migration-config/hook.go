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

package migration_config

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"k8s.io/utils/ptr"

	"github.com/deckhouse/module-sdk/pkg"
	"github.com/deckhouse/module-sdk/pkg/registry"
	"github.com/deckhouse/virtualization/hooks/pkg/settings"
)

const (
	snapshotModuleConfig = "module-config"
	moduleConfigJQFilter = `.metadata.annotations`

	bandwidthPerMigrationAnnotation             = "virtualization.deckhouse.io/bandwidth-per-migration"
	completionTimeoutPerGiBAnnotation           = "virtualization.deckhouse.io/completion-timeout-per-gib"
	parallelOutboundMigrationsPerNodeAnnotation = "virtualization.deckhouse.io/parallel-outbound-migrations-per-node"
	parallelSyncMigrationsPerNodeAnnotation     = "virtualization.deckhouse.io/parallel-sync-migrations-per-node"
	progressTimeoutAnnotation                   = "virtualization.deckhouse.io/progress-timeout"
	disableTLSAnnotation                        = "virtualization.deckhouse.io/disable-tls"

	bandwidthPerMigrationValuesPath             = "virtualization.internal.virtConfig.bandwidthPerMigration"
	completionTimeoutPerGiBValuesPath           = "virtualization.internal.virtConfig.completionTimeoutPerGiB"
	parallelOutboundMigrationsPerNodeValuesPath = "virtualization.internal.virtConfig.parallelOutboundMigrationsPerNode"
	parallelSyncMigrationsPerNodeValuesPath     = "virtualization.internal.virtConfig.parallelSyncMigrationsPerNode"
	progressTimeoutValuesPath                   = "virtualization.internal.virtConfig.progressTimeout"
	disableTLSValuesPath                        = "virtualization.internal.virtConfig.disableTLS"

	defaultBandwidthPerMigration             = "640Mi"
	defaultCompletionTimeoutPerGiB           = 800
	defaultParallelOutboundMigrationsPerNode = 2
	defaultParallelSyncMigrationsPerNode     = 1
	defaultProgressTimeout                   = 150
	defaultDisableTLS                        = false
)

// migrationParams defines migration parameters configurable via ModuleConfig annotations.
// parallelMigrationsPerCluster is intentionally excluded: it is managed by the
// discovery-workload-nodes hook which reads the actual value from the KubeVirt config.
var migrationParams = []migrationParam{
	{
		annotation:   bandwidthPerMigrationAnnotation,
		valuesPath:   bandwidthPerMigrationValuesPath,
		defaultValue: defaultBandwidthPerMigration,
	},
	{
		annotation:   completionTimeoutPerGiBAnnotation,
		valuesPath:   completionTimeoutPerGiBValuesPath,
		defaultValue: defaultCompletionTimeoutPerGiB,
	},
	{
		annotation:   parallelOutboundMigrationsPerNodeAnnotation,
		valuesPath:   parallelOutboundMigrationsPerNodeValuesPath,
		defaultValue: defaultParallelOutboundMigrationsPerNode,
	},
	{
		annotation:   parallelSyncMigrationsPerNodeAnnotation,
		valuesPath:   parallelSyncMigrationsPerNodeValuesPath,
		defaultValue: defaultParallelSyncMigrationsPerNode,
	},
	{
		annotation:   progressTimeoutAnnotation,
		valuesPath:   progressTimeoutValuesPath,
		defaultValue: defaultProgressTimeout,
	},
	{
		annotation:   disableTLSAnnotation,
		valuesPath:   disableTLSValuesPath,
		defaultValue: defaultDisableTLS,
	},
}

type migrationParam struct {
	annotation   string
	valuesPath   string
	defaultValue any
}

func (p migrationParam) resolve(annos map[string]string) (any, error) {
	val, ok := annos[p.annotation]
	if !ok {
		return p.defaultValue, nil
	}

	switch p.defaultValue.(type) {
	case bool:
		v, err := strconv.ParseBool(strings.ToLower(val))
		if err != nil {
			return nil, fmt.Errorf("failed to parse %q annotation: %w", p.annotation, err)
		}
		return v, nil
	case int:
		v, err := strconv.Atoi(val)
		if err != nil {
			return nil, fmt.Errorf("failed to parse %q annotation: %w", p.annotation, err)
		}
		return v, nil
	case string:
		return val, nil
	default:
		return nil, fmt.Errorf("unsupported default value type for %q annotation", p.annotation)
	}
}

func (p migrationParam) getCurrent(input *pkg.HookInput) any {
	switch p.defaultValue.(type) {
	case bool:
		return input.Values.Get(p.valuesPath).Bool()
	case int:
		return int(input.Values.Get(p.valuesPath).Int())
	case string:
		return input.Values.Get(p.valuesPath).String()
	default:
		return nil
	}
}

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
	annos, err := annotationsFromSnapshot(input)
	if err != nil {
		return err
	}

	for _, param := range migrationParams {
		value, err := param.resolve(annos)
		if err != nil {
			return err
		}
		if current := param.getCurrent(input); current != value {
			input.Values.Set(param.valuesPath, value)
		}
	}

	return nil
}

func annotationsFromSnapshot(input *pkg.HookInput) (map[string]string, error) {
	snap := input.Snapshots.Get(snapshotModuleConfig)
	if len(snap) < 1 {
		return nil, fmt.Errorf("moduleConfig is missing, something wrong with Deckhouse configuration")
	}

	var annos map[string]string
	err := snap[0].UnmarshalTo(&annos)
	if err != nil {
		return nil, fmt.Errorf("failed to unmarshal moduleConfig annotations: %w", err)
	}

	return annos, nil
}
