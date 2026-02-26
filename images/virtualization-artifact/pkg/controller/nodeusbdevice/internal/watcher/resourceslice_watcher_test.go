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

package watcher

import (
	"testing"

	resourcev1 "k8s.io/api/resource/v1"
	"k8s.io/utils/ptr"
)

func TestNodeUSBDeviceNamesFromSlice(t *testing.T) {
	resourceSlice := &resourcev1.ResourceSlice{
		Spec: resourcev1.ResourceSliceSpec{
			Devices: []resourcev1.Device{
				{Name: "gpu-1"},
				{Name: "usb-1"},
				{
					Name: "usb-2",
					Attributes: map[resourcev1.QualifiedName]resourcev1.DeviceAttribute{
						"name": {StringValue: ptr.To("USB.Device.2")},
					},
				},
				{
					Name: "usb-duplicate",
					Attributes: map[resourcev1.QualifiedName]resourcev1.DeviceAttribute{
						"name": {StringValue: ptr.To("USB.Device.2")},
					},
				},
			},
		},
	}

	actual := nodeUSBDeviceNamesFromSlice(resourceSlice)
	if len(actual) != 2 {
		t.Fatalf("expected 2 device names, got %d", len(actual))
	}

	if actual[0] != "usb-1" {
		t.Fatalf("unexpected first name %q", actual[0])
	}

	if actual[1] != "usb-device-2" {
		t.Fatalf("unexpected second name %q", actual[1])
	}
}

func TestNodeUSBDeviceName(t *testing.T) {
	name, ok := nodeUSBDeviceName(resourcev1.Device{Name: "gpu-1"})
	if ok {
		t.Fatalf("expected non-usb device to be skipped, got %q", name)
	}

	name, ok = nodeUSBDeviceName(resourcev1.Device{Name: "usb-raw"})
	if !ok || name != "usb-raw" {
		t.Fatalf("expected usb raw name, got %q (%v)", name, ok)
	}
}
