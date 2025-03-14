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
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	slog.SetDefault(logger)

	arch := helpers.GetArch()
	logger.Info(fmt.Sprintf("Get arch %s", arch))
	machine := helpers.GetMachineType(arch)
	logger.Info(fmt.Sprintf("Machine type %s for arch %s", machine, arch))
	if machine == "" {
		logger.Info("Unsupported architecture, exit gracefully")
		return
	}

	kvmMinor := helpers.GetKVMMinor()

	if _, err := os.Stat("/dev/kvm"); err != nil {
		if os.IsNotExist(err) {
			if err := helpers.CreateKVMDevice(kvmMinor); err != nil {
				logger.Error("Failed to create /dev/kvm device", slog.String("error", err.Error()))
				os.Exit(1)
			}
		} else {
			logger.Error("Failed to check /dev/kvm device", slog.String("error", err.Error()))
			os.Exit(1)
		}
	}

	if err := helpers.SetPermissionsRW("/dev/kvm"); err != nil {
		logger.Error("Failed to set permissions for /dev/kvm", slog.String("error", err.Error()))
		os.Exit(1)
	}

	// QEMU requires RW access to query SEV capabilities
	// AMD's Secure Encrypted Virtualization (SEV)
	if _, err := os.Stat("/dev/sev"); err != nil {
		if !os.IsNotExist(err) {
			if err := helpers.SetPermissionsRW("/dev/sev"); err != nil {
				logger.Error("Failed to set permissions for /dev/sev", slog.String("error", err.Error()))
				os.Exit(1)
			}
		}
	}

	logger.Info("Start virtqemud as daemon")
	if err := helpers.StartVirtqemud(); err != nil {
		logger.Error("Failed to start virtqemud", slog.String("error", err.Error()))
		os.Exit(1)
	}

	logger.Info("Connect to libvirt via qemu:///system")
	conn, err := libvirt.NewConnect("qemu:///system")
	if err != nil {
		logger.Error("Failed to connect to libvirt", slog.String("error", err.Error()))
		return
	}
	defer conn.Close()

	if err := os.MkdirAll(outDir, 0o755); err != nil {
		logger.Error("Failed to create output directory", "path", outDir, slog.String("error", err.Error()))
		os.Exit(1)
	}

	logger.Info("Get domain capabilities")
	domCaps, err := conn.GetDomainCapabilities("", arch, machine, virtType, 0)
	if err != nil {
		logger.Error("Failed to retrieve domain capabilities", slog.String("error", err.Error()))
		return
	}

	// Save domcapabilities.xml
	domCapsPath := fmt.Sprintf("%s/virsh_domcapabilities.xml", outDir)
	if err := os.WriteFile(domCapsPath, []byte(domCaps), 0o644); err != nil {
		logger.Error("Failed to write domain capabilities", slog.String("error", err.Error()))
		os.Exit(1)
	}
	logger.Info(fmt.Sprintf("Domcapabilities saved to %s", domCapsPath))

	// hypervisor-cpu-baseline only for x86_64
	if arch == "x86_64" {
		// featuresXML, err := conn.GetCapabilities()
		cpuXML, err := conn.BaselineCPU([]string{domCapsPath}, 1)
		if err != nil {
			logger.Error("Failed to retrieve supported CPU", slog.String("error", err.Error()))
			os.Exit(1)
		}

		supportedCPUPath := fmt.Sprintf("%s/supported_cpu.xml", outDir)
		if err := os.WriteFile(supportedCPUPath, []byte(cpuXML), 0o644); err != nil {
			logger.Error("Failed to write supported cpu", slog.String("error", err.Error()))
			os.Exit(1)
		}

		featuresXML, err := conn.BaselineHypervisorCPU("", arch, machine, virtType, []string{domCapsPath}, 0)
		if err != nil {
			logger.Error("Failed to retrieve supported CPU features", slog.String("error", err.Error()))
			os.Exit(1)
		} else {
			supportedFeaturesPath := fmt.Sprintf("%s/supported_features.xml", outDir)
			if err := os.WriteFile(supportedFeaturesPath, []byte(featuresXML), 0o644); err != nil {
				logger.Error("Failed to write supported features", slog.String("error", err.Error()))
				os.Exit(1)
			}
			logger.Info(fmt.Sprintf("Hypervisor features saved to %s", supportedFeaturesPath))
		}
	}

	// Get host capabilities
	hostCaps, err := conn.GetCapabilities()
	if err != nil {
		logger.Error("Failed to retrieve host capabilities", slog.String("error", err.Error()))
		os.Exit(1)
	}

	// Save capabilities.xml
	capabilitiesPath := fmt.Sprintf("%s/capabilities.xml", outDir)
	if err := os.WriteFile(capabilitiesPath, []byte(hostCaps), 0o644); err != nil {
		logger.Error("Failed to write capabilities", "error", err)
		os.Exit(1)
	}

	logger.Info(fmt.Sprintf("Host capabilities saved to %s", capabilitiesPath))
}
