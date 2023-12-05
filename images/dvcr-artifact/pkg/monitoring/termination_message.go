package monitoring

import (
	"encoding/json"
	"errors"
	"fmt"
	"k8s.io/klog/v2"
	"kubevirt.io/containerized-data-importer/pkg/util"
)

type ImportInfo struct {
	SourceImageSize        uint64 `json:"source-image-size,omitempty"`
	SourceImageVirtualSize uint64 `json:"source-image-virtual-size,omitempty"`
	SourceImageFormat      string `json:"source-image-format,omitempty"`
	AverageSpeed           uint64 `json:"average-speed,omitempty"`
	ErrMessage             string `json:"error-message,omitempty"`
}

var ErrFailedTerminationMessage = errors.New("failed to write termination message")

func WriteImportFailureMessage(err error) error {
	rawMsg, err := json.Marshal(ImportInfo{
		ErrMessage: err.Error(),
	})
	if err != nil {
		return fmt.Errorf("%w: %w", ErrFailedTerminationMessage, err)
	}

	message := string(rawMsg)

	err = util.WriteTerminationMessage(message)
	if err != nil {
		return fmt.Errorf("%w: %w", ErrFailedTerminationMessage, err)
	}

	klog.Infoln("Failed to save image to DVCR: " + message)

	return nil
}

func WriteImportCompleteMessage(sourceImageSize, sourceImageVirtualSize, avgSpeed uint64, sourceImageFormat string) error {
	rawMsg, err := json.Marshal(ImportInfo{
		SourceImageSize:        sourceImageSize,
		SourceImageVirtualSize: sourceImageVirtualSize,
		SourceImageFormat:      sourceImageFormat,
		AverageSpeed:           avgSpeed,
	})
	if err != nil {
		return fmt.Errorf("%w: %w", ErrFailedTerminationMessage, err)
	}

	message := string(rawMsg)

	err = util.WriteTerminationMessage(message)
	if err != nil {
		return fmt.Errorf("%w: %w", ErrFailedTerminationMessage, err)
	}

	klog.Infoln("Image is saved in DVCR: " + message)

	return nil
}
