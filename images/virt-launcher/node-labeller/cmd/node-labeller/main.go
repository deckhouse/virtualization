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
	"fmt"
	"log/slog"
	"node-labeller/pkg/helpers"
	"os"
)

func main() {
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	slog.SetDefault(logger)

	arch := helpers.GetArch()
	slog.Info(fmt.Sprintf("Get arch %s", arch))
	machine := helpers.GetMachineType(arch)
	if machine == "" {
		slog.Info("Unsupported architecture, exit gracefully")
		return
	}

	kvmMinor := helpers.GetKVMMinor()
	virtType := "qemu"

	if !helpers.FileExists("/dev/kvm") && kvmMinor != "" {
		if err := helpers.CreateKVMDevice(kvmMinor); err != nil {
			logger.Error("Failed to create /dev/kvm device", "error", err)
		}
	}

	if helpers.FileExists("/dev/kvm") {
		if err := helpers.SetPermissionsRW("/dev/kvm"); err != nil {
			logger.Error("Failed to set permissions for /dev/kvm", "error", err)
		}
		virtType = "kvm"
	}

	//QEMU requires RW access to query SEV capabilities
	if helpers.FileExists("/dev/sev") {
		if err := helpers.SetPermissionsRW("/dev/sev"); err != nil {
			logger.Error("Failed to set permissions for /dev/sev", "error", err)
		}
	}

	slog.Info("Start virtqemud daemon")
	if err := helpers.StartVirtqemud(); err != nil {
		logger.Error("Failed to start virtqemud", "error", err)
		return
	}

	outDir := "/var/lib/kubevirt-node-labeller"
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		logger.Error("Failed to create output directory", "path", outDir, "error", err)
		return
	}

	slog.Info("Execute virsh domcapabilities")
	domCapsPath := fmt.Sprintf("%s/virsh_domcapabilities.xml", outDir)
	err := helpers.RunCommandToFile("virsh", []string{
		"domcapabilities", "--machine", machine, "--arch", arch, "--virttype", virtType,
	}, domCapsPath)
	if err != nil {
		logger.Error("Failed to retrieve virsh domcapabilities", "error", err)
		return
	}

	// hypervisor-cpu-baseline command only works on x86_64
	if arch == "x86_64" {
		supportedFeaturesPath := fmt.Sprintf("%s/supported_features.xml", outDir)
		err = helpers.RunPipelineToFile([][]string{
			{"virsh", "domcapabilities", "--machine", machine, "--arch", arch, "--virttype", virtType},
			{"virsh", "hypervisor-cpu-baseline", "--features", "/dev/stdin", "--machine", machine, "--arch", arch,
				"--virttype", virtType},
		}, supportedFeaturesPath)
		if err != nil {
			logger.Error("Failed to retrieve supported CPU features", "error", err)
		}
	}

	slog.Info("Execute virsh capabilities")
	capabilitiesPath := fmt.Sprintf("%s/capabilities.xml", outDir)
	err = helpers.RunCommandToFile("virsh", []string{"capabilities"}, capabilitiesPath)
	if err != nil {
		logger.Error("Failed to retrieve virsh capabilities", "error", err)
	}
	slog.Info(fmt.Sprintf("Virsh capabilities saved to %s", capabilitiesPath))
}
