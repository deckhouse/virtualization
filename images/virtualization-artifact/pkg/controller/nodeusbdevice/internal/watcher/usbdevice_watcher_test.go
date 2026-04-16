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

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/deckhouse/virtualization/api/core/v1alpha2"
)

func TestRequestsByUSBDevice(t *testing.T) {
	usbDevice := &v1alpha2.USBDevice{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "usb-device-1",
			Namespace: "test-ns",
			OwnerReferences: []metav1.OwnerReference{{
				APIVersion: v1alpha2.SchemeGroupVersion.String(),
				Kind:       v1alpha2.NodeUSBDeviceKind,
				Name:       "node-usb-device-1",
			}},
		},
	}

	requests := requestsByUSBDevice(usbDevice)
	if len(requests) != 1 {
		t.Fatalf("expected 1 request, got %d", len(requests))
	}
	if requests[0].Name != "node-usb-device-1" {
		t.Fatalf("unexpected request name %q", requests[0].Name)
	}
	if requests[0].Namespace != "" {
		t.Fatalf("expected cluster-scoped request, got namespace %q", requests[0].Namespace)
	}
}

func TestRequestsByUSBDeviceFallsBackToName(t *testing.T) {
	usbDevice := &v1alpha2.USBDevice{ObjectMeta: metav1.ObjectMeta{Name: "usb-device-1", Namespace: "test-ns"}}

	requests := requestsByUSBDevice(usbDevice)
	if len(requests) != 1 {
		t.Fatalf("expected 1 request, got %d", len(requests))
	}
	if requests[0].Name != "usb-device-1" {
		t.Fatalf("unexpected fallback request name %q", requests[0].Name)
	}
}

func TestShouldProcessUSBDeviceUpdate(t *testing.T) {
	oldObj := &v1alpha2.USBDevice{
		ObjectMeta: metav1.ObjectMeta{Name: "usb-device-1"},
		Status: v1alpha2.USBDeviceStatus{Conditions: []metav1.Condition{{
			Type:   "Attached",
			Status: metav1.ConditionFalse,
			Reason: "Available",
		}}},
	}

	sameObj := oldObj.DeepCopy()
	if shouldProcessUSBDeviceUpdate(oldObj, sameObj) {
		t.Fatal("expected unchanged object update to be ignored")
	}

	changedConditions := oldObj.DeepCopy()
	changedConditions.Status.Conditions[0].Status = metav1.ConditionTrue
	if !shouldProcessUSBDeviceUpdate(oldObj, changedConditions) {
		t.Fatal("expected conditions update to be processed")
	}

	changedOwners := oldObj.DeepCopy()
	changedOwners.OwnerReferences = []metav1.OwnerReference{{Name: "node-usb-device-1"}}
	if !shouldProcessUSBDeviceUpdate(oldObj, changedOwners) {
		t.Fatal("expected owner references update to be processed")
	}

	if shouldProcessUSBDeviceUpdate(nil, changedConditions) {
		t.Fatal("expected nil old object to be ignored")
	}
	if shouldProcessUSBDeviceUpdate(oldObj, nil) {
		t.Fatal("expected nil new object to be ignored")
	}
}
