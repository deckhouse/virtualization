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
	"os"

	"node-labeller/pkg/helpers"

	"libvirt.org/go/libvirt"
)

const (
	virtType = "qemu"
	outDir   = "/var/lib/kubevirt-node-labeller"
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

	if !helpers.FileExists("/dev/kvm") && kvmMinor != "" {
		if err := helpers.CreateKVMDevice(kvmMinor); err != nil {
			logger.Error("Failed to create /dev/kvm device", "error", err)
			os.Exit(1)
		}
	}

	if helpers.FileExists("/dev/kvm") {
		if err := helpers.SetPermissionsRW("/dev/kvm"); err != nil {
			logger.Error("Failed to set permissions for /dev/kvm", "error", err)
			os.Exit(1)
		}
	}

	// QEMU requires RW access to query SEV capabilities
	if helpers.FileExists("/dev/sev") {
		if err := helpers.SetPermissionsRW("/dev/sev"); err != nil {
			logger.Error("Failed to set permissions for /dev/sev", "error", err)
			os.Exit(1)
		}
	}

	slog.Info("Start virtqemud daemon")
	if err := helpers.StartVirtqemud(); err != nil {
		logger.Error("Failed to start virtqemud", "error", err)
		os.Exit(1)
	}

	// Connect to libvirt
	conn, err := libvirt.NewConnect("qemu:///system")
	if err != nil {
		logger.Error("Failed to connect to libvirt", "error", err)
		return
	}
	defer conn.Close()

	// outDir := "/var/lib/kubevirt-node-labeller"
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		logger.Error("Failed to create output directory", "path", outDir, "error", err)
		os.Exit(1)
	}

	slog.Info("Execute virsh domcapabilities")
	// Get domain capabilities
	domCaps, err := conn.GetDomainCapabilities("", arch, "", virtType, 0)
	if err != nil {
		logger.Error("Failed to retrieve domain capabilities", "error", err)
		return
	}

	// Save domcapabilities.xml
	domCapsPath := fmt.Sprintf("%s/virsh_domcapabilities.xml", outDir)
	if err := os.WriteFile(domCapsPath, []byte(domCaps), 0o644); err != nil {
		logger.Error("Failed to write domain capabilities", "error", err)
		os.Exit(1)
	}

	// hypervisor-cpu-baseline only for x86_64
	if arch == "x86_64" {
		featuresXML, err := conn.GetCapabilities()
		if err != nil {
			logger.Error("Failed to retrieve supported CPU features", "error", err)
			os.Exit(1)
		} else {
			supportedFeaturesPath := fmt.Sprintf("%s/supported_features.xml", outDir)
			if err := os.WriteFile(supportedFeaturesPath, []byte(featuresXML), 0o644); err != nil {
				logger.Error("Failed to write supported features", "error", err)
				os.Exit(1)
			}
		}
	}

	// Get host capabilities
	hostCaps, err := conn.GetCapabilities()
	if err != nil {
		logger.Error("Failed to retrieve host capabilities", "error", err)
		os.Exit(1)
	}

	// Save capabilities.xml
	capabilitiesPath := fmt.Sprintf("%s/capabilities.xml", outDir)
	if err := os.WriteFile(capabilitiesPath, []byte(hostCaps), 0o644); err != nil {
		logger.Error("Failed to write capabilities", "error", err)
		os.Exit(1)
	}

	slog.Info(fmt.Sprintf("Virsh capabilities saved to %s", capabilitiesPath))
}
