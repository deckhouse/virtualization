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

	"k8s.io/klog/v2"
	"kubevirt.io/containerized-data-importer/pkg/util"
)

type ImportInfo struct {
	SourceImageSize        uint64        `json:"source-image-size,omitempty"`
	SourceImageVirtualSize uint64        `json:"source-image-virtual-size,omitempty"`
	SourceImageFormat      string        `json:"source-image-format,omitempty"`
	Duration               time.Duration `json:"duration,omitempty"`
	AverageSpeed           uint64        `json:"average-speed,omitempty"`
	ErrMessage             string        `json:"error-message,omitempty"`
}

var ErrFailedTerminationMessage = errors.New("failed to write termination message")

// WriteImportFailureMessage persists the import error to the termination
// message and returns the original import error, so that callers can
// `return monitoring.WriteImportFailureMessage(err)` and still fail the
// process. A failed import must terminate the pod in the Failed phase:
// a pod that completes successfully makes the controller read the (empty)
// size stats of the failed import and choke on them.
func WriteImportFailureMessage(importErr error) error {
	rawMsg, err := json.Marshal(ImportInfo{
		ErrMessage: importErr.Error(),
	})
	if err != nil {
		return errors.Join(importErr, fmt.Errorf("%w: %w", ErrFailedTerminationMessage, err))
	}

	message := string(rawMsg)

	err = util.WriteTerminationMessage(message)
	if err != nil {
		return errors.Join(importErr, fmt.Errorf("%w: %w", ErrFailedTerminationMessage, err))
	}

	klog.Infoln("Failed to save image to DVCR: " + message)

	return importErr
}

func WriteDVCRSourceImportCompleteMessage(duration time.Duration) error {
	rawMsg, err := json.Marshal(ImportInfo{
		Duration: duration,
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

func WriteImportCompleteMessage(sourceImageSize, sourceImageVirtualSize, avgSpeed uint64, sourceImageFormat string, duration time.Duration) error {
	rawMsg, err := json.Marshal(ImportInfo{
		SourceImageSize:        sourceImageSize,
		SourceImageVirtualSize: sourceImageVirtualSize,
		SourceImageFormat:      sourceImageFormat,
		AverageSpeed:           avgSpeed,
		Duration:               duration,
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
