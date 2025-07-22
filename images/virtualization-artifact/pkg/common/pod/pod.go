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

package pod

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"
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

func AddEmptyDirVolume(pod *corev1.Pod, container *corev1.Container, volumeName, mountPath string) {
	pod.Spec.Volumes = append(pod.Spec.Volumes, corev1.Volume{Name: volumeName, VolumeSource: corev1.VolumeSource{EmptyDir: &corev1.EmptyDirVolumeSource{}}})
	container.VolumeMounts = append(container.VolumeMounts, corev1.VolumeMount{Name: volumeName, MountPath: mountPath})
}

func AddVolumeDevice(pod *corev1.Pod, container *corev1.Container, volume corev1.Volume, volumeDevice corev1.VolumeDevice) {
	pod.Spec.Volumes = append(pod.Spec.Volumes, volume)
	container.VolumeDevices = append(container.VolumeDevices, volumeDevice)
}

// IsPodRunning returns true if a Pod is in 'Running' phase, false if not.
func IsPodRunning(pod *corev1.Pod) bool {
	return pod != nil && pod.Status.Phase == corev1.PodRunning
}

// IsPodStarted returns true if a Pod is in started state, false if not.
func IsPodStarted(pod *corev1.Pod) bool {
	if pod == nil || pod.Status.StartTime == nil {
		return false
	}

	for _, cs := range pod.Status.ContainerStatuses {
		if cs.Started == nil || !*cs.Started {
			return false
		}
	}

	return true
}

// IsPodComplete returns true if a Pod is in 'Succeeded' phase, false if not.
func IsPodComplete(pod *corev1.Pod) bool {
	return pod != nil && pod.Status.Phase == corev1.PodSucceeded
}

// QemuSubGID is the gid used as the qemu group in fsGroup
const QemuSubGID = int64(107)

// SetRestrictedSecurityContext sets the pod security params to be compatible with restricted PSA
func SetRestrictedSecurityContext(podSpec *corev1.PodSpec) {
	hasVolumeMounts := false
	for _, containers := range [][]corev1.Container{podSpec.InitContainers, podSpec.Containers} {
		for i := range containers {
			container := &containers[i]
			if container.SecurityContext == nil {
				container.SecurityContext = &corev1.SecurityContext{}
			}
			container.SecurityContext.Capabilities = &corev1.Capabilities{
				Drop: []corev1.Capability{
					"ALL",
				},
			}
			container.SecurityContext.SeccompProfile = &corev1.SeccompProfile{
				Type: corev1.SeccompProfileTypeRuntimeDefault,
			}
			container.SecurityContext.AllowPrivilegeEscalation = ptr.To(false)
			container.SecurityContext.RunAsNonRoot = ptr.To(true)
			container.SecurityContext.RunAsUser = ptr.To(QemuSubGID)
			if len(container.VolumeMounts) > 0 {
				hasVolumeMounts = true
			}
		}
	}

	if hasVolumeMounts {
		if podSpec.SecurityContext == nil {
			podSpec.SecurityContext = &corev1.PodSecurityContext{}
		}
		podSpec.SecurityContext.FSGroup = ptr.To(QemuSubGID)
	}
}
