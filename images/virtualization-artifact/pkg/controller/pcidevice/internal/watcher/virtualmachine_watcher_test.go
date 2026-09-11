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

func TestShouldProcessVirtualMachineUpdate(t *testing.T) {
	oldObj := &v1alpha2.VirtualMachine{
		Spec: v1alpha2.VirtualMachineSpec{
			PCIDevices: []v1alpha2.PCIDeviceSpecRef{{Name: "pci-a"}},
		},
		Status: v1alpha2.VirtualMachineStatus{
			PCIDevices: []v1alpha2.PCIDeviceStatusRef{{Name: "pci-a", Attached: false}},
		},
	}

	sameObj := oldObj.DeepCopy()
	if shouldProcessVirtualMachineUpdate(oldObj, sameObj) {
		t.Fatal("expected unchanged VM update to be ignored")
	}

	changedSpec := oldObj.DeepCopy()
	changedSpec.Spec.PCIDevices = []v1alpha2.PCIDeviceSpecRef{{Name: "pci-b"}}
	if !shouldProcessVirtualMachineUpdate(oldObj, changedSpec) {
		t.Fatal("expected VM spec PCI devices update to be processed")
	}

	changedStatus := oldObj.DeepCopy()
	changedStatus.Status.PCIDevices[0].Attached = true
	if !shouldProcessVirtualMachineUpdate(oldObj, changedStatus) {
		t.Fatal("expected VM status PCI devices update to be processed")
	}

	now := metav1.Now()
	changedDeletion := oldObj.DeepCopy()
	changedDeletion.DeletionTimestamp = &now
	if !shouldProcessVirtualMachineUpdate(oldObj, changedDeletion) {
		t.Fatal("expected VM deletion timestamp update to be processed")
	}

	if shouldProcessVirtualMachineUpdate(nil, changedStatus) {
		t.Fatal("expected nil old object to be ignored")
	}
	if shouldProcessVirtualMachineUpdate(oldObj, nil) {
		t.Fatal("expected nil new object to be ignored")
	}
}
