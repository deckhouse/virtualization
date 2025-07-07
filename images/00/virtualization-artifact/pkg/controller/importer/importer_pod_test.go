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

package importer

import (
	"testing"

	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"
)

func Test_MakePodSpec(t *testing.T) {
	podSettings := &PodSettings{
		Name:       "importer-pod",
		Image:      "localhost:5000/importer:latest",
		PullPolicy: string(corev1.PullAlways),
		Namespace:  "virt-controller",
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

	settings := &Settings{
		Verbose:                "1",
		Endpoint:               "https://localhost/mini.iso",
		Source:                 "HTTP",
		InsecureTLS:            false,
		DestinationEndpoint:    "dvcr:5000/test-image:latest",
		DestinationInsecureTLS: "false",
		DestinationAuthSecret:  "dvcr-auth",
	}

	imp := NewImporter(podSettings, settings)

	pod, err := imp.makeImporterPodSpec()
	require.NoError(t, err)

	if pod.Namespace == "" {
		t.Fatalf("pod.Namespace should not be empty!")
	}
}

func Test_MakePodSpec_CABundle(t *testing.T) {
	podSettings := &PodSettings{
		Name:       "importer-pod",
		Image:      "localhost:5000/importer:latest",
		PullPolicy: string(corev1.PullAlways),
		Namespace:  "virt-controller",
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

	settings := &Settings{
		Verbose:                "1",
		Endpoint:               "https://localhost/mini.iso",
		Source:                 "HTTP",
		InsecureTLS:            false,
		CertConfigMap:          "cm-name",
		DestinationEndpoint:    "dvcr:5000/test-image:latest",
		DestinationInsecureTLS: "false",
		DestinationAuthSecret:  "dvcr-auth",
	}

	imp := NewImporter(podSettings, settings)

	pod, err := imp.makeImporterPodSpec()
	require.NoError(t, err)

	hasCAVol := false
	for _, vol := range pod.Spec.Volumes {
		if vol.Name == certVolName {
			hasCAVol = true
			if vol.ConfigMap.Name == "" {
				t.Fatalf("configMap.name should not be empty for %s volume", caBundleVolName)
			}
		}
	}

	if !hasCAVol {
		t.Fatalf("should add %s volume to Pod", caBundleVolName)
	}
}
