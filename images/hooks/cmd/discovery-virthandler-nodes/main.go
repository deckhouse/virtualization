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

package main

import (
	"context"
	"fmt"

	"github.com/deckhouse/module-sdk/pkg"
	"github.com/deckhouse/module-sdk/pkg/app"
	"github.com/deckhouse/module-sdk/pkg/registry"
	"k8s.io/utils/ptr"

	"hooks/pkg/common"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	nodesSnapshot         = "discovery-nodes"
	virtHandlerLabel      = "kubevirt.internal.virtualization.deckhouse.io/schedulable"
	virtHandlerLabelValue = "true"
	nodeJQFilter          = ".metadata.name"

	virtHandlerNodeCountPath = "virtualization.internal.virtHandler.nodeCount"
)

var _ = registry.RegisterFunc(configDiscoveryService, handleDiscoveryVirtHandlerNodes)

var configDiscoveryService = &pkg.HookConfig{
	OnBeforeHelm: &pkg.OrderedConfig{Order: 5},
	Kubernetes: []pkg.KubernetesConfig{
		{
			Name:       nodesSnapshot,
			APIVersion: "v1",
			Kind:       "Node",
			JqFilter:   nodeJQFilter,
			LabelSelector: &metav1.LabelSelector{
				MatchLabels: map[string]string{
					virtHandlerLabel: virtHandlerLabelValue,
				},
			},
			ExecuteHookOnSynchronization: ptr.To(false),
		},
	},

	Queue: fmt.Sprintf("modules/%s", common.MODULE_NAME),
}

func handleDiscoveryVirtHandlerNodes(_ context.Context, input *pkg.HookInput) error {
	nodeCount := len(input.Snapshots.Get(nodesSnapshot))
	input.Values.Set(virtHandlerNodeCountPath, nodeCount)
	return nil
}

func main() {
	app.Run()
}
