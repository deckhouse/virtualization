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

package network

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"

	corev1 "k8s.io/api/core/v1"

	"github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/test/e2e/internal/framework"
	kc "github.com/deckhouse/virtualization/test/e2e/internal/kubectl"
)

const (
	ciliumNamespace = "d8-cni-cilium"
	innaddrAny      = "0.0.0.0"
)

func CheckCiliumAgents(ctx context.Context, kubectl kc.Kubectl, vmName, vmNamespace string) error {
	// Get VM information using kubectl
	vmIP, nodeName, err := getVMInfo(kubectl, vmName, vmNamespace)
	if err != nil {
		return fmt.Errorf("failed to get VM info: %w", err)
	}

	// Get node internal IP using kubectl
	nodeInternalIP, err := getNodeInternalIP(kubectl, nodeName)
	if err != nil {
		return fmt.Errorf("failed to get node internal IP: %w", err)
	}

	// Get Cilium agent pods using kubectl
	pods, err := getCiliumAgentPods(kubectl)
	if err != nil {
		return fmt.Errorf("failed to get Cilium agent pods: %w", err)
	}

	// Check each Cilium agent pod
	var errs []error
	for _, pod := range pods {
		nodeIP := nodeInternalIP
		if pod.Spec.NodeName == nodeName {
			nodeIP = innaddrAny
		}

		ipCache, err := getCiliumIPCache(kubectl, pod)
		if err != nil {
			errs = append(errs, fmt.Errorf("failed to get Cilium Agent's IPCache `%s` on the node `%s`: %w", pod.Name, nodeName, err))
			continue
		}

		err = validateIPInCiliumIPCache(vmIP, nodeIP, ipCache)
		if err != nil {
			errs = append(errs, err)
			err = dumpIPCache(ipCache, nodeName, pod.Name)
			if err != nil {
				errs = append(errs, err)
			}
		}
	}

	if len(errs) > 0 {
		return fmt.Errorf("the Cilium agent check has failed: %w", errors.Join(errs...))
	}

	return nil
}

func getVMInfo(kubectl kc.Kubectl, vmName, vmNamespace string) (string, string, error) {
	result := kubectl.GetResource(v1alpha2.VirtualMachineResource, vmName, kc.GetOptions{Namespace: vmNamespace, Output: "json"})
	if result.Error() != nil {
		return "", "", fmt.Errorf("failed to get VM: %w", result.Error())
	}

	var vm v1alpha2.VirtualMachine
	if err := json.Unmarshal([]byte(result.StdOut()), &vm); err != nil {
		return "", "", fmt.Errorf("failed to parse VM JSON: %w", err)
	}

	if vm.Status.IPAddress == "" {
		return "", "", fmt.Errorf("VM %s has no IP address", vmName)
	}

	if vm.Status.Node == "" {
		return "", "", fmt.Errorf("VM %s has no node assigned", vmName)
	}

	return vm.Status.IPAddress, vm.Status.Node, nil
}

func getNodeInternalIP(kubectl kc.Kubectl, nodeName string) (string, error) {
	result := kubectl.GetResource(kc.ResourceNode, nodeName, kc.GetOptions{Output: "json"})
	if result.Error() != nil {
		return "", fmt.Errorf("failed to get node: %w", result.Error())
	}

	var node corev1.Node
	if err := json.Unmarshal([]byte(result.StdOut()), &node); err != nil {
		return "", fmt.Errorf("failed to parse node JSON: %w", err)
	}

	for _, addr := range node.Status.Addresses {
		if addr.Type == corev1.NodeInternalIP {
			return addr.Address, nil
		}
	}

	return "", fmt.Errorf("no InternalIP found for node %s", nodeName)
}

func getCiliumAgentPods(kubectl kc.Kubectl) ([]corev1.Pod, error) {
	result := kubectl.Get(string(kc.ResourcePod), kc.GetOptions{
		Namespace: ciliumNamespace,
		Labels:    map[string]string{"app": "agent"},
		Output:    "json",
	})
	if result.Error() != nil {
		return nil, fmt.Errorf("failed to get Cilium agent pods: %w", result.Error())
	}

	var podList corev1.PodList
	if err := json.Unmarshal([]byte(result.StdOut()), &podList); err != nil {
		return nil, fmt.Errorf("failed to parse pod list JSON: %w", err)
	}

	return podList.Items, nil
}

func getCiliumIPCache(kubectl kc.Kubectl, pod corev1.Pod) (string, error) {
	cmd := fmt.Sprintf("-n %s exec %s -c cilium-agent -- cilium map get cilium_ipcache", pod.Namespace, pod.Name)
	result := kubectl.RawCommand(cmd, kc.MediumTimeout)
	if result.Error() != nil {
		return "", fmt.Errorf("failed to execute command `%s`: %w", cmd, result.Error())
	}

	return result.StdOut(), nil
}

func validateIPInCiliumIPCache(vmIP, nodeIP, ipCache string) error {
	lines := strings.SplitSeq(ipCache, "\n")
	for line := range lines {
		if strings.Contains(line, vmIP) && strings.Contains(line, nodeIP) {
			return nil
		}
	}

	return fmt.Errorf("VM's IP `%s` not found in the Cilium agent's ipcache; NodeIP: `%s`", vmIP, nodeIP)
}

func dumpIPCache(ipCache, nodeName, podName string) error {
	ft := framework.GetFormattedTestCaseFullText()
	tmpDir := framework.GetTMPDir()

	resFileName := fmt.Sprintf("%s/e2e_failed__%s__%s__%s__cilium_ipcache.yaml", tmpDir, ft, nodeName, podName)
	err := os.WriteFile(resFileName, []byte(ipCache), 0o644)
	if err != nil {
		return fmt.Errorf("saving Cilium Agent's IPCache to file '%s' failed: %w", resFileName, err)
	}

	return nil
}
