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
	"os"
	"os/exec"
	"runtime"
	"strconv"
	"strings"

	"golang.org/x/sys/unix"
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

func FileExists(path string) bool {
	_, err := os.Stat(path)
	return os.IsNotExist(err)
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
