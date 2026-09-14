/*
Copyright 2026 Flant JSC

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

package cdi

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	drapbv1 "k8s.io/kubelet/pkg/apis/dra/v1beta1"
)

const testClaimUID = "3f2b1c00-0000-0000-0000-000000000001"

func newTestManager(t *testing.T) Manager {
	t.Helper()

	m, err := NewManager(t.TempDir(), "usb", "dra.virtualization.deckhouse.io", "node-1", "DRA_USB")
	if err != nil {
		t.Fatalf("new manager: %v", err)
	}
	return m
}

func claimSpecDevices(deviceName string) PreparedDevices {
	return PreparedDevices{{Device: drapbv1.Device{DeviceName: deviceName}}}
}

func TestCreateClaimSpecFileRejectsDeviceNameWithoutPrefix(t *testing.T) {
	m := newTestManager(t)

	// "usb" is three bytes: shorter than the prefix, which used to be stripped by index.
	for _, deviceName := range []string{"", "u", "us", "usb", "usbx", "device-1"} {
		err := m.CreateClaimSpecFile(testClaimUID, claimSpecDevices(deviceName))
		if err == nil {
			t.Errorf("device name %q: expected an error", deviceName)
			continue
		}
		if !strings.Contains(err.Error(), USBDeviceNamePrefix) {
			t.Errorf("device name %q: error does not mention the expected prefix: %v", deviceName, err)
		}
	}
}

func TestCreateClaimSpecFileStripsPrefixFromEnv(t *testing.T) {
	specDir := t.TempDir()
	m, err := NewManager(specDir, "usb", "dra.virtualization.deckhouse.io", "node-1", "DRA_USB")
	if err != nil {
		t.Fatalf("new manager: %v", err)
	}

	if err := m.CreateClaimSpecFile(testClaimUID, claimSpecDevices("usb-c0ffee")); err != nil {
		t.Fatalf("create claim spec file: %v", err)
	}

	specs, err := filepath.Glob(filepath.Join(specDir, "*"))
	if err != nil || len(specs) != 1 {
		t.Fatalf("expected exactly one spec file, got %v (err %v)", specs, err)
	}
	content, err := os.ReadFile(specs[0])
	if err != nil {
		t.Fatalf("read spec file: %v", err)
	}
	want := "DRA_USB_c0ffee_RESOURCE_CLAIM=" + testClaimUID
	if !strings.Contains(string(content), want) {
		t.Errorf("spec file does not contain %q:\n%s", want, content)
	}
}
