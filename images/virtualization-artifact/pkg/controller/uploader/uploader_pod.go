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

package uploader

import (
	"context"

	corev1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/deckhouse/virtualization-controller/pkg/common"
	"github.com/deckhouse/virtualization-controller/pkg/common/annotations"
	podutil "github.com/deckhouse/virtualization-controller/pkg/common/pod"
	"github.com/deckhouse/virtualization-controller/pkg/common/provisioner"
)

const (
	// destinationAuthVol is the name of the volume containing DVCR docker auth config.
	destinationAuthVol = "dvcr-secret-vol"
)

type Pod struct {
	PodSettings *PodSettings
	Settings    *Settings
}

func NewPod(podSettings *PodSettings, settings *Settings) *Pod {
	return &Pod{
		PodSettings: podSettings,
		Settings:    settings,
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
	ServiceName          string
	NodePlacement        *provisioner.NodePlacement
}

// GetOrCreate creates and returns a pointer to a pod which is created based on the passed-in endpoint, secret
// name, etc. A nil secret means the endpoint credentials are not passed to the uploader pod.
func (p *Pod) GetOrCreate(ctx context.Context, c client.Client) (*corev1.Pod, error) {
	pod, err := p.makeSpec()
	if err != nil {
		return nil, err
	}

	err = c.Create(ctx, pod)
	if err == nil {
		return pod, nil
	}
	if k8serrors.IsAlreadyExists(err) {
		err = c.Get(ctx, client.ObjectKeyFromObject(pod), pod)
		return pod, err
	}
	return nil, err
}

func (p *Pod) makeSpec() (*corev1.Pod, error) {
	pod := corev1.Pod{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Pod",
			APIVersion: "v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      p.PodSettings.Name,
			Namespace: p.PodSettings.Namespace,
			Annotations: map[string]string{
				annotations.AnnCreatedBy: "yes",
			},
			Labels: map[string]string{
				annotations.AppLabel:             annotations.DVCRLabelValue,
				annotations.UploaderServiceLabel: p.PodSettings.ServiceName,
				annotations.QuotaExcludeLabel:    annotations.QuotaExcludeValue,
			},
			OwnerReferences: []metav1.OwnerReference{
				p.PodSettings.OwnerReference,
			},
		},
		Spec: corev1.PodSpec{
			// Container and volumes will be added later.
			Containers:        []corev1.Container{},
			Volumes:           []corev1.Volume{},
			RestartPolicy:     corev1.RestartPolicyOnFailure,
			PriorityClassName: p.PodSettings.PriorityClassName,
			ImagePullSecrets:  p.PodSettings.ImagePullSecrets,
		},
	}

	if p.PodSettings.NodePlacement != nil && len(p.PodSettings.NodePlacement.Tolerations) > 0 {
		pod.Spec.Tolerations = p.PodSettings.NodePlacement.Tolerations

		err := provisioner.KeepNodePlacementTolerations(p.PodSettings.NodePlacement, &pod)
		if err != nil {
			return nil, err
		}
	}

	container := p.makeUploaderContainerSpec()
	p.addVolumes(&pod, container)
	pod.Spec.Containers = append(pod.Spec.Containers, *container)

	annotations.SetRecommendedLabels(&pod, p.PodSettings.InstallerLabels, p.PodSettings.ControllerName)
	podutil.SetRestrictedSecurityContext(&pod.Spec)

	return &pod, nil
}

func (p *Pod) makeUploaderContainerSpec() *corev1.Container {
	container := &corev1.Container{
		Name:            common.UploaderContainerName,
		Image:           p.PodSettings.Image,
		ImagePullPolicy: corev1.PullPolicy(p.PodSettings.PullPolicy),
		Command:         []string{"/usr/local/bin/dvcr-uploader"},
		Args:            []string{"-v=" + p.Settings.Verbose},
		Ports: []corev1.ContainerPort{
			{
				Name:          "metrics",
				ContainerPort: 8443,
				Protocol:      corev1.ProtocolTCP,
			},
		},
		Env: p.makeUploaderContainerEnv(),
		SecurityContext: &corev1.SecurityContext{
			ReadOnlyRootFilesystem: ptr.To(true),
		},
	}

	if p.PodSettings.ResourceRequirements != nil {
		container.Resources = *p.PodSettings.ResourceRequirements
	}

	return container
}

func (p *Pod) makeUploaderContainerEnv() []corev1.EnvVar {
	return []corev1.EnvVar{
		{
			Name:  common.OwnerUID,
			Value: string(p.PodSettings.OwnerReference.UID),
		},
		{
			Name:  common.UploaderDestinationEndpoint,
			Value: p.Settings.DestinationEndpoint,
		},
		{
			Name:  common.DestinationInsecureTLSVar,
			Value: p.Settings.DestinationInsecureTLS,
		},
	}
}

// addVolumes fills Volumes in Pod spec and VolumeMounts and envs in container spec.
func (p *Pod) addVolumes(pod *corev1.Pod, container *corev1.Container) {
	podutil.AddEmptyDirVolume(pod, container, "tmp", "/tmp")

	if p.Settings.DestinationAuthSecret != "" {
		// Mount DVCR auth Secret and pass directory with mounted DVCR login config.
		podutil.AddVolume(pod, container,
			podutil.CreateSecretVolume(destinationAuthVol, p.Settings.DestinationAuthSecret),
			podutil.CreateVolumeMount(destinationAuthVol, common.UploaderDestinationAuthConfigDir),
			corev1.EnvVar{
				Name:  common.UploaderDestinationAuthConfigVar,
				Value: common.UploaderDestinationAuthConfigFile,
			},
		)
	}
}
