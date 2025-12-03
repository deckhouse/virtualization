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
func (f *Framework) saveTestCaseDump() {
	ft := GetFormattedTestCaseFullText()
	tmpDir := GetTMPDir()

	f.saveTestCaseResources(ft, tmpDir)
	f.savePodAdditionalInfo(ft, tmpDir)
	f.saveIntvirtvmDescriptions(ft, tmpDir)
	f.saveIntvirtvmiDescriptions(ft, tmpDir)
}

// GetFormattedTestCaseFullText returns CurrentSpecReport().FullText(), formatted with the following rules:
//
//	" " -> "_",
//	":" -> "_",
//	"[" -> "_",
//	"]" -> "_",
//	"(" -> "_",
//	")" -> "_",
//	"|" -> "_",
//	"`" -> "",
//	"'" -> "",
func GetFormattedTestCaseFullText() string {
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
	return replacer.Replace(strings.ToLower(CurrentSpecReport().FullText()))
}

// GetTMPDir returns the temporary directory used for the test case resource dump.
func GetTMPDir() string {
	tmpDir := os.Getenv("RUNNER_TEMP")
	if tmpDir == "" {
		return "/tmp"
	}
	return tmpDir
}

func (f *Framework) saveTestCaseResources(testCaseFullText, dumpPath string) {
	resFileName := fmt.Sprintf("%s/e2e_failed__%s.yaml", dumpPath, testCaseFullText)

	// TODO: Add CVI and VMC to the request when the environment is isolated.
	result := f.Clients.Kubectl().Get("virtualization,intvirt,pod,volumesnapshot,pvc", kubectl.GetOptions{
		Namespace:         f.Namespace().Name,
		Output:            "yaml",
		ShowManagedFields: true,
	})
	if result.Error() != nil {
		GinkgoWriter.Printf("Get resources error:\n%s\n%w\n%s\n", result.GetCmd(), result.Error(), result.StdErr())
	}

	// Stdout may present even if error is occurred.
	if len(result.StdOutBytes()) > 0 {
		err := os.WriteFile(resFileName, result.StdOutBytes(), 0o644)
		if err != nil {
			GinkgoWriter.Printf("Save resources to file '%s' failed: %s\n", resFileName, err)
		}
	}
}

func (f *Framework) savePodAdditionalInfo(testCaseFullText, dumpPath string) {
	pods, err := f.Clients.kubeClient.CoreV1().Pods(f.Namespace().Name).List(context.Background(), metav1.ListOptions{})
	if err != nil {
		GinkgoWriter.Printf("Failed to get PodList:\n%s\n", err)
		return
	}

	if len(pods.Items) == 0 {
		GinkgoWriter.Println("The list of pods is empty; nothing to dump.")
		return
	}

	for _, pod := range pods.Items {
		f.writePodLogs(pod.Name, pod.Namespace, dumpPath, testCaseFullText)
		f.writePodDescription(pod.Name, pod.Namespace, dumpPath, testCaseFullText)
		f.writeVirtualMachineGuestInfo(pod, dumpPath, testCaseFullText)
	}
}

func (f *Framework) saveIntvirtvmDescriptions(testCaseFullText, dumpPath string) {
	describeCmd := f.Clients.Kubectl().RawCommand(fmt.Sprintf("describe intvirtvm --namespace %s", f.Namespace().Name), ShortTimeout)
	if describeCmd.Error() != nil {
		GinkgoWriter.Printf("Failed to describe InternalVirtualizationVirtualMachine:\nError: %s\n", describeCmd.StdErr())
	}

	fileName := fmt.Sprintf("%s/e2e_failed__%s__intvirtvm_describe", dumpPath, testCaseFullText)
	err := os.WriteFile(fileName, describeCmd.StdOutBytes(), 0o644)
	if err != nil {
		GinkgoWriter.Printf("Failed to save InternalVirtualizationVirtualMachine description:\nError: %s\n", err)
	}
}

func (f *Framework) saveIntvirtvmiDescriptions(testCaseFullText, dumpPath string) {
	describeCmd := f.Clients.Kubectl().RawCommand(fmt.Sprintf("describe intvirtvmi --namespace %s", f.Namespace().Name), ShortTimeout)
	if describeCmd.Error() != nil {
		GinkgoWriter.Printf("Failed to describe InternalVirtualizationVirtualMachineInstance:\nError: %s\n", describeCmd.StdErr())
	}

	fileName := fmt.Sprintf("%s/e2e_failed__%s__intvirtvmi_describe", dumpPath, testCaseFullText)
	err := os.WriteFile(fileName, describeCmd.StdOutBytes(), 0o644)
	if err != nil {
		GinkgoWriter.Printf("Failed to save InternalVirtualizationVirtualMachineInstance description:\nError: %s\n", err)
	}
}

func (f *Framework) writePodLogs(name, namespace, filePath, testCaseFullText string) {
	podLogs, err := f.Clients.KubeClient().CoreV1().Pods(namespace).GetLogs(name, &corev1.PodLogOptions{}).Stream(context.Background())
	if err != nil {
		GinkgoWriter.Printf("Failed to get logs:\nPodName: %s\nError: %w\n", name, err)
		return
	}
	defer podLogs.Close()

	logs, err := io.ReadAll(podLogs)
	if err != nil {
		GinkgoWriter.Printf("Failed to read logs:\nPodName: %s\nError: %w\n", name, err)
		return
	}

	fileName := fmt.Sprintf("%s/e2e_failed__%s__%s__logs.json", filePath, testCaseFullText, name)
	err = os.WriteFile(fileName, logs, 0o644)
	if err != nil {
		GinkgoWriter.Printf("Failed to save logs:\nPodName: %s\nError: %w\n", name, err)
	}
}

func (f *Framework) writePodDescription(name, namespace, filePath, testCaseFullText string) {
	describeCmd := f.Clients.Kubectl().RawCommand(fmt.Sprintf("describe pod %s --namespace %s", name, namespace), ShortTimeout)
	if describeCmd.Error() != nil {
		GinkgoWriter.Printf("Failed to describe pod:\nPodName: %s\nError: %s\n", name, describeCmd.StdErr())
	}

	fileName := fmt.Sprintf("%s/e2e_failed__%s__%s__describe", filePath, testCaseFullText, name)
	err := os.WriteFile(fileName, describeCmd.StdOutBytes(), 0o644)
	if err != nil {
		GinkgoWriter.Printf("Failed to save pod description:\nPodName: %s\nError: %w\n", name, err)
	}
}

func (f *Framework) writeVirtualMachineGuestInfo(pod corev1.Pod, filePath, testCaseFullText string) {
	if pod.Labels != nil && pod.Status.Phase == corev1.PodRunning {
		if value, ok := pod.Labels["kubevirt.internal.virtualization.deckhouse.io"]; ok && value == "virt-launcher" {
			vlctlGuestInfoCmd := f.Clients.Kubectl().RawCommand(fmt.Sprintf("exec --stdin=true --tty=true %s --namespace %s -- vlctl guest info", pod.Name, pod.Namespace), ShortTimeout)
			if vlctlGuestInfoCmd.Error() != nil {
				GinkgoWriter.Printf("Failed to get pod guest info:\nPodName: %s\nError: %s\n", pod.Name, vlctlGuestInfoCmd.StdErr())
			}

			fileName := fmt.Sprintf("%s/e2e_failed__%s__%s__vlctl_guest_info", filePath, testCaseFullText, pod.Name)
			err := os.WriteFile(fileName, vlctlGuestInfoCmd.StdOutBytes(), 0o644)
			if err != nil {
				GinkgoWriter.Printf("Failed to save pod guest info:\nPodName: %s\nError: %w\n", pod.Name, err)
			}
		}
	}
}
