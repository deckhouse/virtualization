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

// The purpose of this hook is to prevent already launched virt-handler pods from flapping, since the node group configuration virtualization-detect-kvm.sh will be responsible for installing the label virtualization.deckhouse.io/kvm-enabled.

package main

import (
	"context"
	"fmt"
	"strings"

	"github.com/deckhouse/module-sdk/pkg"
	"github.com/deckhouse/module-sdk/pkg/app"
	"github.com/deckhouse/module-sdk/pkg/registry"
	"k8s.io/utils/ptr"

	"hooks/pkg/common"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	nodesSnapshot      = "virthandler-nodes"
	virtHandlerLabel   = "kubevirt.internal.virtualization.deckhouse.io/schedulable"
	labelPattern       = "virtualization.deckhouse.io/"
	logMessageTemplate = "Removing %d label(s) contains %s from node %s"
	nodeJQFilter       = `{
		"name": .metadata.name,
		"labels": .metadata.labels,
	}`
)

type NodeInfo struct {
	Name   string            `json:"name"`
	Labels map[string]string `json:"labels"`
}

var _ = registry.RegisterFunc(configDiscoveryService, handleCleanUpNodeLabels)

var configDiscoveryService = &pkg.HookConfig{
	OnAfterDeleteHelm: &pkg.OrderedConfig{Order: 5},
	Kubernetes: []pkg.KubernetesConfig{
		{
			Name:       nodesSnapshot,
			APIVersion: "v1",
			Kind:       "Node",
			JqFilter:   nodeJQFilter,
			LabelSelector: &metav1.LabelSelector{
				MatchExpressions: []metav1.LabelSelectorRequirement{
					{
						Key:      virtHandlerLabel,
						Operator: metav1.LabelSelectorOpExists,
					},
				},
			},
			ExecuteHookOnSynchronization: ptr.To(false),
			ExecuteHookOnEvents:          ptr.To(false),
		},
	},

	Queue: fmt.Sprintf("modules/%s", common.MODULE_NAME),
}

func handleCleanUpNodeLabels(_ context.Context, input *pkg.HookInput) error {
	nodes := input.Snapshots.Get(nodesSnapshot)

	input.Logger.Info(fmt.Sprintf("Number of nodes with label \"%s\": %d", virtHandlerLabel, len(nodes)))
	if len(nodes) == 0 {
		return nil
	}

	for _, node := range nodes {
		nodeInfo := &NodeInfo{}
		if err := node.UnmarshalTo(nodeInfo); err != nil {
			input.Logger.Error(fmt.Sprintf("Failed to unmarshal node metadata %v", err))
			continue
		}

		patches := make([]map[string]string, 0)

		for key, _ := range nodeInfo.Labels {
			if strings.Contains(key, labelPattern) {
				patches = append(patches, map[string]string{
					"op":   "remove",
					"path": fmt.Sprintf("/metadata/labels/%s", jsonPatchEscape(key)),
				})
			}
		}

		if len(patches) == 0 {
			continue
		} else {
			input.Logger.Info(fmt.Sprintf(logMessageTemplate, len(patches), labelPattern, nodeInfo.Name))
		}

		input.PatchCollector.PatchWithJSON(patches, "v1", "Node", "", nodeInfo.Name)
	}
	return nil
}

func jsonPatchEscape(s string) string {
	s = strings.ReplaceAll(s, "~", "~0")
	s = strings.ReplaceAll(s, "/", "~1")
	return s
}

func main() {
	app.Run()
}
