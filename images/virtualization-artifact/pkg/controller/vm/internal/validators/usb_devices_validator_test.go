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

package validators

import (
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/deckhouse/virtualization-controller/pkg/controller/indexer"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
)

func TestUSBDevicesValidatorValidateUpdate(t *testing.T) {
	tests := []struct {
		name      string
		oldUSB    []v1alpha2.USBDeviceSpecRef
		newUSB    []v1alpha2.USBDeviceSpecRef
		existing  []client.Object
		wantError bool
	}{
		{
			name:      "should skip conflict check for unchanged usb devices",
			oldUSB:    []v1alpha2.USBDeviceSpecRef{{Name: "usb-legacy"}},
			newUSB:    []v1alpha2.USBDeviceSpecRef{{Name: "usb-legacy"}, {Name: "usb-new"}},
			existing:  []client.Object{newVirtualMachine("vm-other", []v1alpha2.USBDeviceSpecRef{{Name: "usb-legacy"}})},
			wantError: false,
		},
		{
			name:      "should fail when new usb device is already used by another vm",
			oldUSB:    []v1alpha2.USBDeviceSpecRef{{Name: "usb-legacy"}},
			newUSB:    []v1alpha2.USBDeviceSpecRef{{Name: "usb-legacy"}, {Name: "usb-new"}},
			existing:  []client.Object{newVirtualMachine("vm-other", []v1alpha2.USBDeviceSpecRef{{Name: "usb-new"}})},
			wantError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			oldVM := newVirtualMachine("vm-current", tt.oldUSB)
			newVM := newVirtualMachine("vm-current", tt.newUSB)

			objects := []client.Object{oldVM}
			objects = append(objects, tt.existing...)

			validator := NewUSBDevicesValidator(newFakeClientWithUSBVMIndexer(t, objects...))
			_, err := validator.ValidateUpdate(t.Context(), oldVM, newVM)

			if tt.wantError && err == nil {
				t.Fatalf("expected error, got nil")
			}

			if !tt.wantError && err != nil {
				t.Fatalf("expected no error, got %v", err)
			}
		})
	}
}

func newVirtualMachine(name string, usb []v1alpha2.USBDeviceSpecRef) *v1alpha2.VirtualMachine {
	return &v1alpha2.VirtualMachine{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: "default"},
		Spec:       v1alpha2.VirtualMachineSpec{USBDevices: usb},
	}
}

func newFakeClientWithUSBVMIndexer(t *testing.T, objects ...client.Object) client.Client {
	t.Helper()

	scheme := runtime.NewScheme()
	if err := v1alpha2.AddToScheme(scheme); err != nil {
		t.Fatalf("failed to add virtualization API scheme: %v", err)
	}

	vmObj, vmField, vmExtractValue := indexer.IndexVMByUSBDevice()
	return fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(objects...).
		WithIndex(vmObj, vmField, vmExtractValue).
		Build()
}
