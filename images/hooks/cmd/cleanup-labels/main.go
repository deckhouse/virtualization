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
	nodesMetadataSnapshot = "virthandler-nodes"
	virtHandlerLabel      = "kubevirt.internal.virtualization.deckhouse.io/schedulable"
	nodeJQFilter          = ".metadata"
)

var _ = registry.RegisterFunc(configDiscoveryService, handleVirtHandlerNodes)

var configDiscoveryService = &pkg.HookConfig{
	OnAfterDeleteHelm: &pkg.OrderedConfig{Order: 5},
	Kubernetes: []pkg.KubernetesConfig{
		{
			Name:       nodesMetadataSnapshot,
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

func handleVirtHandlerNodes(_ context.Context, input *pkg.HookInput) error {
	input.Logger.Info("Start.")

	nodeMetadatas := input.Snapshots.Get(nodesMetadataSnapshot)
	input.Logger.Info(fmt.Sprintf("Found %d nodes", len(nodeMetadatas)))
	if len(nodeMetadatas) == 0 {
		return nil
	}

	for _, nodeMetadata := range nodeMetadatas {
		metadata := &metav1.ObjectMeta{}
		if err := nodeMetadata.UnmarshalTo(metadata); err != nil {
			input.Logger.Error(fmt.Sprintf("Failed to unmarshal node metadata %v", err))
			continue
		}

		patches := []map[string]interface{}{}

		for key, _ := range metadata.Labels {
			if strings.Contains(key, "virtualization.deckhouse.io") {
				patches = append(patches, map[string]interface{}{
					"op":   "remove",
					"path": fmt.Sprintf("/metadata/labels/%s", jsonPatchEscape(key)),
				})
			}
		}

		if len(patches) == 0 {
			input.Logger.Info("No labels found, nothing to do.")
			continue
		} else {
			input.Logger.Info(fmt.Sprintf("Removing %d labels from node %s", len(patches), metadata.Name))
		}

		input.PatchCollector.PatchWithJSON(patches, "v1", "Node", "", metadata.Name)
	}
	input.Logger.Info("Done.")
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
