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

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/deckhouse/virtualization-controller/pkg/common/annotations"
)

func node(unschedulable bool, nodeAnnotations map[string]string) *corev1.Node {
	return &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{Annotations: nodeAnnotations},
		Spec:       corev1.NodeSpec{Unschedulable: unschedulable},
	}
}

func TestNodeMaintenanceChanged(t *testing.T) {
	tests := []struct {
		name     string
		oldNode  *corev1.Node
		newNode  *corev1.Node
		expected bool
	}{
		{
			name:    "node closed for new workloads",
			oldNode: node(false, nil),
			newNode: node(true, nil),
			// A virtual machine has to look at its node again: the node may be under maintenance now.
			expected: true,
		},
		{
			name:     "maintenance marker added",
			oldNode:  node(true, nil),
			newNode:  node(true, map[string]string{annotations.AnnNodeCordonedBy: "shutdown-inhibitor"}),
			expected: true,
		},
		{
			name:     "drain gave up and left the drained marker",
			oldNode:  node(true, map[string]string{annotations.AnnNodeDraining: "user"}),
			newNode:  node(true, map[string]string{annotations.AnnNodeDrained: "user"}),
			expected: true,
		},
		{
			name:     "who closed the node changed",
			oldNode:  node(true, map[string]string{annotations.AnnNodeCordonedBy: "user"}),
			newNode:  node(true, map[string]string{annotations.AnnNodeCordonedBy: "shutdown-inhibitor"}),
			expected: true,
		},
		{
			name:     "restart approval appeared",
			oldNode:  node(true, map[string]string{annotations.AnnNodeDraining: "user"}),
			newNode:  node(true, map[string]string{annotations.AnnNodeDraining: "user", annotations.AnnNodeVMRestartApproved: ""}),
			expected: true,
		},
		{
			name:     "unrelated annotation added",
			oldNode:  node(false, map[string]string{"example.com/note": "a"}),
			newNode:  node(false, map[string]string{"example.com/note": "b"}),
			expected: false,
		},
		{
			name:     "nothing changed",
			oldNode:  node(true, map[string]string{annotations.AnnNodeDraining: "user"}),
			newNode:  node(true, map[string]string{annotations.AnnNodeDraining: "user"}),
			expected: false,
		},
		{
			name:     "node disappeared",
			oldNode:  node(true, nil),
			newNode:  nil,
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := NodeMaintenanceChanged(tt.oldNode, tt.newNode); got != tt.expected {
				t.Fatalf("NodeMaintenanceChanged() = %v, want %v", got, tt.expected)
			}
		})
	}
}
