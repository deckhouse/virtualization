/*
Copyright 2024 Flant JSC

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

package bounder

import (
	"testing"

	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"
)

func Test_MakePodSpec(t *testing.T) {
	podSettings := &PodSettings{
		Name:       "bounder-pod",
		Image:      "localhost:5000/bounder:latest",
		PullPolicy: string(corev1.PullAlways),
		Namespace:  "virt-controller",
		PVCName:    "bounder-pvc",
		OwnerReference: metav1.OwnerReference{
			APIVersion:         "v1",
			Kind:               "Pod",
			Name:               "other-pod",
			UID:                "123-123",
			Controller:         ptr.To(true),
			BlockOwnerDeletion: ptr.To(true),
		},
		ControllerName: "test-controller",
	}

	imp := NewBounder(podSettings)

	pod, err := imp.makeBounderPodSpec()
	require.NoError(t, err)

	if pod.Namespace == "" {
		t.Fatalf("pod.Namespace should not be empty!")
	}
}
