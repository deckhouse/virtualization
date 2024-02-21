package pod

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// MakeOwnerReference makes owner reference from a Pod
func MakeOwnerReference(pod *corev1.Pod) metav1.OwnerReference {
	blockOwnerDeletion := true
	isController := true
	return metav1.OwnerReference{
		APIVersion:         "v1",
		Kind:               "Pod",
		Name:               pod.Name,
		UID:                pod.GetUID(),
		BlockOwnerDeletion: &blockOwnerDeletion,
		Controller:         &isController,
	}
}

func CreateConfigMapVolume(certVolName, objRef string) corev1.Volume {
	return corev1.Volume{
		Name: certVolName,
		VolumeSource: corev1.VolumeSource{
			ConfigMap: &corev1.ConfigMapVolumeSource{
				LocalObjectReference: corev1.LocalObjectReference{
					Name: objRef,
				},
			},
		},
	}
}

func CreateSecretVolume(thisVolName, objRef string) corev1.Volume {
	return corev1.Volume{
		Name: thisVolName,
		VolumeSource: corev1.VolumeSource{
			Secret: &corev1.SecretVolumeSource{
				SecretName: objRef,
			},
		},
	}
}

func CreateVolumeMount(volName, mountPath string) corev1.VolumeMount {
	return corev1.VolumeMount{
		Name:      volName,
		MountPath: mountPath,
	}
}

func AddVolume(pod *corev1.Pod, container *corev1.Container, volume corev1.Volume, mount corev1.VolumeMount, envVar corev1.EnvVar) {
	pod.Spec.Volumes = append(pod.Spec.Volumes, volume)
	container.VolumeMounts = append(container.VolumeMounts, mount)
	container.Env = append(container.Env, envVar)
}
