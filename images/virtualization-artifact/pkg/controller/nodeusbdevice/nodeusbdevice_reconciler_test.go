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

package nodeusbdevice

import (
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/nodeusbdevicecondition"
)

func TestShouldAutoDeleteNodeUSBDevice(t *testing.T) {
	tests := []struct {
		name          string
		nodeUSBDevice *v1alpha2.NodeUSBDevice
		expectDelete  bool
	}{
		{
			name: "delete unassigned not found device",
			nodeUSBDevice: &v1alpha2.NodeUSBDevice{
				Spec: v1alpha2.NodeUSBDeviceSpec{AssignedNamespace: ""},
				Status: v1alpha2.NodeUSBDeviceStatus{Conditions: []metav1.Condition{{
					Type:   string(nodeusbdevicecondition.ReadyType),
					Status: metav1.ConditionFalse,
					Reason: string(nodeusbdevicecondition.NotFound),
				}}},
			},
			expectDelete: true,
		},
		{
			name: "keep assigned not found device",
			nodeUSBDevice: &v1alpha2.NodeUSBDevice{
				Spec: v1alpha2.NodeUSBDeviceSpec{AssignedNamespace: "test-ns"},
				Status: v1alpha2.NodeUSBDeviceStatus{Conditions: []metav1.Condition{{
					Type:   string(nodeusbdevicecondition.ReadyType),
					Status: metav1.ConditionFalse,
					Reason: string(nodeusbdevicecondition.NotFound),
				}}},
			},
			expectDelete: false,
		},
		{
			name: "keep unassigned ready device",
			nodeUSBDevice: &v1alpha2.NodeUSBDevice{
				Spec: v1alpha2.NodeUSBDeviceSpec{AssignedNamespace: ""},
				Status: v1alpha2.NodeUSBDeviceStatus{Conditions: []metav1.Condition{{
					Type:   string(nodeusbdevicecondition.ReadyType),
					Status: metav1.ConditionTrue,
					Reason: string(nodeusbdevicecondition.Ready),
				}}},
			},
			expectDelete: false,
		},
		{
			name: "keep device already deleting",
			nodeUSBDevice: func() *v1alpha2.NodeUSBDevice {
				now := metav1.Now()
				return &v1alpha2.NodeUSBDevice{
					ObjectMeta: metav1.ObjectMeta{DeletionTimestamp: &now},
					Spec:       v1alpha2.NodeUSBDeviceSpec{AssignedNamespace: ""},
					Status: v1alpha2.NodeUSBDeviceStatus{Conditions: []metav1.Condition{{
						Type:   string(nodeusbdevicecondition.ReadyType),
						Status: metav1.ConditionFalse,
						Reason: string(nodeusbdevicecondition.NotFound),
					}}},
				}
			}(),
			expectDelete: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if actual := shouldAutoDeleteNodeUSBDevice(tt.nodeUSBDevice); actual != tt.expectDelete {
				t.Fatalf("expected delete=%v, got %v", tt.expectDelete, actual)
			}
		})
	}
}
