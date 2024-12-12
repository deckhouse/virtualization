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

func GetFinalReportFromPod(pod *corev1.Pod) (*FinalReport, error) {
	if pod == nil {
		return nil, errors.New("got nil Pod: unable to get the final report from the nil Pod")
	}

	if len(pod.Status.ContainerStatuses) == 0 || pod.Status.ContainerStatuses[0].State.Terminated == nil {
		return nil, ErrTerminationMessageNotFound
	}

	message := pod.Status.ContainerStatuses[0].State.Terminated.Message

	if message == "" {
		return nil, ErrTerminationMessageNotFound
	}

	var report FinalReport
	err := json.Unmarshal([]byte(message), &report)
	if err != nil {
		return nil, fmt.Errorf("problem parsing final report %s: %w", message, err)
	}

	return &report, nil
}
