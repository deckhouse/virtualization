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
	"context"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/deckhouse/virtualization-controller/pkg/common"
	"github.com/deckhouse/virtualization-controller/pkg/common/annotations"
	podutil "github.com/deckhouse/virtualization-controller/pkg/common/pod"
	"github.com/deckhouse/virtualization-controller/pkg/common/provisioner"
)

type Bounder struct {
	PodSettings *PodSettings
}

func NewBounder(podSettings *PodSettings) *Bounder {
	return &Bounder{
		PodSettings: podSettings,
	}
}

type PodSettings struct {
	Name                 string
	Image                string
	PullPolicy           string
	Namespace            string
	OwnerReference       metav1.OwnerReference
	ControllerName       string
	InstallerLabels      map[string]string
	ResourceRequirements *corev1.ResourceRequirements
	ImagePullSecrets     []corev1.LocalObjectReference
	PriorityClassName    string
	PVCName              string
	NodePlacement        *provisioner.NodePlacement
	Finalizer            string
}

// CreatePod creates and returns a pointer to a pod which is created based on the passed-in endpoint, secret
// name, etc. A nil secret means the endpoint credentials are not passed to the
// bounder pod.
func (imp *Bounder) CreatePod(ctx context.Context, client client.Client) (*corev1.Pod, error) {
	pod, err := imp.makeBounderPodSpec()
	if err != nil {
		return nil, err
	}

	err = client.Create(ctx, pod)
	if err != nil {
		return nil, err
	}

	return pod, nil
}

// makeBounderPodSpec creates and return the bounder pod spec based on the passed-in endpoint, secret and pvc.
func (imp *Bounder) makeBounderPodSpec() (*corev1.Pod, error) {
	pod := corev1.Pod{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Pod",
			APIVersion: "v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      imp.PodSettings.Name,
			Namespace: imp.PodSettings.Namespace,
			Annotations: map[string]string{
				annotations.AnnCreatedBy: "yes",
			},
			Finalizers: []string{
				imp.PodSettings.Finalizer,
			},
			OwnerReferences: []metav1.OwnerReference{
				imp.PodSettings.OwnerReference,
			},
		},
		Spec: corev1.PodSpec{
			// Container and volumes will be added later.
			Containers:        []corev1.Container{},
			Volumes:           []corev1.Volume{},
			RestartPolicy:     corev1.RestartPolicyOnFailure,
			PriorityClassName: imp.PodSettings.PriorityClassName,
			ImagePullSecrets:  imp.PodSettings.ImagePullSecrets,
		},
	}

	if imp.PodSettings.NodePlacement != nil && len(imp.PodSettings.NodePlacement.Tolerations) > 0 {
		pod.Spec.Tolerations = imp.PodSettings.NodePlacement.Tolerations

		err := provisioner.KeepNodePlacementTolerations(imp.PodSettings.NodePlacement, &pod)
		if err != nil {
			return nil, err
		}
	}

	annotations.SetRecommendedLabels(&pod, imp.PodSettings.InstallerLabels, imp.PodSettings.ControllerName)
	podutil.SetRestrictedSecurityContext(&pod.Spec)

	container := imp.makeBounderContainerSpec()
	imp.addVolumes(&pod, container)
	pod.Spec.Containers = append(pod.Spec.Containers, *container)

	return &pod, nil
}

func (imp *Bounder) makeBounderContainerSpec() *corev1.Container {
	container := &corev1.Container{
		Name:            common.BounderContainerName,
		Image:           imp.PodSettings.Image,
		ImagePullPolicy: corev1.PullPolicy(imp.PodSettings.PullPolicy),
		SecurityContext: &corev1.SecurityContext{
			ReadOnlyRootFilesystem: ptr.To(true),
		},
	}

	if imp.PodSettings.ResourceRequirements != nil {
		container.Resources = *imp.PodSettings.ResourceRequirements
	}

	return container
}

// addVolumes fills Volumes in Pod spec and VolumeMounts and envs in container spec.
func (imp *Bounder) addVolumes(pod *corev1.Pod, container *corev1.Container) {
	if imp.PodSettings.PVCName != "" {
		podutil.AddVolumeDevice(
			pod,
			container,
			corev1.Volume{
				Name: "volume",
				VolumeSource: corev1.VolumeSource{
					PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{
						ClaimName: imp.PodSettings.PVCName,
					},
				},
			},
			corev1.VolumeDevice{
				Name:       "volume",
				DevicePath: "/dev/xvda",
			},
		)
	}
}
