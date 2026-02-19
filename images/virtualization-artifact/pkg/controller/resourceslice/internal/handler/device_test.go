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

package handler

import (
	"testing"

	resourcev1 "k8s.io/api/resource/v1"
)

func TestIsUSBDevice(t *testing.T) {
	tests := []struct {
		name       string
		deviceName string
		expectUSB  bool
	}{
		{name: "usb device", deviceName: "usb-device-1", expectUSB: true},
		{name: "non usb device", deviceName: "gpu-device-1", expectUSB: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			device := resourcev1.Device{Name: tt.deviceName}
			if IsUSBDevice(device) != tt.expectUSB {
				t.Fatalf("expected %v for device %q", tt.expectUSB, tt.deviceName)
			}
		})
	}
}

func ptrString(v string) *string { return &v }
