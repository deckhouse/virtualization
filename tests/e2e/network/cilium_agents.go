package network

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	virtualizationv1alpha2 "github.com/deckhouse/virtualization/api/core/v1alpha2"
	kc "github.com/deckhouse/virtualization/tests/e2e/kubectl"
	corev1 "k8s.io/api/core/v1"
)

const (
	ciliumNamespace = "d8-cni-cilium"
	innaddrAny      = "0.0.0.0"
)

func CheckCilliumAgents(ctx context.Context, kubectl kc.Kubectl, vmName, vmNamespace string) error {
	// Get VM information using kubectl
	vmIP, nodeName, err := getVMInfo(kubectl, vmName, vmNamespace)
	if err != nil {
		return fmt.Errorf("failed to get VM info: %v", err)
	}

	// Get node internal IP using kubectl
	nodeInternalIP, err := getNodeInternalIP(kubectl, nodeName)
	if err != nil {
		return fmt.Errorf("failed to get node internal IP: %v", err)
	}

	// Get Cilium agent pods using kubectl
	pods, err := getCiliumAgentPods(kubectl)
	if err != nil {
		return fmt.Errorf("failed to get Cilium agent pods: %v", err)
	}

	// Check each Cilium agent pod
	for _, pod := range pods {
		if pod.Spec.NodeName == nodeName {
			// For pods on the same node as the VM
			found, err := searchIPFromCiliumIPCache(kubectl, pod, vmIP, innaddrAny)
			if err != nil {
				return err
			}

			if !found {
				return fmt.Errorf("failed: cilium agent %s for VM's node %s", pod.Name, nodeName)
			}
		} else {
			// For pods on different nodes
			found, err := searchIPFromCiliumIPCache(kubectl, pod, vmIP, nodeInternalIP)
			if err != nil {
				return err
			}

			if !found {
				return fmt.Errorf("failed: cilium agent %s for node %s", pod.Name, pod.Spec.NodeName)
			}
		}
	}

	return nil
}

func getVMInfo(kubectl kc.Kubectl, vmName, vmNamespace string) (string, string, error) {
	result := kubectl.GetResource(kc.ResourceVM, vmName, kc.GetOptions{Namespace: vmNamespace, Output: "json"})
	if result.Error() != nil {
		return "", "", fmt.Errorf("failed to get VM: %v", result.Error())
	}

	var vm virtualizationv1alpha2.VirtualMachine
	if err := json.Unmarshal([]byte(result.StdOut()), &vm); err != nil {
		return "", "", fmt.Errorf("failed to parse VM JSON: %v", err)
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
		return "", fmt.Errorf("failed to get node: %v", result.Error())
	}

	var node corev1.Node
	if err := json.Unmarshal([]byte(result.StdOut()), &node); err != nil {
		return "", fmt.Errorf("failed to parse node JSON: %v", err)
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
		return nil, fmt.Errorf("failed to get Cilium agent pods: %v", result.Error())
	}

	var podList corev1.PodList
	if err := json.Unmarshal([]byte(result.StdOut()), &podList); err != nil {
		return nil, fmt.Errorf("failed to parse pod list JSON: %v", err)
	}

	return podList.Items, nil
}

func searchIPFromCiliumIPCache(kubectl kc.Kubectl, pod corev1.Pod, vmIP, nodeIP string) (bool, error) {
	cmd := fmt.Sprintf("-n %s exec %s -c cilium-agent -- cilium map get cilium_ipcache", pod.Namespace, pod.Name)
	result := kubectl.RawCommand(cmd, kc.MediumTimeout)
	if result.Error() != nil {
		return false, fmt.Errorf("failed to execute command: %v", result.Error())
	}

	output := result.StdOut()
	lines := strings.Split(output, "\n")
	found := false
	for _, line := range lines {
		if strings.Contains(line, vmIP) && strings.Contains(line, nodeIP) {
			found = true
			break
		}
	}

	return found, nil
}
