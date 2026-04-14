//go:build !linux

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
	"fmt"
	"log/slog"
	"runtime"
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
	return ""
}

func CreateCharDevice(path string, major, minor int) error {
	_ = path
	_ = major
	_ = minor
	return fmt.Errorf("character device management is supported only on linux")
}

func CreateKVMDevice(minor string) error {
	_ = minor
	return fmt.Errorf("kvm device management is supported only on linux")
}

func SetPermissionsRW(path string) error {
	_ = path
	return fmt.Errorf("device permission management is supported only on linux")
}

func StartVirtqemud() error {
	return fmt.Errorf("virtqemud startup is supported only on linux")
}

func RunCommand(cmd string, args []string) (string, error) {
	_ = cmd
	_ = args
	return "", fmt.Errorf("command execution is supported only on linux in this helper")
}

func RunCommandWithError(cmd string, args []string) error {
	_, err := RunCommand(cmd, args)
	return err
}

func GetCPUFeatureDomCaps(domCapsXML string, logger *slog.Logger) ([]string, error) {
	_ = domCapsXML
	_ = logger
	return nil, fmt.Errorf("libvirt capability parsing is supported only on linux in this helper")
}
