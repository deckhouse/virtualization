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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/deckhouse/virtualization/api/core/v1alpha2"
)

func TestShouldProcessResourceClaimTemplateUpdate(t *testing.T) {
	oldObj := &resourcev1.ResourceClaimTemplate{
		ObjectMeta: metav1.ObjectMeta{
			OwnerReferences: []metav1.OwnerReference{{Kind: "PCIDevice", Name: "pci-a"}},
		},
	}

	sameObj := oldObj.DeepCopy()
	if shouldProcessResourceClaimTemplateUpdate(oldObj, sameObj) {
		t.Fatal("expected unchanged owner references update to be ignored")
	}

	changedOwners := oldObj.DeepCopy()
	changedOwners.OwnerReferences = []metav1.OwnerReference{{Kind: "PCIDevice", Name: "pci-b"}}
	if !shouldProcessResourceClaimTemplateUpdate(oldObj, changedOwners) {
		t.Fatal("expected owner references update to be processed")
	}

	changedSpec := oldObj.DeepCopy()
	changedSpec.Spec = resourcev1.ResourceClaimTemplateSpec{
		Spec: resourcev1.ResourceClaimSpec{
			Devices: resourcev1.DeviceClaim{
				Requests: []resourcev1.DeviceRequest{{
					Name: "req-pci-a",
				}},
			},
		},
	}
	if !shouldProcessResourceClaimTemplateUpdate(oldObj, changedSpec) {
		t.Fatal("expected spec update to be processed")
	}

	if shouldProcessResourceClaimTemplateUpdate(nil, changedOwners) {
		t.Fatal("expected nil old object to be ignored")
	}
	if shouldProcessResourceClaimTemplateUpdate(oldObj, nil) {
		t.Fatal("expected nil new object to be ignored")
	}
}

func TestMapResourceClaimTemplateToPCIDeviceName(t *testing.T) {
	templateWithOwner := &resourcev1.ResourceClaimTemplate{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "pci-a-template",
			Namespace: "ns-a",
			OwnerReferences: []metav1.OwnerReference{{
				Kind:       v1alpha2.PCIDeviceKind,
				APIVersion: v1alpha2.SchemeGroupVersion.String(),
				Name:       "pci-a",
			}},
		},
	}

	name, ok := mapResourceClaimTemplateToPCIDeviceName(templateWithOwner)
	if !ok || name != "pci-a" {
		t.Fatalf("expected name from owner reference, got %q (%v)", name, ok)
	}

	templateBySuffix := &resourcev1.ResourceClaimTemplate{
		ObjectMeta: metav1.ObjectMeta{Name: "pci-b-template"},
	}

	name, ok = mapResourceClaimTemplateToPCIDeviceName(templateBySuffix)
	if !ok || name != "pci-b" {
		t.Fatalf("expected name from template suffix, got %q (%v)", name, ok)
	}

	templateWithoutOwnerAndSuffix := &resourcev1.ResourceClaimTemplate{
		ObjectMeta: metav1.ObjectMeta{Name: "something-else"},
	}

	name, ok = mapResourceClaimTemplateToPCIDeviceName(templateWithoutOwnerAndSuffix)
	if ok {
		t.Fatalf("expected no mapped name, got %q", name)
	}
}
