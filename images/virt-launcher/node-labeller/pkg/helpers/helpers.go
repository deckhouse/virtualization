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

package helpers

import (
	"encoding/xml"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"runtime"
	"strconv"
	"strings"

	"golang.org/x/sys/unix"
	"libvirt.org/go/libvirtxml"
)

const (
	kvmCharDeviceMajorVersion = 10
)

func GetArch() string {
	switch runtime.GOARCH {
	case "arm64":
		return "aarch64"
	case "amd64":
		return "x86_64"
	case "s390x":
		return "s390x"
	default:
		return ""
	}
}

func GetMachineType(arch string) string {
	switch arch {
	case "aarch64":
		return "virt"
	case "s390x":
		return "s390-ccw-virtio"
	case "x86_64":
		return "q35"
	default:
		return ""
	}
}

func GetKVMMinor() string {
	data, err := os.ReadFile("/proc/misc")
	if err != nil {
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

func CreateCharDevice(path string, major, minor int) error {
	// Permissions: 0666 (-rw-rw-rw-)
	mode := uint32(unix.S_IFCHR | 0o666)

	// Create the device number
	dev := unix.Mkdev(uint32(major), uint32(minor))

	// Create the special device file
	err := unix.Mknod(path, mode, int(dev))
	if err != nil {
		return fmt.Errorf("failed to create char device %s: %v", path, err)
	}

	return nil
}

func CreateKVMDevice(minor string) error {
	m, err := strconv.Atoi(minor)
	if err != nil {
		return fmt.Errorf("unexpected value, expect int, take %v", minor)
	}
	return CreateCharDevice("/dev/kvm", kvmCharDeviceMajorVersion, m)
}

func SetPermissionsRW(path string) error {
	mode := os.FileMode(0o666) // `chmod o+rw`
	return os.Chmod(path, mode)
}

func StartVirtqemud() error {
	return RunCommandWithError("virtqemud", []string{"-d"})
}

func RunCommand(cmd string, args []string) (string, error) {
	out, err := exec.Command(cmd, args...).CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("%s failed: %w - output: %s", cmd, err, string(out))
	}
	return string(out), nil
}

func RunCommandWithError(cmd string, args []string) error {
	_, err := RunCommand(cmd, args)
	return err
}

func findHostVendor(modes []libvirtxml.DomainCapsCPUMode) string {
	for _, mode := range modes {
		if mode.Name == "host-model" && mode.Supported == "yes" {
			return mode.Vendor
		}
	}
	return ""
}

// Check if mode is supported and matches host vendor
func isModeSupported(mode libvirtxml.DomainCapsCPUMode, hostVendor string) bool {
	return mode.Supported == "yes" && mode.Vendor == hostVendor
}

type cpuFeature struct {
	Policy string `xml:"policy,attr"`
	Name   string `xml:"name,attr"`
}

type cpuModel struct {
	Text     string `xml:",chardata"`
	Fallback string `xml:"fallback,attr,omitempty"`
}

type customCPU struct {
	XMLName  xml.Name     `xml:"cpu"`
	Mode     string       `xml:"mode,attr"`
	Model    cpuModel     `xml:"model"`
	Vendor   string       `xml:"vendor,omitempty"`
	Features []cpuFeature `xml:"feature"`
}

// Construct base CPU configuration
func buildCustomCPU(mode libvirtxml.DomainCapsCPUMode,
	model libvirtxml.DomainCapsCPUModel,
	hostVendor string,
) customCPU {
	return customCPU{
		Mode: mode.Name,
		Model: cpuModel{
			Text:     model.Name,
			Fallback: model.Fallback,
		},
		Vendor: hostVendor,
	}
}

// Add features from mode to CPU configuration
func addModeFeatures(cpu *customCPU, features []libvirtxml.DomainCapsCPUFeature) {
	for _, feature := range features {
		cpu.Features = append(cpu.Features, cpuFeature{
			Policy: feature.Policy,
			Name:   feature.Name,
		})
	}
}

// GetCPUFeatureDomCaps extracts CPU feature domain capabilities from libvirt XML data.
// It returns XML snippets representing compatible CPU configurations.
func GetCPUFeatureDomCaps(domCapsXML string, logger *slog.Logger) ([]string, error) {
	const indent = "  "
	var (
		cpuXMLs    []string
		domainCaps libvirtxml.DomainCaps
		hostVendor string
	)

	if err := domainCaps.Unmarshal(domCapsXML); err != nil {
		return nil, fmt.Errorf("failed to parse domain capabilities: %w", err)
	}

	hostVendor = findHostVendor(domainCaps.CPU.Modes)

	for _, mode := range domainCaps.CPU.Modes {
		if !isModeSupported(mode, hostVendor) {
			continue
		}

		for _, model := range mode.Models {
			cpu := buildCustomCPU(mode, model, hostVendor)
			addModeFeatures(&cpu, mode.Features)

			xmlData, err := xml.MarshalIndent(cpu, "", indent)
			if err != nil {
				logger.Warn("Failed to marshal CPU configuration", "error", err)
				continue // Skip invalid configurations [[7]]
			}

			cpuXMLs = append(cpuXMLs, string(xmlData))
		}
	}

	return cpuXMLs, nil
}
