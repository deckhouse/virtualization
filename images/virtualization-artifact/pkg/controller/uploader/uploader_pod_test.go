package uploader

import (
	"testing"

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
		DestinationEndpoint:    "dvcr:5000/test-image:latest",
		DestinationInsecureTLS: "false",
		DestinationAuthSecret:  "dvcr-auth",
	}

	imp := NewPod(podSettings, settings)

	pod := imp.makeSpec()

	if pod.Namespace == "" {
		t.Fatalf("pod.Namespace should not be empty!")
	}
}
