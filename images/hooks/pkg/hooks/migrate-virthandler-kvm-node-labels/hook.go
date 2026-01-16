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

package migrate_virthandler_kvm_node_labels

import (
	"context"
	"fmt"
	"hooks/pkg/settings"
	"strings"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"

	"github.com/deckhouse/module-sdk/pkg"
	"github.com/deckhouse/module-sdk/pkg/registry"
)

const (
	nodesSnapshot         = "virthandler-nodes"
	virtHandlerLabel      = "kubevirt.internal.virtualization.deckhouse.io/schedulable"
	virtHandlerLabelValue = "true"
	kvmEnabledLabel       = "virtualization.deckhouse.io/kvm-enabled"
	kvmEnabledLabelValue  = "true"
	nodeJQFilter          = ".metadata"
	logMessageTemplate    = "Active hypervisor node detected, setting %s label on node %s"
)

type NodeInfo struct {
	Name   string            `json:"name"`
	Labels map[string]string `json:"labels"`
}

var kvmLabelPatch = []map[string]string{
	{"op": "add", "path": fmt.Sprintf("/metadata/labels/%s", jsonPatchEscape(kvmEnabledLabel)), "value": kvmEnabledLabelValue},
}

var _ = registry.RegisterFunc(config, handler)

var config = &pkg.HookConfig{
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
			ExecuteHookOnEvents:          ptr.To(false),
		},
	},

	Queue: fmt.Sprintf("modules/%s", settings.ModuleName),
}

func handler(_ context.Context, input *pkg.HookInput) error {
	nodes := input.Snapshots.Get(nodesSnapshot)
	if len(nodes) == 0 {
		return nil
	}

	for _, node := range nodes {
		nodeInfo := &NodeInfo{}
		if err := node.UnmarshalTo(nodeInfo); err != nil {
			input.Logger.Error(fmt.Sprintf("Failed to unmarshal node metadata %v", err))
			continue
		}

		if _, ok := nodeInfo.Labels[kvmEnabledLabel]; !ok {
			input.PatchCollector.PatchWithJSON(kvmLabelPatch, "v1", "Node", "", nodeInfo.Name)
			input.Logger.Info(fmt.Sprintf(logMessageTemplate, kvmEnabledLabel, nodeInfo.Name))
		}
	}
	return nil
}

func jsonPatchEscape(s string) string {
	s = strings.ReplaceAll(s, "~", "~0")
	s = strings.ReplaceAll(s, "/", "~1")
	return s
}
