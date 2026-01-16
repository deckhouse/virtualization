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

package discovery_clusterip_service_for_dvcr

import (
	"context"
	"fmt"
	"strings"

	"hooks/pkg/settings"

	"k8s.io/utils/ptr"

	"github.com/deckhouse/module-sdk/pkg"
	"github.com/deckhouse/module-sdk/pkg/registry"
)

const (
	discoveryService = "discovery-service"
	serviceName      = "dvcr"

	serviceIPValuePath = "virtualization.internal.dvcr.serviceIP"
)

var _ = registry.RegisterFunc(configDiscoveryService, handleDiscoveryService)

var configDiscoveryService = &pkg.HookConfig{
	// Note: this hook should run before TLS certificate generator for DVCR. Order should be lower than 5.
	OnBeforeHelm: &pkg.OrderedConfig{Order: 3},
	Kubernetes: []pkg.KubernetesConfig{
		{
			Name:       discoveryService,
			APIVersion: "v1",
			Kind:       "Service",
			JqFilter:   ".spec.clusterIP",

			NameSelector: &pkg.NameSelector{
				MatchNames: []string{serviceName},
			},

			NamespaceSelector: &pkg.NamespaceSelector{
				NameSelector: &pkg.NameSelector{
					MatchNames: []string{settings.ModuleNamespace},
				},
			},

			ExecuteHookOnSynchronization: ptr.To(false),
		},
	},

	Queue: fmt.Sprintf("modules/%s", settings.ModuleName),
}

func handleDiscoveryService(_ context.Context, input *pkg.HookInput) error {
	clusterIP := getClusterIP(input)

	if clusterIP == "" {
		input.Logger.Info(fmt.Sprintf("ClusterIP of service dvcr not found. Delete value from %s", serviceIPValuePath))
		input.Values.Remove(serviceIPValuePath)
		return nil
	}

	oldClusterIP := input.Values.Get(serviceIPValuePath).String()
	if clusterIP != oldClusterIP {
		input.Logger.Info(fmt.Sprintf("Set ip %s to %s", clusterIP, serviceIPValuePath))
		input.Values.Set(serviceIPValuePath, clusterIP)
	}
	return nil
}

func getClusterIP(input *pkg.HookInput) string {
	snapshots := input.Snapshots.Get(discoveryService)
	if len(snapshots) > 0 {
		return strings.Trim(snapshots[0].String(), `"`)
	}
	return ""
}
