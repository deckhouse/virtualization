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
	messageMap := map[string]string{
		"error": err.Error(),
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
