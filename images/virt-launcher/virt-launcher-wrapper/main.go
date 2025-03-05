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

package main

import (
	"context"
	"log/slog"
	"os"
	"os/exec"
	"strings"
	"time"
)

// Logger instance using slog
var logger = slog.New(slog.NewJSONHandler(os.Stdout, nil))

// findVMName retrieves the VM name by running "virsh list --name"
func findVMName(ctx context.Context) string {
	for {
		select {
		case <-ctx.Done():
			return ""
		default:
			out, err := exec.Command("virsh", "list", "--name").Output()
			if err == nil {
				vmName := strings.TrimSpace(string(out))
				if vmName != "" {
					return vmName
				}
			}
			time.Sleep(1 * time.Second) // Wait before retrying
		}
	}
}

// monitorVM manages the VM lifecycle and listens for shutdown events
func monitorVM(ctx context.Context) {
	vmName := findVMName(ctx)
	if vmName == "" {
		logger.Warn("No VM detected, exiting VM monitor")
		return
	}

	logger.Info("Setting reboot action to shutdown", slog.String("domain", vmName))

	// Set reboot action to shutdown
	err := exec.Command("virsh", "qemu-monitor-command", vmName, `{"execute": "set-action", "arguments":{"reboot":"shutdown"}}`).Run()
	if err != nil {
		logger.Error("Failed to set reboot action", slog.String("domain", vmName), slog.Any("error", err))
		return
	}

	logger.Info("Monitoring domain events", slog.String("domain", vmName))

	// Monitor VM shutdown event and write to termination log
	cmd := exec.Command("virsh", "qemu-monitor-event", "--domain", vmName, "--loop", "--event", "SHUTDOWN")
	logFile, err := os.OpenFile("/dev/termination-log", os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		logger.Error("Failed to open termination log", slog.Any("error", err))
		return
	}
	defer logFile.Close()

	cmd.Stdout = logFile
	cmd.Stderr = logFile

	err = cmd.Run()
	if err != nil {
		logger.Error("Error in monitoring domain events", slog.Any("error", err))
	}
}

func main() {
	logger.Info("Start domain monitor daemon", slog.String("component", "virt-launcher-monitor-wrapper"))

	// Create a context to allow proper cleanup if needed
	ctx := context.Background()

	// Run domain monitor in a separate Goroutine
	go monitorVM(ctx)

	// Ensure target executable exists before launching
	targetBinary := "/usr/bin/virt-launcher-monitor-orig"
	if _, err := os.Stat(targetBinary); os.IsNotExist(err) {
		logger.Error("Target binary is absent", slog.String("path", targetBinary))
		os.Exit(1)
	}

	logger.Info("Executing original virt-launcher-monitor", slog.String("path", targetBinary))

	// Run original virt-launcher-monitor with provided arguments
	cmd := exec.Command(targetBinary, os.Args[1:]...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		logger.Error("Error running target executable", slog.Any("error", err))
		os.Exit(1)
	}
}
