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
		return "" // Unsupported architecture, exit gracefully
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
		slog.Error(fmt.Sprintf("unexpected value, expect int, take %v", minor))
		return err
	}
	return CreateCharDevice("/dev/kvm", kvmCharDeviceMajorVersion, m)
}

func SetPermissionsRW(path string) error {
	mode := os.FileMode(0o666) // Equivalent to `chmod o+rw`
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

func GetCPUFeatureDomCaps(domCapsXML string, logger *slog.Logger) ([]string, error) {
	type CustomCPU struct {
		XMLName xml.Name `xml:"cpu"`
		Mode    string   `xml:"mode,attr"`
		Model   struct {
			Text     string `xml:",chardata"`
			Fallback string `xml:"fallback,attr,omitempty"`
		} `xml:"model"`
		Vendor   string `xml:"vendor,omitempty"`
		Features []struct {
			Policy string `xml:"policy,attr"`
			Name   string `xml:"name,attr"`
		} `xml:"feature"`
	}

	// type CustomCPU struct {
	// 	XMLName xml.Name `xml:"cpu"`
	// 	libvirtxml.DomainCapsCPUMode
	// }

	var xmlCPUs []string
	var domainCaps libvirtxml.DomainCaps
	var modeVendor string

	// var tst CapsCPU

	if err := domainCaps.Unmarshal(domCapsXML); err != nil {
		return nil, err
	}

	for _, mode := range domainCaps.CPU.Modes {
		if len(mode.Models) == 0 {
			continue
		}

		for _, model := range mode.Models {
			// for _, model := range mode.Models {
			// Skip models with mismatched vendors
			if mode.Vendor == "" {
				modeVendor = "unknown"
			} else {
				modeVendor = mode.Vendor
			}

			customCPU := CustomCPU{
				Mode: mode.Name,
				Model: struct {
					Text     string `xml:",chardata"`
					Fallback string `xml:"fallback,attr,omitempty"`
				}{
					Text:     model.Name,
					Fallback: model.Fallback,
				},
				Vendor: modeVendor,
			}

			// Add features from the mode
			for _, feature := range mode.Features {
				customCPU.Features = append(customCPU.Features, struct {
					Policy string `xml:"policy,attr"`
					Name   string `xml:"name,attr"`
				}{
					Policy: feature.Policy,
					Name:   feature.Name,
				})
			}

			xmlData, err := xml.MarshalIndent(customCPU, "", "  ")
			if err != nil {
				return nil, err
			}

			xmlCPUs = append(xmlCPUs, string(xmlData))
		}
	}

	// xmlCPUFeatures, err := xml.Marshal(domainCaps.CPU)
	// xmlCPUFeatures, err := xml.Marshal(customCPU)
	// if err != nil {
	// 	return "", err
	// }

	printDbg := fmt.Sprintf("XML:\n%s\n", strings.Join(xmlCPUs, "\n"))
	fmt.Print(printDbg)

	// return string(xmlCPUFeatures), nil
	return xmlCPUs, nil
}

func GetCPUFeatureDomCaps2(domCapsXML string, logger *slog.Logger) ([]string, error) {
	var domainCaps libvirtxml.DomainCaps
	var some []string

	type CustomCPU2 struct {
		XMLName xml.Name `xml:"cpu"`
		// libvirtxml.DomainCapsCPUMode
		Mode libvirtxml.DomainCapsCPUMode `xml:"mode"`
	}

	// var Ccp CustomCPU2

	if err := domainCaps.Unmarshal(domCapsXML); err != nil {
		logger.Error("Failed unmarshar domCaps")
		return nil, err
	}

	for _, mode := range domainCaps.CPU.Modes {

		ccpa := CustomCPU2{
			Mode: mode,
		}

		fmt.Println(ccpa.Mode.Name)

		s, e := xml.Marshal(ccpa)
		// s, e := xml.Marshal(mode)
		if e != nil {
			logger.Error("Failed marshar domCaps")
			return nil, e
		}
		fmt.Println("--debug--", string(s), "--", "")
		some = append(some, string(s))
	}

	fmt.Println("=====", some, "=====")
	return some, nil
}
