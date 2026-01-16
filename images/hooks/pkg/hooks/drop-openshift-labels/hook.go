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

package drop_openshift_labels

import (
	"context"
	"fmt"
	"hooks/pkg/settings"
	"strings"

	"k8s.io/utils/ptr"

	"github.com/deckhouse/module-sdk/pkg"
	"github.com/deckhouse/module-sdk/pkg/registry"
)

const (
	moduleNamespace = "virtualization-module-namespace"
)

const (
	openshiftClusterMonitoringLabel = "openshift.io/cluster-monitoring"
)

// TODO: Delete me after v1
var _ = registry.RegisterFunc(configModuleNamespace, handlerModuleNamespace)

var configModuleNamespace = &pkg.HookConfig{
	OnAfterHelm: &pkg.OrderedConfig{Order: 5},
	Kubernetes: []pkg.KubernetesConfig{
		{
			Name:       moduleNamespace,
			APIVersion: "v1",
			Kind:       "Namespace",
			JqFilter:   ".metadata",

			NameSelector: &pkg.NameSelector{
				MatchNames: []string{settings.ModuleNamespace},
			},
			ExecuteHookOnEvents: ptr.To(false),
		},
	},

	Queue: fmt.Sprintf("modules/%s", settings.ModuleName),
}

type NamespaceMetadata struct {
	Name   string            `json:"name"`
	Labels map[string]string `json:"labels"`
}

func handlerModuleNamespace(_ context.Context, input *pkg.HookInput) error {
	snaps := input.Snapshots.Get(moduleNamespace)
	if len(snaps) == 0 {
		input.Logger.Debug("no namespace found")
		return nil
	}
	ns := &NamespaceMetadata{}
	err := snaps[0].UnmarshalTo(ns)
	if err != nil {
		input.Logger.Error("failed to unmarshal namespace", "error", err)
		return err
	}

	if _, exist := ns.Labels[openshiftClusterMonitoringLabel]; !exist {
		input.Logger.Debug("no labels to update")
		return nil
	}

	patch := []map[string]interface{}{
		{"op": "remove", "path": fmt.Sprintf("/metadata/labels/%s", jsonPatchEscape(openshiftClusterMonitoringLabel)), "value": nil},
	}
	input.PatchCollector.PatchWithJSON(
		patch,
		"v1",
		"Namespace",
		"",
		settings.ModuleNamespace,
	)

	input.Logger.Debug(fmt.Sprintf("Added patch to PatchCollector for replace labels on %q namespace", settings.ModuleNamespace))
	input.Logger.Debug("Hook finished")

	return nil
}

func jsonPatchEscape(s string) string {
	return strings.NewReplacer("~", "~0", "/", "~1").Replace(s)
}
