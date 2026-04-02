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
	"strings"
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/component-base/featuregate"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/deckhouse/virtualization-controller/pkg/controller/indexer"
	"github.com/deckhouse/virtualization-controller/pkg/featuregates"
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

			validator := NewUSBDevicesValidator(newFakeClientWithUSBVMIndexer(t, objects...), newUSBFeatureGate(t, true))
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

func TestUSBDevicesValidatorValidateCreateReturnsErrorWhenUSBFeatureDisabled(t *testing.T) {
	vm := newVirtualMachine("vm-current", []v1alpha2.USBDeviceSpecRef{{Name: "usb-1"}})

	validator := NewUSBDevicesValidator(newFakeClientWithUSBVMIndexer(t), newUSBFeatureGate(t, false))
	_, err := validator.ValidateCreate(t.Context(), vm)
	if err == nil {
		t.Fatal("expected error, got nil")
	}

	if !strings.Contains(err.Error(), "Kubernetes version 1.34 or newer") {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(err.Error(), "DRA feature gates") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestUSBDevicesValidatorValidateCreateSucceedsWhenUSBFeatureEnabled(t *testing.T) {
	vm := newVirtualMachine("vm-current", []v1alpha2.USBDeviceSpecRef{{Name: "usb-1"}})

	validator := NewUSBDevicesValidator(newFakeClientWithUSBVMIndexer(t), newUSBFeatureGate(t, true))
	_, err := validator.ValidateCreate(t.Context(), vm)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
}

func TestUSBDevicesValidatorValidateUpdateExcludesLocalUSBsFromPortAccounting(t *testing.T) {
	oldVM := newVirtualMachine("vm-current", []v1alpha2.USBDeviceSpecRef{{Name: "usb-local"}})
	oldVM.Status.Node = "node-1"
	oldVM.Status.USBDevices = []v1alpha2.USBDeviceStatusRef{{Name: "usb-local", Attached: true}}

	newVM := oldVM.DeepCopy()
	newVM.Spec.USBDevices = []v1alpha2.USBDeviceSpecRef{{Name: "usb-local"}, {Name: "usb-remote"}}

	objects := []client.Object{
		oldVM,
		&corev1.Node{ObjectMeta: metav1.ObjectMeta{Name: "node-1", Annotations: map[string]string{
			"usb.virtualization.deckhouse.io/usbip-total-ports":                "2",
			"usb.virtualization.deckhouse.io/usbip-high-speed-hub-used-ports":  "1",
			"usb.virtualization.deckhouse.io/usbip-super-speed-hub-used-ports": "0",
		}}},
		&v1alpha2.USBDevice{ObjectMeta: metav1.ObjectMeta{Name: "usb-local", Namespace: "default"}, Status: v1alpha2.USBDeviceStatus{NodeName: "node-1", Attributes: v1alpha2.NodeUSBDeviceAttributes{Speed: 480}}},
		&v1alpha2.USBDevice{ObjectMeta: metav1.ObjectMeta{Name: "usb-remote", Namespace: "default"}, Status: v1alpha2.USBDeviceStatus{NodeName: "node-2", Attributes: v1alpha2.NodeUSBDeviceAttributes{Speed: 480}}},
	}

	validator := NewUSBDevicesValidator(newFakeClientWithUSBVMIndexer(t, objects...), newUSBFeatureGate(t, true))
	_, err := validator.ValidateUpdate(t.Context(), oldVM, newVM)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
}

func TestUSBDevicesValidatorValidateUpdateReturnsErrorWhenUSBFeatureDisabled(t *testing.T) {
	oldVM := newVirtualMachine("vm-current", nil)
	newVM := newVirtualMachine("vm-current", []v1alpha2.USBDeviceSpecRef{{Name: "usb-1"}})

	validator := NewUSBDevicesValidator(newFakeClientWithUSBVMIndexer(t, oldVM), newUSBFeatureGate(t, false))
	_, err := validator.ValidateUpdate(t.Context(), oldVM, newVM)
	if err == nil {
		t.Fatal("expected error, got nil")
	}

	if !strings.Contains(err.Error(), "Kubernetes version 1.34 or newer") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func newFakeClientWithUSBVMIndexer(t *testing.T, objects ...client.Object) client.Client {
	t.Helper()

	scheme := runtime.NewScheme()
	if err := v1alpha2.AddToScheme(scheme); err != nil {
		t.Fatalf("failed to add virtualization API scheme: %v", err)
	}
	if err := corev1.AddToScheme(scheme); err != nil {
		t.Fatalf("failed to add core API scheme: %v", err)
	}

	vmObj, vmField, vmExtractValue := indexer.IndexVMByUSBDevice()
	vmNodeObj, vmNodeField, vmNodeExtractValue := indexer.IndexVMByNode()
	return fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(objects...).
		WithIndex(vmObj, vmField, vmExtractValue).
		WithIndex(vmNodeObj, vmNodeField, vmNodeExtractValue).
		Build()
}

func newUSBFeatureGate(t *testing.T, enabled bool) featuregate.FeatureGate {
	t.Helper()

	gate, setFromMap, err := featuregates.NewUnlocked()
	if err != nil {
		t.Fatalf("failed to create feature gate: %v", err)
	}

	if err = setFromMap(map[string]bool{string(featuregates.USB): enabled}); err != nil {
		t.Fatalf("failed to set USB feature gate: %v", err)
	}

	return gate
}
