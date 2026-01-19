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
	"net/netip"

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

	podSubnetCIDRPath     = "global.clusterConfiguration.podSubnetCIDR"
	serviceSubnetCIDRPath = "global.clusterConfiguration.serviceSubnetCIDR"
)

var _ = registry.RegisterFunc(config, Reconcile)

// TODO Disable "execute on events" after implementing additional first-time-enable component with webhook validators.
// (For now this hook is responsible for tracking ModuleConfig editing when module is deployed for the first time).
// TODO Save valid settings into Secret to load them on next Deckhouse reload. (Standard practice for deckhouse modules).
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

	podSubnet, err := podSubnetFromValues(input)
	if err != nil {
		return err
	}

	serviceSubnet, err := serviceSubnetFromValues(input)
	if err != nil {
		return err
	}

	// Run checks.
	err = validateModuleConfigSettings(mc, nodes, podSubnet, serviceSubnet)
	if err != nil {
		input.Values.Set(settings.InternalValuesConfigValidationPath, map[string]string{
			"error": fmt.Sprintf("ModuleConfig/virtualization is invalid: %v", err),
		})
	} else {
		// Module is valid, remove moduleConfigValidation object to indicate valid state for the readiness probe.
		input.Values.Remove(settings.InternalValuesConfigValidationPath)
		// Copy valid settings from config values to apply them in helm templates.
		copyModuleConfigSettingsIntoInternalValues(input)
	}

	return nil
}

func validateModuleConfigSettings(mc *mcapi.ModuleConfig, nodes []corev1.Node, podSubnet, serviceSubnet netip.Prefix) error {
	CIDRs, err := moduleconfig.ParseCIDRs(mc.Spec.Settings)
	if err != nil {
		return err
	}

	err = moduleconfig.CheckCIDRsOverlap(CIDRs)
	if err != nil {
		return err
	}

	err = moduleconfig.CheckCIDRsOverlapWithNodeAddresses(CIDRs, nodes)
	if err != nil {
		return err
	}

	err = moduleconfig.CheckCIDRsOverlapWithPodSubnet(CIDRs, podSubnet)
	if err != nil {
		return err
	}

	err = moduleconfig.CheckCIDRsOverlapWithServiceSubnet(CIDRs, serviceSubnet)
	if err != nil {
		return err
	}

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

func podSubnetFromValues(input *pkg.HookInput) (netip.Prefix, error) {
	podSubnetStr := input.Values.Get(podSubnetCIDRPath).String()
	if podSubnetStr == "" {
		return netip.Prefix{}, fmt.Errorf("get podSubnetCIDR: %s value should not be empty, check cluster configuration", podSubnetCIDRPath)
	}
	cidr, err := netip.ParsePrefix(podSubnetStr)
	if err != nil {
		return netip.Prefix{}, fmt.Errorf("parse global podSubnetCIDR: %w", err)
	}
	return cidr, nil
}

func serviceSubnetFromValues(input *pkg.HookInput) (netip.Prefix, error) {
	serviceSubnetStr := input.Values.Get(serviceSubnetCIDRPath).String()
	if serviceSubnetStr == "" {
		return netip.Prefix{}, fmt.Errorf("get serviceSubnetCIDR: %s value should not be empty, check cluster configuration", podSubnetCIDRPath)
	}
	cidr, err := netip.ParsePrefix(serviceSubnetStr)
	if err != nil {
		return netip.Prefix{}, fmt.Errorf("parse global serviceSubnetCIDR: %w", err)
	}
	return cidr, nil
}
