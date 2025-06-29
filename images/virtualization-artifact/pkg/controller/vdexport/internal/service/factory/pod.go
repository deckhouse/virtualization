package factory

import (
	"maps"
	"strconv"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/deckhouse/virtualization-controller/pkg/common/annotations"
	podutil "github.com/deckhouse/virtualization-controller/pkg/common/pod"
)

func (d defaultFactory) Pod() *corev1.Pod {
	pod := &corev1.Pod{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Pod",
			APIVersion: "v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      d.sup.ExporterPod().Name,
			Namespace: d.sup.ExporterPod().Namespace,
			Annotations: map[string]string{
				annotations.AnnCreatedBy: "yes",
			},
			Labels: map[string]string{
				annotations.AppLabel: annotations.DVCRExporterLabelValues,
			},
			OwnerReferences: []metav1.OwnerReference{
				d.makeOwnerReference(),
			},
		},
		Spec: corev1.PodSpec{
			// Container and volumes will be added later.
			Containers: []corev1.Container{
				{
					Name:            "exporter",
					Image:           d.image,
					ImagePullPolicy: d.pullPolicy,
					Command:         []string{"/usr/local/bin/dvcr-exporter"},
					Args: []string{
						"--listen-address", "0.0.0.0",
						"--listen-port", strconv.Itoa(exporterPort),
					},
					Ports: []corev1.ContainerPort{
						{
							Name:          exporterPortName,
							ContainerPort: exporterPort,
							Protocol:      corev1.ProtocolTCP,
						},
					},
				},
			},
			Volumes:           []corev1.Volume{},
			RestartPolicy:     corev1.RestartPolicyOnFailure,
			PriorityClassName: d.priorityClassName,
			ImagePullSecrets:  d.imagePullSecrets,
			Tolerations:       d.tolerations,
		},
	}

	maps.Copy(pod.Labels, d.podSelector())
	annotations.SetRecommendedLabels(pod, d.extraLabels, d.controllerName)
	podutil.SetRestrictedSecurityContext(&pod.Spec)

	d.addVolumes(pod, &pod.Spec.Containers[0])

	return pod
}

func (d defaultFactory) addVolumes(pod *corev1.Pod, container *corev1.Container) {
	if d.dvcrAuthSecret != "" {
		// Mount DVCR auth Secret and pass directory with mounted DVCR login config.
		podutil.AddVolume(pod, container,
			podutil.CreateSecretVolume(destinationAuthVol, d.dvcrAuthSecret),
			podutil.CreateVolumeMount(destinationAuthVol, "/dvcr-auth"),
			corev1.EnvVar{
				Name:  "EXPORTER_AUTH_CONFIG",
				Value: "/dvcr-auth/.dockerconfigjson",
			},
		)
	}
}
