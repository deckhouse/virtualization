package importer

import (
	"encoding/json"
	"fmt"

	"github.com/dustin/go-humanize"
	corev1 "k8s.io/api/core/v1"
)

// FinalReport example: { "source-image-size": "1111", "source-image-virtual-size": "8888", "source-image-format": "qcow2"}
type FinalReport struct {
	StoredSizeBytes   uint64 `json:"source-image-size"`
	UnpackedSizeBytes uint64 `json:"source-image-virtual-size"`
	Format            string `json:"source-image-format"`
}

func (r *FinalReport) StoredSize() string {
	return humanize.Bytes(r.StoredSizeBytes)
}

func (r *FinalReport) UnpackedSize() string {
	return humanize.Bytes(r.UnpackedSizeBytes)
}

// ImporterFinalReport
//
//nolint:revive
func ImporterFinalReport(pod *corev1.Pod) (*FinalReport, error) {
	if pod != nil && pod.Status.ContainerStatuses != nil &&
		pod.Status.ContainerStatuses[0].State.Terminated != nil {
		message := pod.Status.ContainerStatuses[0].State.Terminated.Message
		report := new(FinalReport)
		err := json.Unmarshal([]byte(message), report)
		if err != nil {
			return nil, fmt.Errorf("problem parsing final report %s: %w", message, err)
		}
		return report, nil
	}
	return nil, nil
}
