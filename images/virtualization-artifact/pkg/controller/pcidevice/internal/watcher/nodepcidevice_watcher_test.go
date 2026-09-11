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

func TestShouldProcessNodePCIDeviceUpdate(t *testing.T) {
	oldObj := &v1alpha2.NodePCIDevice{
		Spec: v1alpha2.NodePCIDeviceSpec{AssignedNamespace: "ns-a"},
		Status: v1alpha2.NodePCIDeviceStatus{
			NodeName: "node-a",
			Conditions: []metav1.Condition{{
				Type:   "Ready",
				Status: metav1.ConditionTrue,
			}},
		},
	}

	sameObj := oldObj.DeepCopy()
	if shouldProcessNodePCIDeviceUpdate(oldObj, sameObj) {
		t.Fatal("expected unchanged object update to be ignored")
	}

	changedNamespace := oldObj.DeepCopy()
	changedNamespace.Spec.AssignedNamespace = "ns-b"
	if !shouldProcessNodePCIDeviceUpdate(oldObj, changedNamespace) {
		t.Fatal("expected assigned namespace update to be processed")
	}

	changedConditions := oldObj.DeepCopy()
	changedConditions.Status.Conditions[0].Reason = "Changed"
	if !shouldProcessNodePCIDeviceUpdate(oldObj, changedConditions) {
		t.Fatal("expected conditions update to be processed")
	}

	if shouldProcessNodePCIDeviceUpdate(nil, changedConditions) {
		t.Fatal("expected nil old object to be ignored")
	}
	if shouldProcessNodePCIDeviceUpdate(oldObj, nil) {
		t.Fatal("expected nil new object to be ignored")
	}
}
