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

	bandwidthPerMigrationAnnotation                 = "virtualization.deckhouse.io/bandwidth-per-migration"
	completionTimeoutPerGiBAnnotation               = "virtualization.deckhouse.io/completion-timeout-per-gib"
	maxActiveMigrationsPerClusterAnnotation         = "virtualization.deckhouse.io/max-active-migrations-per-cluster"
	inboundMigrationSchedulerAnnotation             = "virtualization.deckhouse.io/inbound-migration-scheduler"
	maxActiveOutboundMigrationsPerNodeAnnotation    = "virtualization.deckhouse.io/max-active-outbound-migrations-per-node"
	maxActiveInboundMigrationsPerNodeAnnotation     = "virtualization.deckhouse.io/max-active-inbound-migrations-per-node"
	maxPreactiveOutboundMigrationsPerNodeAnnotation = "virtualization.deckhouse.io/max-preactive-outbound-migrations-per-node"
	maxActiveMigrationsPerNodeAnnotation            = "virtualization.deckhouse.io/max-active-migrations-per-node"
	progressTimeoutAnnotation                       = "virtualization.deckhouse.io/progress-timeout"
	disableTLSAnnotation                            = "virtualization.deckhouse.io/disable-tls"
	disableFirmwareUpdateAnnotation                 = "virtualization.deckhouse.io/disable-firmware-update"

	bandwidthPerMigrationValuesPath              = "virtualization.internal.virtConfig.bandwidthPerMigration"
	completionTimeoutPerGiBValuesPath            = "virtualization.internal.virtConfig.completionTimeoutPerGiB"
	parallelOutboundMigrationsPerNodeValuesPath  = "virtualization.internal.virtConfig.parallelOutboundMigrationsPerNode"
	maxActiveOutboundMigrationsPerNodeValuesPath = "virtualization.internal.virtConfig.maxActiveOutboundMigrationsPerNode"
	maxActiveInboundMigrationsPerNodeValuesPath  = "virtualization.internal.virtConfig.maxActiveInboundMigrationsPerNode"
	maxActiveMigrationsPerClusterValuesPath      = "virtualization.internal.virtConfig.maxActiveMigrationsPerCluster"
	inboundMigrationSchedulerValuesPath          = "virtualization.internal.virtConfig.inboundMigrationScheduler"
	maxActiveMigrationsPerNodeValuesPath         = "virtualization.internal.virtConfig.maxActiveMigrationsPerNode"
	progressTimeoutValuesPath                    = "virtualization.internal.virtConfig.progressTimeout"
	disableTLSValuesPath                         = "virtualization.internal.virtConfig.disableTLS"
	disableFirmwareUpdateValuesPath              = "virtualization.internal.disableFirmwareUpdate"

	liveMigrationNetworkTypeConfigPath = "virtualization.liveMigration.network.type"
	unlimitedBandwidthPerMigration     = "0"
	disabledLimit                      = "disabled"

	defaultBandwidthPerMigration                 = "640Mi"
	defaultCompletionTimeoutPerGiB               = 800
	defaultMaxActiveOutboundMigrationsPerNode    = 1
	defaultMaxActiveInboundMigrationsPerNode     = 1
	defaultMaxPreactiveOutboundMigrationsPerNode = 1
	defaultMaxActiveMigrationsPerCluster         = ""
	defaultInboundMigrationScheduler             = ""
	defaultProgressTimeout                       = 150
	defaultDisableTLS                            = false
	defaultDisableFirmwareUpdate                 = false
)

// migrationParams defines migration parameters configurable via ModuleConfig annotations.
// The default cluster limit of active migrations is the node count the discovery-workload-nodes
// hook reads, so the annotation stays empty unless it is set.
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
		annotation:   maxActiveMigrationsPerClusterAnnotation,
		valuesPath:   maxActiveMigrationsPerClusterValuesPath,
		defaultValue: defaultMaxActiveMigrationsPerCluster,
	},
	{
		annotation:   inboundMigrationSchedulerAnnotation,
		valuesPath:   inboundMigrationSchedulerValuesPath,
		defaultValue: defaultInboundMigrationScheduler,
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
	{
		annotation:   disableFirmwareUpdateAnnotation,
		valuesPath:   disableFirmwareUpdateValuesPath,
		defaultValue: defaultDisableFirmwareUpdate,
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

	hasDedicatedMigrationNetwork := input.ConfigValues.Get(liveMigrationNetworkTypeConfigPath).String() != ""

	for _, param := range migrationParams {
		// Traffic on a dedicated migration network does not compete with workloads.
		if hasDedicatedMigrationNetwork && param.annotation == bandwidthPerMigrationAnnotation {
			param.defaultValue = unlimitedBandwidthPerMigration
		}

		value, err := param.resolve(annos)
		if err != nil {
			return err
		}
		if current := param.getCurrent(input); current != value {
			input.Values.Set(param.valuesPath, value)
		}
	}

	slots, err := resolveMigrationSlots(annos)
	if err != nil {
		return err
	}
	if _, err := limitAnnotation(annos, 0, maxActiveMigrationsPerClusterAnnotation); err != nil {
		return err
	}
	if slots.shared > 0 && slots.separateSet {
		input.Logger.Warn("The " + maxActiveMigrationsPerNodeAnnotation + " annotation is set; the separate outbound and inbound migration limits are ignored.")
	}
	for _, name := range removedMigrationAnnotations {
		if _, ok := annos[name]; ok {
			input.Logger.Warn("The " + name + " annotation is no longer read; set the max-active-* migration annotations instead.")
		}
	}
	for path, value := range map[string]int{
		parallelOutboundMigrationsPerNodeValuesPath:  slots.outboundWindow(),
		maxActiveOutboundMigrationsPerNodeValuesPath: slots.activeOutbound,
		maxActiveInboundMigrationsPerNodeValuesPath:  slots.activeInbound,
		maxActiveMigrationsPerNodeValuesPath:         slots.shared,
	} {
		// 0 disables a limit, so a missing value is set too: the templates default it to a limit.
		if current := input.Values.Get(path); !current.Exists() || int(current.Int()) != value {
			input.Values.Set(path, value)
		}
	}

	return nil
}

// removedMigrationAnnotations are the previous names of the migration limits. They are not read
// anymore; a warning tells an admin who still has them set what to change.
var removedMigrationAnnotations = []string{
	"virtualization.deckhouse.io/parallel-outbound-migrations-per-node",
	"virtualization.deckhouse.io/parallel-sync-migrations-per-node",
	"virtualization.deckhouse.io/parallel-inbound-migrations-per-node",
	"virtualization.deckhouse.io/parallel-per-cluster-migration-limit",
	"virtualization.deckhouse.io/inbound-migration-limit",
	"virtualization.deckhouse.io/outbound-migration-limit",
	"virtualization.deckhouse.io/parallel-per-node-migration-limit",
}

// migrationSlots is the per-node migration capacity. A node runs up to activeOutbound
// outgoing and activeInbound incoming migrations that transfer memory, and prepares up to
// preactiveOutbound more outgoing ones, so the next migration starts transferring as soon as
// a slot frees up instead of waiting for its target to be prepared. A limit of 0 is disabled.
type migrationSlots struct {
	activeOutbound    int
	activeInbound     int
	preactiveOutbound int
	// shared is the budget of migrations a node transfers in any direction. When set, it
	// replaces activeOutbound and activeInbound.
	shared int
	// separateSet reports that a separate outbound or inbound limit is set explicitly.
	separateSet bool
}

// outboundWindow is the number of migrations a node may have in flight, preparing or
// transferring, or 0 when the outgoing migrations are not limited. KubeVirt enforces it
// before it creates a target pod.
func (s migrationSlots) outboundWindow() int {
	switch {
	case s.shared > 0:
		return s.shared + s.preactiveOutbound
	case s.activeOutbound > 0:
		return s.activeOutbound + s.preactiveOutbound
	default:
		return 0
	}
}

// resolveMigrationSlots reads the slot annotations.
func resolveMigrationSlots(annos map[string]string) (migrationSlots, error) {
	activeOutbound, err := limitAnnotation(annos, defaultMaxActiveOutboundMigrationsPerNode, maxActiveOutboundMigrationsPerNodeAnnotation)
	if err != nil {
		return migrationSlots{}, err
	}
	activeInbound, err := limitAnnotation(annos, defaultMaxActiveInboundMigrationsPerNode, maxActiveInboundMigrationsPerNodeAnnotation)
	if err != nil {
		return migrationSlots{}, err
	}
	preactiveOutbound, err := intAnnotation(annos, defaultMaxPreactiveOutboundMigrationsPerNode, 0, maxPreactiveOutboundMigrationsPerNodeAnnotation)
	if err != nil {
		return migrationSlots{}, err
	}

	shared, err := limitAnnotation(annos, 0, maxActiveMigrationsPerNodeAnnotation)
	if err != nil {
		return migrationSlots{}, err
	}

	slots := migrationSlots{activeOutbound: activeOutbound, activeInbound: activeInbound, preactiveOutbound: preactiveOutbound, shared: shared}
	for _, name := range []string{maxActiveOutboundMigrationsPerNodeAnnotation, maxActiveInboundMigrationsPerNodeAnnotation} {
		if _, ok := annos[name]; ok {
			slots.separateSet = true
		}
	}

	return slots, nil
}

// limitAnnotation reads a limit annotation: a number of at least 1, or "disabled", returned as 0.
func limitAnnotation(annos map[string]string, defaultValue int, name string) (int, error) {
	if annos[name] == disabledLimit {
		return 0, nil
	}
	value, err := intAnnotation(annos, defaultValue, 1, name)
	if err != nil {
		return 0, fmt.Errorf("%w; set a number or %q", err, disabledLimit)
	}
	return value, nil
}

// intAnnotation returns the value of the annotation, or defaultValue when it is not set.
func intAnnotation(annos map[string]string, defaultValue, minValue int, name string) (int, error) {
	raw, ok := annos[name]
	if !ok {
		return defaultValue, nil
	}
	value, err := strconv.Atoi(raw)
	if err != nil {
		return 0, fmt.Errorf("failed to parse %q annotation: %w", name, err)
	}
	if value < minValue {
		return 0, fmt.Errorf("the %q annotation must be at least %d, got %d", name, minValue, value)
	}
	return value, nil
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
