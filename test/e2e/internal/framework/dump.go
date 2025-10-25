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

package framework

import (
	"context"
	"fmt"
	"io"
	"os"
	"strings"

	. "github.com/onsi/ginkgo/v2"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/deckhouse/virtualization/test/e2e/internal/kubectl"
)

// SaveTestCaseDump dump some resources, logs and descriptions that may help in further diagnostic.
//
// NOTE: This method is called in AfterEach for failed specs only. Avoid to use Expect,
// as it fails without reporting. Better use GinkgoWriter to report errors at this point.
func (f *Framework) SaveTestCaseDump(ctx context.Context, labels map[string]string, additional, namespace string) {
	replacer := strings.NewReplacer(
		" ", "_",
		":", "_",
		"[", "_",
		"]", "_",
		"(", "_",
		")", "_",
		"|", "_",
		"`", "",
		"'", "",
	)
	additional = replacer.Replace(strings.ToLower(additional))

	tmpDir := os.Getenv("RUNNER_TEMP")
	if tmpDir == "" {
		tmpDir = "/tmp"
	}

	f.saveTestCaseResources(labels, additional, namespace, tmpDir)
	f.savePodAdditionalInfo(ctx, labels, additional, namespace, tmpDir)
	f.saveIntvirtvmDescriptions(labels, additional, namespace, tmpDir)
}

func (f *Framework) saveTestCaseResources(labels map[string]string, additional, namespace, dumpPath string) {
	resFileName := fmt.Sprintf("%s/e2e_failed__%s__%s.yaml", dumpPath, labels["testcase"], additional)

	clusterResourceResult := f.Clients.Kubectl().Get("cvi,vmc", kubectl.GetOptions{
		Labels:            labels,
		Namespace:         namespace,
		Output:            "yaml",
		ShowManagedFields: true,
	})
	if clusterResourceResult.Error() != nil {
		GinkgoWriter.Printf("Get resources error:\n%s\n%w\n%s\n", clusterResourceResult.GetCmd(), clusterResourceResult.Error(), clusterResourceResult.StdErr())
	}

	namespacedResourceResult := f.Clients.Kubectl().Get("virtualization,intvirt,pod,volumesnapshot,pvc", kubectl.GetOptions{
		Namespace:         namespace,
		Output:            "yaml",
		ShowManagedFields: true,
	})
	if namespacedResourceResult.Error() != nil {
		GinkgoWriter.Printf("Get resources error:\n%s\n%w\n%s\n", namespacedResourceResult.GetCmd(), namespacedResourceResult.Error(), namespacedResourceResult.StdErr())
	}

	// Stdout may present even if error is occurred.
	if len(clusterResourceResult.StdOutBytes()) > 0 || len(namespacedResourceResult.StdOutBytes()) > 0 {
		delimiter := []byte("---\n")

		result := make([]byte, 0, len(clusterResourceResult.StdOutBytes())+len(delimiter)+len(namespacedResourceResult.StdOutBytes()))
		result = append(result, clusterResourceResult.StdOutBytes()...)
		result = append(result, delimiter...)
		result = append(result, namespacedResourceResult.StdOutBytes()...)

		err := os.WriteFile(resFileName, result, 0o644)
		if err != nil {
			GinkgoWriter.Printf("Save resources to file '%s' failed: %s\n", resFileName, err)
		}
	}
}

func (f *Framework) savePodAdditionalInfo(ctx context.Context, labels map[string]string, additional, namespace, dumpPath string) {
	pods, err := f.Clients.kubeClient.CoreV1().Pods(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		GinkgoWriter.Printf("Failed to get PodList:\n%s\n", err)
	}

	if len(pods.Items) == 0 {
		GinkgoWriter.Println("The list of pods is empty; nothing to dump.")
	}

	for _, pod := range pods.Items {
		f.writePodLogs(ctx, pod.Name, pod.Namespace, dumpPath, additional, labels)
		f.writePodDescription(pod.Name, pod.Namespace, dumpPath, additional, labels)
		f.writeVirtualMachineGuestInfo(pod, dumpPath, additional, labels)
	}
}

func (f *Framework) saveIntvirtvmDescriptions(labels map[string]string, additional, namespace, dumpPath string) {
	describeCmd := f.Clients.Kubectl().RawCommand(fmt.Sprintf("describe intvirtvm --namespace %s", namespace), ShortTimeout)
	if describeCmd.Error() != nil {
		GinkgoWriter.Printf("Failed to describe InternalVirtualizationVirtualMachine:\nError: %s\n", describeCmd.StdErr())
	}

	fileName := fmt.Sprintf("%s/e2e_failed__%s__%s__intvirtvm_describe", dumpPath, labels["testcase"], additional)
	err := os.WriteFile(fileName, describeCmd.StdOutBytes(), 0o644)
	if err != nil {
		GinkgoWriter.Printf("Failed to save InternalVirtualizationVirtualMachine description:\nError: %s\n", err)
	}
}

func (f *Framework) writePodLogs(ctx context.Context, name, namespace, filePath, leadNodeText string, labels map[string]string) {
	podLogs, err := f.Clients.KubeClient().CoreV1().Pods(namespace).GetLogs(name, &corev1.PodLogOptions{}).Stream(ctx)
	if err != nil {
		GinkgoWriter.Printf("Failed to get logs:\nPodName: %s\nError: %w\n", name, err)
	}
	defer podLogs.Close()

	logs, err := io.ReadAll(podLogs)
	if err != nil {
		GinkgoWriter.Printf("Failed to read logs:\nPodName: %s\nError: %w\n", name, err)
	}

	fileName := fmt.Sprintf("%s/e2e_failed__%s__%s__%s__logs.json", filePath, labels["testcase"], leadNodeText, name)
	err = os.WriteFile(fileName, logs, 0o644)
	if err != nil {
		GinkgoWriter.Printf("Failed to save logs:\nPodName: %s\nError: %w\n", name, err)
	}
}

func (f *Framework) writePodDescription(name, namespace, filePath, leadNodeText string, labels map[string]string) {
	describeCmd := f.Clients.Kubectl().RawCommand(fmt.Sprintf("describe pod %s --namespace %s", name, namespace), ShortTimeout)
	if describeCmd.Error() != nil {
		GinkgoWriter.Printf("Failed to describe pod:\nPodName: %s\nError: %s\n", name, describeCmd.StdErr())
	}

	fileName := fmt.Sprintf("%s/e2e_failed__%s__%s__%s__describe", filePath, labels["testcase"], leadNodeText, name)
	err := os.WriteFile(fileName, describeCmd.StdOutBytes(), 0o644)
	if err != nil {
		GinkgoWriter.Printf("Failed to save pod description:\nPodName: %s\nError: %w\n", name, err)
	}
}

func (f *Framework) writeVirtualMachineGuestInfo(pod corev1.Pod, filePath, leadNodeText string, labels map[string]string) {
	if pod.Labels != nil && pod.Status.Phase == corev1.PodRunning {
		if value, ok := pod.Labels["kubevirt.internal.virtualization.deckhouse.io"]; ok && value == "virt-launcher" {
			vlctlGuestInfoCmd := f.Clients.Kubectl().RawCommand(fmt.Sprintf("exec --stdin=true --tty=true %s --namespace %s -- vlctl guest info", pod.Name, pod.Namespace), ShortTimeout)
			if vlctlGuestInfoCmd.Error() != nil {
				GinkgoWriter.Printf("Failed to get pod guest info:\nPodName: %s\nError: %s\n", pod.Name, vlctlGuestInfoCmd.StdErr())
			}

			fileName := fmt.Sprintf("%s/e2e_failed__%s__%s__%s__vlctl_guest_info", filePath, labels["testcase"], leadNodeText, pod.Name)
			err := os.WriteFile(fileName, vlctlGuestInfoCmd.StdOutBytes(), 0o644)
			if err != nil {
				GinkgoWriter.Printf("Failed to save pod guest info:\nPodName: %s\nError: %w\n", pod.Name, err)
			}
		}
	}
}
