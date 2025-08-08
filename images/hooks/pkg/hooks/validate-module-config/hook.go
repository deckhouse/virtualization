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

package validate_module_config

import (
	"context"
	"fmt"

	"hooks/pkg/settings"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/utils/ptr"

	"github.com/deckhouse/module-sdk/pkg"
	"github.com/deckhouse/module-sdk/pkg/registry"
	"github.com/deckhouse/virtualization-controller/pkg/controller/moduleconfig"
	mcapi "github.com/deckhouse/virtualization-controller/pkg/controller/moduleconfig/api"
)

const (
	snapshotModuleConfig = "module-config"
	moduleConfigJQFilter = `{"kind":.kind, "apiVersion":.apiVersion, "metadata":{"name": .metadata.name}, "spec":{"settings": .spec.settings}}`

	snapshotNodes = "nodes"
	nodesJQFilter = `{"kind":.kind, "apiVersion":.apiVersion, "metadata":{"name": .metadata.name}, "status":{"addresses": .status.addresses}}`
)

var _ = registry.RegisterFunc(config, Reconcile)

// TODO Disable "execute on events" after implementing additional first-enable-only component with webhook validators.
// TODO For now this hook is tracking ModuleConfig editing when module is deployed for the first time.
var config = &pkg.HookConfig{
	OnBeforeHelm: &pkg.OrderedConfig{Order: 10},
	Kubernetes: []pkg.KubernetesConfig{
		{
			Name:       snapshotModuleConfig,
			APIVersion: "deckhouse.io/v1alpha1",
			Kind:       "ModuleConfig",
			NameSelector: &pkg.NameSelector{
				MatchNames: []string{"virtualization"},
			},
			ExecuteHookOnSynchronization: ptr.To(true),
			ExecuteHookOnEvents:          ptr.To(true),
			JqFilter:                     moduleConfigJQFilter,
		},
		{
			Name:                         snapshotNodes,
			APIVersion:                   "v1",
			Kind:                         "Node",
			ExecuteHookOnSynchronization: ptr.To(true),
			ExecuteHookOnEvents:          ptr.To(true),
			JqFilter:                     nodesJQFilter,
		},
	},

	Queue: fmt.Sprintf("modules/%s", settings.ModuleName),
}

func Reconcile(_ context.Context, input *pkg.HookInput) error {
	mc, err := moduleConfigFromSnapshot(input)
	if err != nil {
		return err
	}

	nodes, err := nodesFromSnapshot(input)
	if err != nil {
		return err
	}

	// Run checks.
	err = validateModuleConfigSettings(mc, nodes)
	if err != nil {
		input.Values.Set(settings.InternalValuesReadinessPath, map[string]string{
			"moduleConfigValidationError": fmt.Sprintf("ModuleConfig/virtualization is invalid: %v", err),
		})
	} else {
		input.Values.Remove(settings.InternalValuesReadinessPath)
		copyModuleConfigSettingsIntoInternalValues(input)
	}

	return nil
}

func validateModuleConfigSettings(mc *mcapi.ModuleConfig, nodes []corev1.Node) error {
	CIDRs, err := moduleconfig.ParseCIDRs(mc.Spec.Settings)
	if err != nil {
		return err
	}

	err = moduleconfig.CheckOverlapsCIDRs(CIDRs)
	if err != nil {
		return err
	}

	err = moduleconfig.CheckNodeAddressesOverlap(nodes, CIDRs)
	if err != nil {
		return err
	}

	// TODO check Pods network.

	// TODO check Services network.

	return nil
}

// copyModuleConfigSettingsIntoInternalValues copies all options in ModuleConfig spec.settings
// into internal.moduleConfig object.
func copyModuleConfigSettingsIntoInternalValues(input *pkg.HookInput) {
	cfg := input.ConfigValues.Get("virtualization")
	input.Values.Set(settings.InternalValuesConfigCopyPath, cfg.Value())
}

func moduleConfigFromSnapshot(input *pkg.HookInput) (*mcapi.ModuleConfig, error) {
	// Unmarshal ModuleConfig from jqFilter result.
	snap := input.Snapshots.Get(snapshotModuleConfig)
	if len(snap) < 1 {
		return nil, fmt.Errorf("moduleConfig is missing, something wrong with Deckhouse configuration")
	}

	var mc mcapi.ModuleConfig
	err := snap[0].UnmarshalTo(&mc)
	return &mc, err
}

func nodesFromSnapshot(input *pkg.HookInput) ([]corev1.Node, error) {
	// Unmarshal Nodes from jqFilter results.
	snap := input.Snapshots.Get(snapshotNodes)
	if len(snap) == 0 {
		return nil, fmt.Errorf("nodes are missing, can't validate ModuleConfig")
	}

	nodes := make([]corev1.Node, 0, len(snap))
	for _, snapNode := range snap {
		node := corev1.Node{}
		err := snapNode.UnmarshalTo(&node)
		if err != nil {
			return nil, err
		}
		nodes = append(nodes, node)
	}

	return nodes, nil
}
