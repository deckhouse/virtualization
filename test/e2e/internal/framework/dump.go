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
	"path"
	"sort"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/yaml"

	"github.com/deckhouse/virtualization/test/e2e/internal/kubectl"
)

const (
	d8vContainerPrefix = "d8v"
	// maxTestNameLen is the maximum length of the test name portion in a dump filename.
	// Linux filesystems (ext4/xfs) limit filenames to 255 bytes. We reserve ~70 bytes
	// for the "e2e_failed__" prefix, "__<namespace>__events.yaml" suffix, and directory path.
	maxTestNameLen = 180
	delimiter      = "..."
)

// truncateTestName shortens s to at most maxLen bytes while keeping the text
// human-readable: it preserves a prefix and a suffix separated by ellipsis.
func truncateTestName(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	availableLen := maxLen - len(delimiter)
	halfAvailableLen := availableLen / 2
	return s[:halfAvailableLen] + delimiter + s[len(s)-(availableLen-halfAvailableLen):]
}

// SaveTestCaseDump dump some resources, logs and descriptions that may help in further diagnostic.
//
// NOTE: This method is called in AfterEach for failed specs only. Avoid to use Expect,
// as it fails without reporting. Better use GinkgoWriter to report errors at this point.
func (f *Framework) saveTestCaseDump(ctx context.Context) {
	ft := GetFormattedTestCaseFullText()
	tmpDir := GetTMPDir()
	dumpDir := path.Join(tmpDir, "e2e_failed", ft)

	err := os.MkdirAll(dumpDir, 0o755)
	if err != nil {
		GinkgoWriter.Printf("Failed to create dump directory:\nDir: %s\nError: %v\n", dumpDir, err)
		return
	}

	f.saveFailureSummary(ctx, dumpDir)
	f.saveTestCaseResources(ctx, dumpDir)
	f.saveVMScreenshots(ctx, dumpDir)
	f.saveVMSerialConsoles(ctx, dumpDir)
	f.savePodAdditionalInfo(ctx, dumpDir)
	f.saveIntvirtvmDescriptions(ctx, dumpDir)
	f.saveIntvirtvmiDescriptions(ctx, dumpDir)
	f.saveNodeAdditionalInfo(ctx, dumpDir)
	f.saveEvents(ctx, dumpDir)
	f.saveClusterNetworkInfo(ctx, dumpDir)
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
	result := replacer.Replace(strings.ToLower(CurrentSpecReport().FullText()))
	return truncateTestName(result, maxTestNameLen)
}

// GetTMPDir returns the temporary directory used for the test case resource dump.
func GetTMPDir() string {
	tmpDir := os.Getenv("RUNNER_TEMP")
	if tmpDir == "" {
		return "/tmp"
	}
	return tmpDir
}

func (f *Framework) saveTestCaseResources(ctx context.Context, dumpDir string) {
	resFileName := path.Join(dumpDir, "resources.yaml")

	// TODO: Add CVI and VMC to the request when the environment is isolated.
	result := f.Clients.Kubectl().GetContext(ctx, "virtualization,intvirt,pod,volumesnapshot,pvc", kubectl.GetOptions{
		Namespace: f.Namespace().Name,
		Output:    "yaml",
	})
	if result.Error() != nil {
		GinkgoWriter.Printf("Get resources error:\n%s\n%v\n%s\n", result.GetCmd(), result.Error(), result.StdErr())
	}

	// Stdout may present even if error is occurred.
	if len(result.StdOutBytes()) > 0 {
		err := os.WriteFile(resFileName, result.StdOutBytes(), 0o644)
		if err != nil {
			GinkgoWriter.Printf("Save resources to file '%s' failed: %s\n", resFileName, err)
		}
	}
}

func (f *Framework) savePodAdditionalInfo(ctx context.Context, dumpDir string) {
	pods, err := f.Clients.kubeClient.CoreV1().Pods(f.Namespace().Name).List(ctx, metav1.ListOptions{})
	if err != nil {
		GinkgoWriter.Printf("Failed to get PodList:\n%s\n", err)
		return
	}

	if len(pods.Items) == 0 {
		GinkgoWriter.Println("The list of pods is empty; nothing to dump.")
		return
	}

	for _, pod := range pods.Items {
		f.writePodLogs(ctx, pod.Name, pod.Namespace, dumpDir)
		f.writePodDescription(ctx, pod.Name, pod.Namespace, dumpDir)
		f.writeVirtualMachineGuestInfo(ctx, pod, dumpDir)
	}
}

func (f *Framework) saveIntvirtvmDescriptions(ctx context.Context, dumpDir string) {
	describeCmd := f.Clients.Kubectl().RawCommandContext(ctx, fmt.Sprintf("describe intvirtvm --namespace %s", f.Namespace().Name), ShortTimeout)
	if describeCmd.Error() != nil {
		GinkgoWriter.Printf("Failed to describe InternalVirtualizationVirtualMachine:\nError: %s\n", describeCmd.StdErr())
	}

	fileName := path.Join(dumpDir, "intvirtvm_describe.log")
	err := os.WriteFile(fileName, describeCmd.StdOutBytes(), 0o644)
	if err != nil {
		GinkgoWriter.Printf("Failed to save InternalVirtualizationVirtualMachine description:\nError: %s\n", err)
	}
}

func (f *Framework) saveIntvirtvmiDescriptions(ctx context.Context, dumpDir string) {
	describeCmd := f.Clients.Kubectl().RawCommandContext(ctx, fmt.Sprintf("describe intvirtvmi --namespace %s", f.Namespace().Name), ShortTimeout)
	if describeCmd.Error() != nil {
		GinkgoWriter.Printf("Failed to describe InternalVirtualizationVirtualMachineInstance:\nError: %s\n", describeCmd.StdErr())
	}

	fileName := path.Join(dumpDir, "intvirtvmi_describe.log")
	err := os.WriteFile(fileName, describeCmd.StdOutBytes(), 0o644)
	if err != nil {
		GinkgoWriter.Printf("Failed to save InternalVirtualizationVirtualMachineInstance description:\nError: %s\n", err)
	}
}

func (f *Framework) writePodLogs(ctx context.Context, name, namespace, dumpDir string) {
	pod, err := f.Clients.kubeClient.CoreV1().Pods(namespace).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		GinkgoWriter.Printf("Failed to get pod:\nPodName: %s\nError: %v\n", name, err)
		return
	}

	for _, container := range pod.Spec.Containers {
		if !strings.HasPrefix(container.Name, d8vContainerPrefix) {
			GinkgoWriter.Printf("Skipping container without d8v prefix:\nPodName: %s\nContainer: %s\n", pod.Name, container.Name)
			continue
		}
		f.writePodContainerLogs(ctx, pod, container.Name, dumpDir)
	}
}

func (f *Framework) writePodContainerLogs(ctx context.Context, pod *corev1.Pod, containerName, dumpDir string) {
	podLogs, err := f.Clients.KubeClient().CoreV1().Pods(pod.Namespace).GetLogs(pod.Name,
		&corev1.PodLogOptions{
			Container: containerName,
		}).Stream(ctx)
	if err != nil {
		GinkgoWriter.Printf("Failed to get logs:\nPodName: %s\nContainer: %s\nError: %v\n", pod.Name, containerName, err)
		return
	}
	defer podLogs.Close()

	logs, err := io.ReadAll(podLogs)
	if err != nil {
		GinkgoWriter.Printf("Failed to read logs:\nPodName: %s\nContainer: %s\nError: %v\n", pod.Name, containerName, err)
		return
	}

	fileName := path.Join(dumpDir, fmt.Sprintf("pod_%s__%s_logs.json", pod.Name, containerName))
	err = os.WriteFile(fileName, logs, 0o644)
	if err != nil {
		GinkgoWriter.Printf("Failed to save logs:\nPodName: %s\nContainer: %s\nError: %v\n", pod.Name, containerName, err)
	}
}

func (f *Framework) writePodDescription(ctx context.Context, name, namespace, dumpDir string) {
	describeCmd := f.Clients.Kubectl().RawCommandContext(ctx, fmt.Sprintf("describe pod %s --namespace %s", name, namespace), ShortTimeout)
	if describeCmd.Error() != nil {
		GinkgoWriter.Printf("Failed to describe pod:\nPodName: %s\nError: %s\n", name, describeCmd.StdErr())
	}

	fileName := path.Join(dumpDir, fmt.Sprintf("pod_%s_describe.log", name))
	err := os.WriteFile(fileName, describeCmd.StdOutBytes(), 0o644)
	if err != nil {
		GinkgoWriter.Printf("Failed to save pod description:\nPodName: %s\nError: %v\n", name, err)
	}
}

func (f *Framework) writeVirtualMachineGuestInfo(ctx context.Context, pod corev1.Pod, dumpDir string) {
	if pod.Labels != nil && pod.Status.Phase == corev1.PodRunning {
		if value, ok := pod.Labels["kubevirt.internal.virtualization.deckhouse.io"]; ok && value == "virt-launcher" {
			vlctlGuestInfoCmd := f.Clients.Kubectl().RawCommandContext(ctx, fmt.Sprintf("exec %s --namespace %s -- vlctl guest info", pod.Name, pod.Namespace), ShortTimeout)
			if vlctlGuestInfoCmd.Error() != nil {
				GinkgoWriter.Printf("Failed to get pod guest info:\nPodName: %s\nError: %s\n", pod.Name, vlctlGuestInfoCmd.StdErr())
			}

			fileName := path.Join(dumpDir, fmt.Sprintf("pod_%s_vlctl_guest_info.log", pod.Name))
			err := os.WriteFile(fileName, vlctlGuestInfoCmd.StdOutBytes(), 0o644)
			if err != nil {
				GinkgoWriter.Printf("Failed to save pod guest info:\nPodName: %s\nError: %v\n", pod.Name, err)
			}
		}
	}
}

func (f *Framework) saveNodeAdditionalInfo(ctx context.Context, dumpDir string) {
	GinkgoHelper()

	f.writeNodeDescription(ctx, dumpDir)
	f.writeNodeList(ctx, dumpDir)
}

func (f *Framework) writeNodeDescription(ctx context.Context, dumpDir string) {
	GinkgoHelper()

	cmd := f.Clients.Kubectl().RawCommandContext(ctx, "describe nodes", ShortTimeout)
	if cmd.Error() != nil {
		GinkgoWriter.Printf("Failed to run 'kubectl describe nodes':\nCmdError: %v\nStderr: %s\n", cmd.Error(), cmd.StdErr())
	}

	fileName := path.Join(dumpDir, "nodes_describe.log")
	if len(cmd.StdOutBytes()) > 0 {
		err := os.WriteFile(fileName, cmd.StdOutBytes(), 0o644)
		if err != nil {
			GinkgoWriter.Printf("Failed to write node description dump:\nFile: %s\nError: %v\n", fileName, err)
		}
	}
}

func (f *Framework) writeNodeList(ctx context.Context, dumpDir string) {
	GinkgoHelper()

	cmd := f.Clients.Kubectl().RawCommandContext(ctx, "get nodes -o wide", ShortTimeout)
	if cmd.Error() != nil {
		GinkgoWriter.Printf("Failed to run 'kubectl get nodes -o wide':\nCmdError: %v\nStderr: %s\n", cmd.Error(), cmd.StdErr())
	}

	fileName := path.Join(dumpDir, "nodes_owide.log")
	if len(cmd.StdOutBytes()) > 0 {
		err := os.WriteFile(fileName, cmd.StdOutBytes(), 0o644)
		if err != nil {
			GinkgoWriter.Printf("Failed to write node list dump (wide):\nFile: %s\nError: %v\n", fileName, err)
		}
	}
}

// summaryFileName sorts first in the dump directory: it is the file to read before
// any other.
const summaryFileName = "00_summary.log"

// maxSummaryLinesInOutput caps how much of the summary is echoed into the ginkgo
// output, so a failure states its cause without opening the dump at all.
const maxSummaryLinesInOutput = 20

// saveFailureSummary collects the lines that usually explain a failure - Warning
// events and containers that never started - into a single file and echoes the
// head of it to the ginkgo output. Without it the cause (a FailedMount message, a
// FailedScheduling reason) stays buried in events_<namespace>.yaml.
func (f *Framework) saveFailureSummary(ctx context.Context, dumpDir string) {
	var lines []string
	lines = append(lines, f.warningEventLines(ctx)...)
	lines = append(lines, f.stuckContainerLines(ctx)...)
	if len(lines) == 0 {
		return
	}

	fileName := path.Join(dumpDir, summaryFileName)
	err := os.WriteFile(fileName, []byte(strings.Join(lines, "\n")+"\n"), 0o644)
	if err != nil {
		GinkgoWriter.Printf("Failed to write failure summary:\nFile: %s\nError: %v\n", fileName, err)
	}

	GinkgoWriter.Printf("Failure summary (%s):\n", fileName)
	for i, line := range lines {
		if i == maxSummaryLinesInOutput {
			GinkgoWriter.Printf("  ... %d more line(s) in %s\n", len(lines)-i, summaryFileName)
			break
		}
		GinkgoWriter.Printf("  %s\n", line)
	}
}

// warningEventLines returns the namespace Warning events as single-line records,
// oldest first.
func (f *Framework) warningEventLines(ctx context.Context) []string {
	namespace := f.Namespace().Name
	events, err := f.Clients.kubeClient.CoreV1().Events(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		GinkgoWriter.Printf("Failed to get events for the failure summary:\nError: %v\n", err)
		return nil
	}

	warnings := make([]corev1.Event, 0, len(events.Items))
	for _, event := range events.Items {
		if event.Type == corev1.EventTypeWarning {
			warnings = append(warnings, event)
		}
	}
	sort.Slice(warnings, func(i, j int) bool {
		return eventTime(warnings[i]).Before(eventTime(warnings[j]))
	})

	lines := make([]string, 0, len(warnings))
	for _, event := range warnings {
		obj := event.InvolvedObject
		lines = append(lines, fmt.Sprintf("WARN  %s  %s  %s/%s  x%d  %s",
			eventTime(event).Format(time.RFC3339), event.Reason, obj.Kind, obj.Name, event.Count,
			strings.Join(strings.Fields(event.Message), " ")))
	}
	return lines
}

// stuckContainerLines reports containers that are not running, with the reason the
// kubelet gave: a pod stuck in ContainerCreating carries no logs to dump.
func (f *Framework) stuckContainerLines(ctx context.Context) []string {
	pods, err := f.Clients.kubeClient.CoreV1().Pods(f.Namespace().Name).List(ctx, metav1.ListOptions{})
	if err != nil {
		GinkgoWriter.Printf("Failed to get pods for the failure summary:\nError: %v\n", err)
		return nil
	}

	var lines []string
	for _, pod := range pods.Items {
		for _, status := range append(pod.Status.InitContainerStatuses, pod.Status.ContainerStatuses...) {
			switch {
			case status.State.Waiting != nil:
				lines = append(lines, fmt.Sprintf("POD   %s/%s  phase=%s  waiting=%s  %s",
					pod.Name, status.Name, pod.Status.Phase, status.State.Waiting.Reason,
					strings.Join(strings.Fields(status.State.Waiting.Message), " ")))
			case status.State.Terminated != nil && status.State.Terminated.ExitCode != 0:
				lines = append(lines, fmt.Sprintf("POD   %s/%s  phase=%s  terminated=%s  exit=%d  %s",
					pod.Name, status.Name, pod.Status.Phase, status.State.Terminated.Reason,
					status.State.Terminated.ExitCode,
					strings.Join(strings.Fields(status.State.Terminated.Message), " ")))
			}
		}
	}
	return lines
}

func eventTime(event corev1.Event) time.Time {
	if !event.LastTimestamp.IsZero() {
		return event.LastTimestamp.Time
	}
	if !event.EventTime.IsZero() {
		return event.EventTime.Time
	}
	return event.CreationTimestamp.Time
}

func (f *Framework) saveEvents(ctx context.Context, dumpDir string) {
	GinkgoHelper()
	namespace := f.Namespace().Name
	events, err := f.Clients.kubeClient.CoreV1().Events(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		GinkgoWriter.Printf("Failed to get events:\nError: %v\n", err)
		return
	}

	fileName := path.Join(dumpDir, fmt.Sprintf("events_%s.yaml", namespace))
	if len(events.Items) > 0 {
		data, err := yaml.Marshal(events)
		if err != nil {
			GinkgoWriter.Printf("Failed to marshal events dump:\nFile: %s\nError: %v\n", fileName, err)
			return
		}

		err = os.WriteFile(fileName, data, 0o644)
		if err != nil {
			GinkgoWriter.Printf("Failed to write events dump:\nFile: %s\nError: %v\n", fileName, err)
		}
	}
}

func (f *Framework) saveClusterNetworkInfo(ctx context.Context, dumpDir string) {
	GinkgoHelper()

	// Only for tests that use additional networks.
	if !strings.Contains(CurrentSpecReport().FullText(), "VirtualMachineAdditionalNetworkInterfaces") {
		return
	}

	// Get all ClusterNetwork resources
	cmd := f.Clients.Kubectl().RawCommandContext(ctx, "get clusternetwork -o yaml", ShortTimeout)
	if cmd.Error() != nil {
		GinkgoWriter.Printf("Failed to get clusternetwork:\nCmdError: %v\nStderr: %s\n", cmd.Error(), cmd.StdErr())
	}

	fileName := path.Join(dumpDir, "clusternetwork.yaml")
	if len(cmd.StdOutBytes()) > 0 {
		err := os.WriteFile(fileName, cmd.StdOutBytes(), 0o644)
		if err != nil {
			GinkgoWriter.Printf("Failed to write clusternetwork dump:\nFile: %s\nError: %v\n", fileName, err)
		}
	}

	// Get CEP (Cilium Endpoints) for the namespace
	cepCmd := f.Clients.Kubectl().RawCommandContext(ctx, fmt.Sprintf("get cep -n %s -o yaml", f.Namespace().Name), ShortTimeout)
	if cepCmd.Error() != nil {
		GinkgoWriter.Printf("Failed to get cep:\nCmdError: %v\nStderr: %s\n", cepCmd.Error(), cepCmd.StdErr())
	}

	cepFileName := path.Join(dumpDir, "cep.yaml")
	if len(cepCmd.StdOutBytes()) > 0 {
		err := os.WriteFile(cepFileName, cepCmd.StdOutBytes(), 0o644)
		if err != nil {
			GinkgoWriter.Printf("Failed to write cep dump:\nFile: %s\nError: %v\n", cepFileName, err)
		}
	}
}
