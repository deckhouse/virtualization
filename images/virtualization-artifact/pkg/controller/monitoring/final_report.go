/*
Copyright 2024 Flant JSC

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

package monitoring

import (
	"encoding/json"
	"errors"
	"fmt"
	"time"

	corev1 "k8s.io/api/core/v1"

	"github.com/deckhouse/virtualization-controller/pkg/common/humanize_bytes"
)

// FinalReport example: { "source-image-size": 1111, "source-image-virtual-size": 8888, "source-image-format": "qcow2"}
type FinalReport struct {
	StoredSizeBytes   uint64        `json:"source-image-size,omitempty"`
	UnpackedSizeBytes uint64        `json:"source-image-virtual-size,omitempty"`
	Format            string        `json:"source-image-format,omitempty"`
	Duration          time.Duration `json:"duration,omitempty"`
	AverageSpeed      uint64        `json:"average-speed,omitempty"`
	ErrMessage        string        `json:"error-message,omitempty"`
	// Permanent is set by the provisioner when retrying cannot change the outcome, so a
	// transient failure is not mistaken for a dead import: the pod restarts either way.
	Permanent bool `json:"permanent,omitempty"`
}

func (r *FinalReport) GetAverageSpeed() string {
	return humanize_bytes.HumanizeIBytes(r.AverageSpeed) + "/s"
}

func (r *FinalReport) GetAverageSpeedRaw() uint64 {
	return r.AverageSpeed
}

func (r *FinalReport) GetImportDuration() time.Duration {
	return r.Duration
}

var ErrTerminationMessageNotFound = errors.New("termination message not found in the Pod status")

// GetFinalReportFromPod returns the report of the container's current termination. A pod that
// is restarting has none: its report belongs to the previous attempt and lives in
// LastTerminationState, see GetLastFinalReportFromPod.
func GetFinalReportFromPod(pod *corev1.Pod) (*FinalReport, error) {
	status, err := firstContainerStatus(pod)
	if err != nil {
		return nil, err
	}
	return parseFinalReport(status.State.Terminated)
}

// GetLastFinalReportFromPod returns the report of the latest finished attempt: the current
// termination if the container has one, the previous one otherwise.
//
// Provisioner pods run with RestartPolicy: OnFailure, so a pod that keeps failing sits in
// CrashLoopBackOff with its container Waiting: the report of the attempt that failed is in
// LastTerminationState, and State.Terminated is only filled during the brief moment between the
// exit and the restart. Reading just the current state turns finding the reason into a race with
// the kubelet.
//
// The report of a previous attempt says nothing about the attempt in flight, so a caller must
// decide for itself whether a stale verdict may be acted upon.
func GetLastFinalReportFromPod(pod *corev1.Pod) (*FinalReport, error) {
	status, err := firstContainerStatus(pod)
	if err != nil {
		return nil, err
	}
	terminated := status.State.Terminated
	if terminated == nil {
		terminated = status.LastTerminationState.Terminated
	}
	return parseFinalReport(terminated)
}

func firstContainerStatus(pod *corev1.Pod) (*corev1.ContainerStatus, error) {
	if pod == nil {
		return nil, errors.New("got nil Pod: unable to get the final report from the nil Pod")
	}
	if len(pod.Status.ContainerStatuses) == 0 {
		return nil, ErrTerminationMessageNotFound
	}
	return &pod.Status.ContainerStatuses[0], nil
}

func parseFinalReport(terminated *corev1.ContainerStateTerminated) (*FinalReport, error) {
	if terminated == nil || terminated.Message == "" {
		return nil, ErrTerminationMessageNotFound
	}

	var report FinalReport
	err := json.Unmarshal([]byte(terminated.Message), &report)
	if err != nil {
		return nil, fmt.Errorf("problem parsing final report %s: %w", terminated.Message, err)
	}

	return &report, nil
}

// PermanentFailureFromPod returns the reason a provisioner pod reported when restarting it
// cannot change the outcome, and an empty string otherwise. Provisioner pods run with
// RestartPolicy: OnFailure and never reach PodFailed, so without this a permanently broken
// import (the image does not fit, the target refuses the write) reports "provisioning" forever,
// with the reason visible only in the log of an attempt the restarts push out within seconds.
//
// A transient failure (the registry blinked, DVCR was restarted mid-import) is deliberately not
// surfaced: the kubelet restarts the pod and the import carries on.
func PermanentFailureFromPod(pod *corev1.Pod) string {
	if pod == nil {
		return ""
	}

	// A running container means a new attempt is under way, and the report on hand belongs to
	// the previous one. Reporting it now would call the import failed while one that may well
	// succeed is in flight: the condition that caused the failure can be fixed from the outside
	// (the share extended, the quota raised) between two restarts.
	if len(pod.Status.ContainerStatuses) > 0 && pod.Status.ContainerStatuses[0].State.Running != nil {
		return ""
	}

	// An unreadable report is not a reason to fail: keep waiting and let the restart produce a
	// readable one.
	report, err := GetLastFinalReportFromPod(pod)
	if err != nil || report == nil || !report.Permanent || report.ErrMessage == "" {
		return ""
	}

	return report.ErrMessage
}
