/*
Copyright 2026 Flant JSC

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

package discover_kube_apiserver_feature_gates

import (
	"context"
	"fmt"
	"hooks/pkg/settings"
	"slices"
	"strings"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"

	"github.com/deckhouse/module-sdk/pkg"
	"github.com/deckhouse/module-sdk/pkg/registry"
)

const (
	snapshotKubeAPIServerPod = "kube-apiserver-pod"

	featureGatesPath    = "virtualization.internal.kubeAPIServerFeatureGates"
	draFeatureGatesPath = "virtualization.internal.hasDraFeatureGates"
)

var _ = registry.RegisterFunc(config, Reconcile)

var config = &pkg.HookConfig{
	OnBeforeHelm: &pkg.OrderedConfig{Order: 5},
	Kubernetes: []pkg.KubernetesConfig{
		{
			Name:       snapshotKubeAPIServerPod,
			APIVersion: "v1",
			Kind:       "Pod",
			NamespaceSelector: &pkg.NamespaceSelector{
				NameSelector: &pkg.NameSelector{
					MatchNames: []string{"kube-system"},
				},
			},
			LabelSelector: &metav1.LabelSelector{
				MatchLabels: map[string]string{"component": "kube-apiserver"},
			},
			JqFilter: `{
				"name": .metadata.name,
				"command": (.spec.containers[0].command // []),
				"args": (.spec.containers[0].args // [])
			}`,
			ExecuteHookOnSynchronization: ptr.To(false),
		},
	},
	Queue: fmt.Sprintf("modules/%s", settings.ModuleName),
}

func Reconcile(_ context.Context, input *pkg.HookInput) error {
	featureGates, err := discoverFeatureGates(input)
	if err != nil {
		return fmt.Errorf("failed to discover feature gates: %w", err)
	}

	input.Values.Set(featureGatesPath, featureGates)

	if slices.Contains(featureGates, "DRADeviceBindingConditions") &&
		slices.Contains(featureGates, "DRAResourceClaimDeviceStatus") &&
		slices.Contains(featureGates, "DRAConsumableCapacity") {
		input.Values.Set(draFeatureGatesPath, "true")
	}

	return nil
}

// discoverFeatureGates extracts enabled feature gates from kube-apiserver pod command/args.
// Returns a list of enabled feature gate names (those set to "true").
func discoverFeatureGates(input *pkg.HookInput) ([]string, error) {
	pods := input.Snapshots.Get(snapshotKubeAPIServerPod)
	if len(pods) == 0 {
		return nil, fmt.Errorf("no kube-apiserver pods found")
	}

	// Use the first pod - all kube-apiserver pods should have the same feature gates
	pod := pods[0]

	var podInfo struct {
		Command []string `json:"command"`
		Args    []string `json:"args"`
	}

	err := pod.UnmarshalTo(&podInfo)
	if err != nil {
		return nil, fmt.Errorf("failed to unmarshal kube-apiserver pod: %w", err)
	}

	allArgs := make([]string, 0, len(podInfo.Command)+len(podInfo.Args))
	allArgs = append(allArgs, podInfo.Command...)
	allArgs = append(allArgs, podInfo.Args...)

	var enabledGates []string

	for _, arg := range allArgs {
		if !strings.HasPrefix(arg, "--feature-gates=") {
			continue
		}

		// Parse feature-gates value: "Gate1=true,Gate2=false,Gate3=true"
		gatesStr := strings.TrimPrefix(arg, "--feature-gates=")
		gates := strings.SplitSeq(gatesStr, ",")

		for gate := range gates {
			gate = strings.TrimSpace(gate)
			if gate == "" {
				continue
			}

			parts := strings.SplitN(gate, "=", 2)
			if len(parts) != 2 {
				continue
			}

			gateName := parts[0]
			gateValue := strings.ToLower(parts[1])

			if gateValue == "true" {
				enabledGates = append(enabledGates, gateName)
			}
		}
	}

	return enabledGates, nil
}
