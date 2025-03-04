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
	"time"
)

// getRunningVM fetches the name of the running VM in a loop until one is found
func getRunningVM(ctx context.Context) string {
	for {
		select {
		case <-ctx.Done():
			slog.Warn("Context cancelled while waiting for VM name")
			return ""
		default:
			out, err := exec.Command("virsh", "list", "--name").Output()
			if err == nil {
				vmName := string(out)
				if len(vmName) > 0 {
					return vmName
				}
			}
			time.Sleep(1 * time.Second)
		}
	}
}

// setDomainRebootAction sets the reboot action to "shutdown" for the VM
func setDomainRebootAction(vmName string) {
	slog.Info("Setting reboot action to shutdown", "domain", vmName)
	cmd := exec.Command("virsh", "qemu-monitor-command", vmName,
		`{"execute": "set-action", "arguments":{"reboot":"shutdown"}}`)

	if err := cmd.Run(); err != nil {
		slog.Error("Failed to set reboot action", "error", err)
	}
}

// monitorDomainEvents listens for domain shutdown events and logs them to /dev/termination-log
func monitorDomainEvents(vmName string) {
	slog.Info("Monitoring domain events", "domain", vmName)

	cmd := exec.Command("virsh", "qemu-monitor-event", "--domain", vmName, "--loop", "--event", "SHUTDOWN")
	file, err := os.Create("/dev/termination-log")
	if err != nil {
		slog.Error("Failed to open termination log", "error", err)
		os.Exit(1)
	}
	defer file.Close()

	cmd.Stdout = file
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		slog.Error("Failed to monitor domain events", "error", err)
	}
}

func main() {
	// Setup structured logging
	opts := &slog.HandlerOptions{
		Level: slog.LevelInfo, // Adjust log level here if needed
	}
	logger := slog.New(slog.NewJSONHandler(os.Stdout, opts))
	slog.SetDefault(logger)

	slog.Info("Starting domain monitor daemon", "component", "virt-launcher-monitor-wrapper")

	// Run the domain monitor in a separate goroutine
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go func() {
		vmName := getRunningVM(ctx)
		if vmName == "" {
			slog.Error("No VM detected, exiting domain monitor goroutine")
			return
		}
		setDomainRebootAction(vmName)
		monitorDomainEvents(vmName)
	}()

	// Check if the original `virt-launcher-monitor-orig` exists
	origMonitorPath := "/usr/bin/virt-launcher-monitor-orig"
	if _, err := os.Stat(origMonitorPath); os.IsNotExist(err) {
		slog.Error("Missing original monitor binary", "path", origMonitorPath)
		os.Exit(1)
	}

	slog.Info("Executing original virt-launcher-monitor", "component", "virt-launcher-monitor-wrapper")

	// Execute the original `virt-launcher-monitor-orig` with passed arguments
	cmd := exec.Command(origMonitorPath, os.Args[1:]...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		slog.Error("Failed to execute original monitor", "error", err, "path", origMonitorPath)
		os.Exit(1)
	}
}
