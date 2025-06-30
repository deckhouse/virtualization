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

package discovery_workload_nodes

import (
	"context"
	"fmt"

	"hooks/pkg/settings"

	"k8s.io/utils/ptr"

	"github.com/deckhouse/module-sdk/pkg"
	"github.com/deckhouse/module-sdk/pkg/registry"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	discoveryNodesSnapshot = "discovery-nodes"
	nodeLabel              = "kubevirt.internal.virtualization.deckhouse.io/schedulable"
	nodeLabelValue         = "true"

	virtHandlerNodeCountPath = "virtualization.internal.virtHandler.nodeCount"
)

var _ = registry.RegisterFunc(configDiscoveryService, handleDiscoveryNodes)

var configDiscoveryService = &pkg.HookConfig{
	OnBeforeHelm: &pkg.OrderedConfig{Order: 5},
	Kubernetes: []pkg.KubernetesConfig{
		{
			Name:       discoveryNodesSnapshot,
			APIVersion: "v1",
			Kind:       "Node",
			JqFilter:   ".metadata.name",
			LabelSelector: &metav1.LabelSelector{
				MatchLabels: map[string]string{
					nodeLabel: nodeLabelValue,
				},
			},
			ExecuteHookOnSynchronization: ptr.To(false),
		},
	},

	Queue: fmt.Sprintf("modules/%s", settings.ModuleName),
}

func handleDiscoveryNodes(_ context.Context, input *pkg.HookInput) error {
	nodeCount := len(input.Snapshots.Get(discoveryNodesSnapshot))
	input.Values.Set(virtHandlerNodeCountPath, nodeCount)
	return nil
}
