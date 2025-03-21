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
	outDir = "/var/lib/kubevirt-node-labeller"
)

func run(logger *slog.Logger) error {
	// Define virt type qemu for get domain capabilites if no device kvm
	virtType := "qemu"

	arch := helpers.GetArch()
	logger.Info(fmt.Sprintf("Get arch %s", arch))
	machine := helpers.GetMachineType(arch)
	logger.Info(fmt.Sprintf("Machine type %s for arch %s", machine, arch))
	if machine == "" {
		return fmt.Errorf("unsupported architecture, exit")
	}

	kvmMinor := helpers.GetKVMMinor()

	logger.Info("Check and create if not exists /dev/kvm")
	if _, err := os.Stat("/dev/kvm"); err != nil {
		if os.IsNotExist(err) {
			if err := helpers.CreateKVMDevice(kvmMinor); err != nil {
				return fmt.Errorf("failed to create /dev/kvm device, error %w", err)
			}
		} else {
			return fmt.Errorf("failed to check /dev/kvm device error %w", err)
		}
	} else {
		virtType = "kvm"
	}

	logger.Info("Set permissions on /dev/kvm")
	if err := helpers.SetPermissionsRW("/dev/kvm"); err != nil {
		return fmt.Errorf("failed to set permissions for /dev/kvm error %w", err)
	}

	// QEMU requires RW access to query SEV capabilities
	// AMD's Secure Encrypted Virtualization (SEV)
	if _, err := os.Stat("/dev/sev"); err != nil {
		if !os.IsNotExist(err) {
			if err := helpers.SetPermissionsRW("/dev/sev"); err != nil {
				return fmt.Errorf("failed to set permissions for /dev/sev error %w", err)
			}
		}
	}

	logger.Info("Start virtqemud as daemon")
	if err := helpers.StartVirtqemud(); err != nil {
		return fmt.Errorf("failed to start virtqemud error %w", err)
	}

	logger.Info("Connect to libvirt via qemu:///system")
	conn, err := libvirt.NewConnect("qemu:///system")
	if err != nil {
		return fmt.Errorf("failed to connect to libvirt error %w", err)
	}
	defer conn.Close()

	if err := os.MkdirAll(outDir, 0o755); err != nil {
		return fmt.Errorf("failed to create output directory %s, error %w", outDir, err)
	}

	logger.Info("Get domain capabilities")
	domCapsXML, err := conn.GetDomainCapabilities("", arch, machine, virtType, 0)
	if err != nil {
		return fmt.Errorf("failed to retrieve domain capabilities error %w", err)
	}

	// Save domcapabilities.xml
	domCapsPath := fmt.Sprintf("%s/virsh_domcapabilities.xml", outDir)
	if err := os.WriteFile(domCapsPath, []byte(domCapsXML), 0o644); err != nil {
		return fmt.Errorf("failed to write domain capabilities error %w", err)
	}
	logger.Info(fmt.Sprintf("Domcapabilities saved to %s", domCapsPath))

	cpuXML, err := helpers.GetCPUFeatureDomCaps(domCapsXML, logger)
	if err != nil {
		return fmt.Errorf("failed to retrieve dom caps error %w", err)
	}

	// hypervisor-cpu-baseline only for x86_64
	if arch == "x86_64" {

		featuresXML, err := conn.BaselineHypervisorCPU("", arch, machine, virtType, cpuXML, libvirt.CONNECT_BASELINE_CPU_EXPAND_FEATURES)
		if err != nil {
			return fmt.Errorf("failed to retrieve supported CPU features error %w", err)
		}

		supportedFeaturesPath := fmt.Sprintf("%s/supported_features.xml", outDir)
		if err := os.WriteFile(supportedFeaturesPath, []byte(featuresXML), 0o644); err != nil {
			return fmt.Errorf("failed to write supported features error %w", err)
		}
		logger.Info(fmt.Sprintf("Hypervisor features saved to %s", supportedFeaturesPath))

	}

	// Get host capabilities
	hostCaps, err := conn.GetCapabilities()
	if err != nil {
		return fmt.Errorf("failed to retrieve host capabilities error %w", err)
	}

	// Save capabilities.xml
	capabilitiesPath := fmt.Sprintf("%s/capabilities.xml", outDir)
	if err := os.WriteFile(capabilitiesPath, []byte(hostCaps), 0o644); err != nil {
		return fmt.Errorf("failed to write capabilities, error %w", err)
	}

	logger.Info(fmt.Sprintf("Host capabilities saved to %s", capabilitiesPath))

	return nil
}
