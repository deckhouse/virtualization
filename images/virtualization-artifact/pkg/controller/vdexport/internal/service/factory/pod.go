/*
Copyright 2025 Flant JSC

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
	args := []string{
		"--listen-address", "0.0.0.0",
		"--listen-port", strconv.Itoa(exporterPort),
		"--image", d.exportImage,
	}
	if !d.withCA {
		args = append(args, "--dest-insecure")
	}

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
					Args:            args,
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
	// Mount DVCR auth Secret and pass directory with mounted DVCR login config.
	podutil.AddVolume(pod, container,
		podutil.CreateSecretVolume(destinationAuthVol, d.sup.DVCRAuthSecret().Name),
		podutil.CreateVolumeMount(destinationAuthVol, "/dvcr-auth"),
		corev1.EnvVar{
			Name:  "EXPORTER_DEST_AUTH_CONFIG",
			Value: "/dvcr-auth/.dockerconfigjson",
		},
	)
	if d.withCA {
		podutil.AddVolume(pod, container,
			podutil.CreateConfigMapVolume(destinationCACertVol, d.sup.CABundleConfigMap().Name),
			podutil.CreateVolumeMount(destinationCACertVol, "/dvcr-ca"),
			corev1.EnvVar{
				Name:  "EXPORTER_DEST_CERT_PATH",
				Value: "/dvcr-ca/ca.crt",
			},
		)
	}
}
