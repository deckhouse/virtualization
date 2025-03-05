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
	"bytes"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"runtime"
	"strings"
)

func main() {
	// Setup logger
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	slog.SetDefault(logger)

	arch := getArch()
	machine := getMachineType(arch)
	if machine == "" {
		return // Unsupported architecture, exit gracefully
	}

	kvmMinor := getKVMMinor()
	virtType := "qemu"

	if !fileExists("/dev/kvm") && kvmMinor != "" {
		if err := createKVMDevice(kvmMinor); err != nil {
			logger.Error("Failed to create /dev/kvm device", "error", err)
		}
	}

	if fileExists("/dev/kvm") {
		if err := setPermissionsGo("/dev/kvm"); err != nil {
			logger.Error("Failed to set permissions for /dev/kvm", "error", err)
		}
		virtType = "kvm"
	}

	if fileExists("/dev/sev") {
		if err := setPermissionsGo("/dev/sev"); err != nil {
			logger.Error("Failed to set permissions for /dev/sev", "error", err)
		}
	}

	// Start virtqemud daemon
	if err := startVirtqemud(); err != nil {
		logger.Error("Failed to start virtqemud", "error", err)
		return
	}

	outDir := "/var/lib/kubevirt-node-labeller"
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		logger.Error("Failed to create output directory", "path", outDir, "error", err)
		return
	}

	// Execute virsh domcapabilities
	domCapsPath := fmt.Sprintf("%s/virsh_domcapabilities.xml", outDir)
	err := runCommandToFile("virsh", []string{
		"domcapabilities", "--machine", machine, "--arch", arch, "--virttype", virtType,
	}, domCapsPath)
	if err != nil {
		logger.Error("Failed to retrieve virsh domcapabilities", "error", err)
		return
	}

	// hypervisor-cpu-baseline command only works on x86_64
	if arch == "x86_64" {
		supportedFeaturesPath := fmt.Sprintf("%s/supported_features.xml", outDir)
		err = runPipelineToFile([][]string{
			{"virsh", "domcapabilities", "--machine", machine, "--arch", arch, "--virttype", virtType},
			{"virsh", "hypervisor-cpu-baseline", "--features", "/dev/stdin", "--machine", machine, "--arch", arch, "--virttype", virtType},
		}, supportedFeaturesPath)
		if err != nil {
			logger.Error("Failed to retrieve supported CPU features", "error", err)
		}
	}

	// Execute virsh capabilities
	capabilitiesPath := fmt.Sprintf("%s/capabilities.xml", outDir)
	err = runCommandToFile("virsh", []string{"capabilities"}, capabilitiesPath)
	if err != nil {
		logger.Error("Failed to retrieve virsh capabilities", "error", err)
	}
}

func getArch() string {
	output := runtime.GOARCH
	// output, err := runCommand("uname", []string{"-m"})
	// if err != nil {
	// 	slog.Error("Failed to retrieve architecture", "error", err)
	// 	return ""
	// }
	if output == "amd64" {
		output = "x86_64"
	}
	return strings.TrimSpace(output)
}

func getMachineType(arch string) string {
	switch arch {
	case "aarch64":
		return "virt"
	case "s390x":
		return "s390-ccw-virtio"
	case "x86_64":
		return "q35"
	default:
		slog.Info("Unsupported architecture", "arch", arch)
		return "" // Unsupported architecture, exit gracefully
	}
}

func getKVMMinor() string {
	data, err := os.ReadFile("/proc/misc")
	if err != nil {
		slog.Error("Failed to read /proc/misc", "error", err)
		return ""
	}

	lines := strings.Split(string(data), "\n")
	for _, line := range lines {
		fields := strings.Fields(line)
		if len(fields) >= 2 && fields[1] == "kvm" {
			return fields[0] // Return the minor number
		}
	}
	return ""
}

func createKVMDevice(minor string) error {
	return runCommandWithError("mknod", []string{"/dev/kvm", "c", "10", minor})
}

// Use os.Chmod instead of chmod command
func setPermissionsGo(path string) error {
	mode := os.FileMode(0o666) // Equivalent to `chmod o+rw`
	return os.Chmod(path, mode)
}

func startVirtqemud() error {
	return runCommandWithError("virtqemud", []string{"-d"})
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func runCommand(cmd string, args []string) (string, error) {
	out, err := exec.Command(cmd, args...).CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("%s failed: %w - output: %s", cmd, err, string(out))
	}
	return string(out), nil
}

func runCommandWithError(cmd string, args []string) error {
	_, err := runCommand(cmd, args)
	return err
}

func runCommandToFile(cmd string, args []string, outputPath string) error {
	cmdExec := exec.Command(cmd, args...)
	outBytes, err := cmdExec.Output()
	if err != nil {
		return fmt.Errorf("error running %s: %w", cmd, err)
	}
	return os.WriteFile(outputPath, outBytes, 0o644)
}

func runPipelineToFile(commands [][]string, outputPath string) error {
	var outputBuffer bytes.Buffer
	var inputPipe *bytes.Buffer

	for i, cmdArgs := range commands {
		cmd := exec.Command(cmdArgs[0], cmdArgs[1:]...)

		// If not first command, use previous output as input
		if i > 0 {
			cmd.Stdin = inputPipe
		}

		// Capture output
		var outBuffer bytes.Buffer
		cmd.Stdout = &outBuffer
		if err := cmd.Run(); err != nil {
			return fmt.Errorf("command failed: %v - error: %w", cmdArgs, err)
		}

		// Set new input for next command
		inputPipe = &outBuffer
		outputBuffer = outBuffer
	}

	return os.WriteFile(outputPath, outputBuffer.Bytes(), 0o644)
}
