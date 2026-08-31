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

package monitoring

import (
	"errors"
	"testing"

	corev1 "k8s.io/api/core/v1"
)

func podWithStates(current, last *corev1.ContainerStateTerminated) *corev1.Pod {
	return &corev1.Pod{
		Status: corev1.PodStatus{
			ContainerStatuses: []corev1.ContainerStatus{
				{
					State:                corev1.ContainerState{Terminated: current},
					LastTerminationState: corev1.ContainerState{Terminated: last},
				},
			},
		},
	}
}

func TestGetLastFinalReportFromCrashLoopingPod(t *testing.T) {
	// A provisioner pod runs with RestartPolicy: OnFailure, so while it backs off its
	// container is Waiting and the report of the failed attempt is in LastTerminationState.
	// Reading only the current state makes finding the reason a race with the kubelet.
	report, err := GetLastFinalReportFromPod(podWithStates(nil, &corev1.ContainerStateTerminated{
		Message: `{"error-message":"Unable to process data: no space","permanent":true}`,
	}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if report.ErrMessage != "Unable to process data: no space" {
		t.Fatalf("got %q, want the message from the last termination state", report.ErrMessage)
	}
	if !report.Permanent {
		t.Fatal("permanent flag lost")
	}
}

func TestGetLastFinalReportPrefersCurrentState(t *testing.T) {
	report, err := GetLastFinalReportFromPod(podWithStates(
		&corev1.ContainerStateTerminated{Message: `{"error-message":"current"}`},
		&corev1.ContainerStateTerminated{Message: `{"error-message":"stale"}`},
	))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if report.ErrMessage != "current" {
		t.Fatalf("got %q, want the current termination state to win", report.ErrMessage)
	}
}

func TestGetLastFinalReportWithoutAnyTerminationState(t *testing.T) {
	_, err := GetLastFinalReportFromPod(podWithStates(nil, nil))
	if !errors.Is(err, ErrTerminationMessageNotFound) {
		t.Fatalf("got %v, want ErrTerminationMessageNotFound", err)
	}
}

func TestGetFinalReportIgnoresPreviousAttempt(t *testing.T) {
	// Every other consumer of the report acts on it right away: an image is failed the moment
	// a report with an error message shows up. The report of an attempt that has already been
	// restarted says nothing about the one in flight, so it must not reach them.
	_, err := GetFinalReportFromPod(podWithStates(nil, &corev1.ContainerStateTerminated{
		Message: `{"error-message":"the registry blinked"}`,
	}))
	if !errors.Is(err, ErrTerminationMessageNotFound) {
		t.Fatalf("got %v, want ErrTerminationMessageNotFound", err)
	}
}

func TestGetFinalReportFromTerminatedPod(t *testing.T) {
	report, err := GetFinalReportFromPod(podWithStates(
		&corev1.ContainerStateTerminated{Message: `{"error-message":"current"}`}, nil,
	))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if report.ErrMessage != "current" {
		t.Fatalf("got %q, want the current termination state", report.ErrMessage)
	}
}
