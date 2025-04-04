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
	"crypto/rand"
	"fmt"

	"hooks/pkg/common"

	"github.com/deckhouse/module-sdk/pkg"
	"github.com/deckhouse/module-sdk/pkg/app"
	"github.com/deckhouse/module-sdk/pkg/registry"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"
)

const (
	virtualizationPodName = "virtualization-controller-pods"
	envName               = "VIRTUAL_MACHINES_MAC_ADDRESS_OUI"
)

var _ = registry.RegisterFunc(configModuleOUI, handlerModuleOUI)

var configModuleOUI = &pkg.HookConfig{
	OnBeforeHelm: &pkg.OrderedConfig{Order: 5},
	Kubernetes: []pkg.KubernetesConfig{
		{
			Name:       virtualizationPodName,
			APIVersion: "v1",
			Kind:       "Pod",
			LabelSelector: &metav1.LabelSelector{
				MatchLabels: map[string]string{
					"app": "virtualization-controller",
				},
			},
			NamespaceSelector: &pkg.NamespaceSelector{
				NameSelector: &pkg.NameSelector{
					MatchNames: []string{common.MODULE_NAMESPACE},
				},
			},
			ExecuteHookOnSynchronization: ptr.To(false),
		},
	},
	Queue: fmt.Sprintf("modules/%s", common.MODULE_NAME),
}

func handlerModuleOUI(_ context.Context, input *pkg.HookInput) error {
	input.Logger.Info("dlopatin Start to generate OUI for MAC address")
	podSnapshots := input.Snapshots.Get(virtualizationPodName)
	if len(podSnapshots) == 0 {
		input.Logger.Info("No virtualization-controller pods found")
		return nil
	}

	for _, pod := range podSnapshots {
		var podSpec struct {
			Spec struct {
				Containers []struct {
					Name string `json:"name"`
					Env  []struct {
						Name  string `json:"name"`
						Value string `json:"value"`
					} `json:"env"`
				} `json:"containers"`
			} `json:"spec"`
			Metadata struct {
				Name      string `json:"name"`
				Namespace string `json:"namespace"`
			} `json:"metadata"`
		}

		if err := pod.UnmarhalTo(&podSpec); err != nil {
			input.Logger.Error("Failed to unmarshal pod: %v", err)
			continue
		}

		for _, container := range podSpec.Spec.Containers {
			ouiFound := false
			for _, env := range container.Env {
				if env.Name == envName {
					ouiFound = true
					if env.Value == "" {
						newOUI, err := generateOUI()
						if err != nil {
							return fmt.Errorf("failed to generate OUI: %w", err)
						}
						input.PatchCollector.MergePatch(map[string]interface{}{
							"spec": map[string]interface{}{
								"containers": []map[string]interface{}{
									{
										"name": container.Name,
										"env": []map[string]interface{}{
											{
												"name":  envName,
												"value": newOUI,
											},
										},
									},
								},
							},
						}, "v1", "Pod", podSpec.Metadata.Namespace, podSpec.Metadata.Name)
					}
					break
				}
			}

			if !ouiFound {
				newOUI, err := generateOUI()
				if err != nil {
					return fmt.Errorf("failed to generate OUI: %w", err)
				}
				input.PatchCollector.MergePatch(map[string]interface{}{
					"spec": map[string]interface{}{
						"containers": []map[string]interface{}{
							{
								"name": container.Name,
								"env": []map[string]interface{}{
									{
										"name":  envName,
										"value": newOUI,
									},
								},
							},
						},
					},
				}, "v1", "Pod", podSpec.Metadata.Namespace, podSpec.Metadata.Name)
			}
		}
	}

	input.Logger.Info("dlopatin Stop to generate OUI for MAC address")
	return nil
}

func generateOUI() (string, error) {
	b := make([]byte, 3)
	_, err := rand.Read(b)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%02x%02x%02x", b[0], b[1], b[2]), nil
}

func main() {
	app.Run()
}
