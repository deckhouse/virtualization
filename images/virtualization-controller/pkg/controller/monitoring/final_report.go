package monitoring

import (
	"encoding/json"
	"errors"
	"fmt"

	"github.com/dustin/go-humanize"
	corev1 "k8s.io/api/core/v1"
)

// FinalReport example: { "source-image-size": "1111", "source-image-virtual-size": "8888", "source-image-format": "qcow2"}
type FinalReport struct {
	StoredSizeBytes   uint64 `json:"source-image-size"`
	UnpackedSizeBytes uint64 `json:"source-image-virtual-size"`
	Format            string `json:"source-image-format"`
	AverageSpeed      uint64 `json:"average-speed"`
}

func (r *FinalReport) StoredSize() string {
	return humanize.Bytes(r.StoredSizeBytes)
}

func (r *FinalReport) UnpackedSize() string {
	return humanize.Bytes(r.UnpackedSizeBytes)
}

func (r *FinalReport) GetAverageSpeed() string {
	return humanize.Bytes(r.AverageSpeed) + "/s"
}

func (r *FinalReport) GetAverageSpeedRaw() uint64 {
	return r.AverageSpeed
}

func GetFinalReportFromPod(pod *corev1.Pod) (*FinalReport, error) {
	if pod == nil {
		return nil, errors.New("got nil Pod: unable to get the final report from the nil Pod")
	}

	if len(pod.Status.ContainerStatuses) == 0 || pod.Status.ContainerStatuses[0].State.Terminated == nil {
		return nil, errors.New("termination message not found in the Pod status")
	}

	message := pod.Status.ContainerStatuses[0].State.Terminated.Message

	var report FinalReport
	err := json.Unmarshal([]byte(message), &report)
	if err != nil {
		return nil, fmt.Errorf("problem parsing final report %s: %w", message, err)
	}

	return &report, nil
}
