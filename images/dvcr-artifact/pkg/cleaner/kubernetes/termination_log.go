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

package kubernetes

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
)

const (
	PodTerminationMessageFile = "/dev/termination-log"
)

var ErrFailedTerminationMessage = errors.New("failed to write termination message")

func ReportTerminationMessage(err error, extra ...map[string]string) error {
	messageMap := map[string]string{}
	if err != nil {
		messageMap["error"] = err.Error()
	}
	for _, extraMap := range extra {
		for k, v := range extraMap {
			messageMap[k] = v
		}
	}

	message, err := json.Marshal(messageMap)
	if err != nil {
		return fmt.Errorf("%w: %w", ErrFailedTerminationMessage, err)
	}

	err = writeTerminationMessage(message)
	if err != nil {
		return fmt.Errorf("%w: %w", ErrFailedTerminationMessage, err)
	}

	return nil
}

func writeTerminationMessage(message []byte) error {
	err := os.WriteFile(PodTerminationMessageFile, message, 0600)
	if err != nil {
		return fmt.Errorf("write termination message to %s: %w", err)
	}
	return nil
}
