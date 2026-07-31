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

package usbip

import "testing"

func TestVhciHcdStatusPath(t *testing.T) {
	want := map[int]string{
		0: "/sys/devices/platform/vhci_hcd.0/status",
		1: "/sys/devices/platform/vhci_hcd.0/status.1",
		7: "/sys/devices/platform/vhci_hcd.0/status.7",
	}

	for controller, expected := range want {
		if got := vhciHcdStatusPath(controller); got != expected {
			t.Errorf("vhciHcdStatusPath(%d) = %q, want %q", controller, got, expected)
		}
	}
}
