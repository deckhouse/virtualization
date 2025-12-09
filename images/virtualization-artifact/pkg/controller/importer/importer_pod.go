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
	"context"
	"strconv"

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
	// CertVolName is the name of the volume containing certs
	certVolName = "cert-vol"

	// CABundleVolName is the name of the volume containing certs from dataSource.http.caBundle field.
	caBundleVolName = "ca-bundle-vol"

	// AnnOwnerRef is used when owner is in a different namespace
	AnnOwnerRef = annotations.AnnAPIGroup + "/storage.ownerRef"

	// PodRunningReason is const that defines the pod was started as a reason
	// PodRunningReason = "Pod is running"

	// ProxyCertVolName is the name of the volumecontaining certs
	proxyCertVolName = "cdi-proxy-cert-vol"

	// destinationAuthVol is the name of the volume containing DVCR docker auth config.
	destinationAuthVol = "dvcr-secret-vol"

	// sourceRegistryAuthVol is the name of the volume containing source registry docker auth config.
	sourceRegistryAuthVol = "source-registry-secret-vol"

	// KeyAccess provides a constant to the accessKeyId label using in controller pkg and transport_test.go
	KeyAccess = "accessKeyId"

	// KeySecret provides a constant to the secretKey label using in controller pkg and transport_test.go
	KeySecret = "secretKey"
)

type Importer struct {
	PodSettings *PodSettings
	EnvSettings *Settings
}

func NewImporter(podSettings *PodSettings, envSettings *Settings) *Importer {
	return &Importer{
		PodSettings: podSettings,
		EnvSettings: envSettings,
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

// GetOrCreatePod creates and returns a pointer to a pod which is created based on the passed-in endpoint, secret
// name, etc. A nil secret means the endpoint credentials are not passed to the
// importer pod.
func (imp *Importer) GetOrCreatePod(ctx context.Context, c client.Client) (*corev1.Pod, error) {
	pod, err := imp.makeImporterPodSpec()
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

// makeImporterPodSpec creates and return the importer pod spec based on the passed-in endpoint, secret and pvc.
func (imp *Importer) makeImporterPodSpec() (*corev1.Pod, error) {
	pod := corev1.Pod{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Pod",
			APIVersion: "v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      imp.PodSettings.Name,
			Namespace: imp.PodSettings.Namespace,
			Labels: map[string]string{
				annotations.AppLabel: annotations.DVCRLabelValue,
			},
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


	container := imp.makeImporterContainerSpec()
	imp.addVolumes(&pod, container)
	pod.Spec.Containers = append(pod.Spec.Containers, *container)

	annotations.SetRecommendedLabels(&pod, imp.PodSettings.InstallerLabels, imp.PodSettings.ControllerName)
	podutil.SetRestrictedSecurityContext(&pod.Spec)

	return &pod, nil
}

func (imp *Importer) makeImporterContainerSpec() *corev1.Container {
	container := &corev1.Container{
		Name:            common.ImporterContainerName,
		Image:           imp.PodSettings.Image,
		ImagePullPolicy: corev1.PullPolicy(imp.PodSettings.PullPolicy),
		Command:         []string{"/usr/local/bin/dvcr-importer"},
		Args:            []string{"-v=" + imp.EnvSettings.Verbose},
		Ports: []corev1.ContainerPort{
			{
				Name:          "metrics",
				ContainerPort: 8443,
				Protocol:      corev1.ProtocolTCP,
			},
		},
		Env: imp.makeImporterContainerEnv(),
		SecurityContext: &corev1.SecurityContext{
			ReadOnlyRootFilesystem: ptr.To(true),
		},
	}

	if imp.PodSettings.ResourceRequirements != nil {
		container.Resources = *imp.PodSettings.ResourceRequirements
	}

	return container
}

// makeImporterEnvs returns the Env portion for the importer container.
func (imp *Importer) makeImporterContainerEnv() []corev1.EnvVar {
	env := []corev1.EnvVar{
		{
			Name:  common.ImporterSource,
			Value: imp.EnvSettings.Source,
		},
		{
			Name:  common.ImporterEndpoint,
			Value: imp.EnvSettings.Endpoint,
		},
		{
			Name:  common.ImporterContentType,
			Value: imp.EnvSettings.ContentType,
		},
		{
			Name:  common.ImporterImageSize,
			Value: imp.EnvSettings.ImageSize,
		},
		{
			Name:  common.OwnerUID,
			Value: string(imp.PodSettings.OwnerReference.UID),
		},
		{
			Name:  common.FilesystemOverheadVar,
			Value: imp.EnvSettings.FilesystemOverhead,
		},
		{
			Name:  common.InsecureTLSVar,
			Value: strconv.FormatBool(imp.EnvSettings.InsecureTLS),
		},
		{
			Name:  common.ImporterDiskID,
			Value: imp.EnvSettings.DiskID,
		},
		{
			Name:  common.ImporterUUID,
			Value: imp.EnvSettings.UUID,
		},
		{
			Name:  common.ImporterReadyFile,
			Value: imp.EnvSettings.ReadyFile,
		},
		{
			Name:  common.ImporterDoneFile,
			Value: imp.EnvSettings.DoneFile,
		},
		{
			Name:  common.ImporterBackingFile,
			Value: imp.EnvSettings.BackingFile,
		},
		{
			Name:  common.ImporterThumbprint,
			Value: imp.EnvSettings.Thumbprint,
		},
		{
			Name:  common.ImportProxyHTTP,
			Value: imp.EnvSettings.HTTPProxy,
		},
		{
			Name:  common.ImportProxyHTTPS,
			Value: imp.EnvSettings.HTTPSProxy,
		},
		{
			Name:  common.ImportProxyNoProxy,
			Value: imp.EnvSettings.NoProxy,
		},
	}

	// Destination settings: endpoint, insecure flag.
	env = append(env, []corev1.EnvVar{
		{
			Name:  common.ImporterDestinationEndpoint,
			Value: imp.EnvSettings.DestinationEndpoint,
		},
		{
			Name:  common.DestinationInsecureTLSVar,
			Value: imp.EnvSettings.DestinationInsecureTLS,
		},
	}...)

	// HTTP source checksum settings: md5 and sha256.
	if imp.EnvSettings.SHA256 != "" {
		env = append(env, corev1.EnvVar{
			Name:  common.ImporterSHA256Sum,
			Value: imp.EnvSettings.SHA256,
		})
	}
	if imp.EnvSettings.MD5 != "" {
		env = append(env, corev1.EnvVar{
			Name:  common.ImporterMD5Sum,
			Value: imp.EnvSettings.MD5,
		})
	}

	// Pass basic auth configuration from Secret with downward API.
	if imp.EnvSettings.SecretName != "" {
		env = append(env, corev1.EnvVar{
			Name: common.ImporterAccessKeyID,
			ValueFrom: &corev1.EnvVarSource{
				SecretKeyRef: &corev1.SecretKeySelector{
					LocalObjectReference: corev1.LocalObjectReference{
						Name: imp.EnvSettings.SecretName,
					},
					Key: KeyAccess,
				},
			},
		}, corev1.EnvVar{
			Name: common.ImporterSecretKey,
			ValueFrom: &corev1.EnvVarSource{
				SecretKeyRef: &corev1.SecretKeySelector{
					LocalObjectReference: corev1.LocalObjectReference{
						Name: imp.EnvSettings.SecretName,
					},
					Key: KeySecret,
				},
			},
		})
	}

	return env
}

// addVolumes fills Volumes in Pod spec and VolumeMounts and envs in container spec.
func (imp *Importer) addVolumes(pod *corev1.Pod, container *corev1.Container) {
	podutil.AddEmptyDirVolume(pod, container, "tmp", "/tmp")

	if imp.EnvSettings.AuthSecret != "" {
		// Mount source registry auth Secret and pass directory with mounted source registry login config.
		podutil.AddVolume(pod, container,
			podutil.CreateSecretVolume(sourceRegistryAuthVol, imp.EnvSettings.AuthSecret),
			podutil.CreateVolumeMount(sourceRegistryAuthVol, common.ImporterAuthConfigDir),
			corev1.EnvVar{
				Name:  common.ImporterAuthConfigVar,
				Value: common.ImporterAuthConfigFile,
			},
		)
	}

	if imp.EnvSettings.DestinationAuthSecret != "" {
		// Mount DVCR auth Secret and pass directory with mounted DVCR login config.
		podutil.AddVolume(pod, container,
			podutil.CreateSecretVolume(destinationAuthVol, imp.EnvSettings.DestinationAuthSecret),
			podutil.CreateVolumeMount(destinationAuthVol, common.ImporterDestinationAuthConfigDir),
			corev1.EnvVar{
				Name:  common.ImporterDestinationAuthConfigVar,
				Value: common.ImporterDestinationAuthConfigFile,
			},
		)
	}

	// Volume with CA certificates either from caBundle field or from existing ConfigMap.
	if imp.EnvSettings.CertConfigMap != "" {
		podutil.AddVolume(pod, container,
			podutil.CreateConfigMapVolume(certVolName, imp.EnvSettings.CertConfigMap),
			podutil.CreateVolumeMount(certVolName, common.ImporterCertDir),
			corev1.EnvVar{
				Name:  common.ImporterCertDirVar,
				Value: common.ImporterCertDir,
			},
		)
	}

	if imp.EnvSettings.CertConfigMapProxy != "" {
		podutil.AddVolume(pod, container,
			podutil.CreateConfigMapVolume(proxyCertVolName, imp.EnvSettings.CertConfigMapProxy), //  GetImportProxyConfigMapName(args.cvmi.Name)
			podutil.CreateVolumeMount(proxyCertVolName, common.ImporterProxyCertDir),
			corev1.EnvVar{
				Name:  common.ImporterProxyCertDirVar,
				Value: common.ImporterProxyCertDir,
			},
		)
	}

	if imp.PodSettings.PVCName != "" {
		volume := corev1.Volume{
			Name: "volume",
			VolumeSource: corev1.VolumeSource{
				PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{
					ClaimName: imp.PodSettings.PVCName,
				},
			},
		}

		if imp.EnvSettings.Source == SourceFilesystem {
			podutil.AddVolume(pod, container, volume, corev1.VolumeMount{Name: "volume", MountPath: "/tmp/fs"}, corev1.EnvVar{Name: "IMPORTER_FILESYSTEM_DIR", Value: "/tmp/fs"})
		} else {
			podutil.AddVolumeDevice(pod, container, volume, corev1.VolumeDevice{Name: "volume", DevicePath: "/dev/xvda"})
		}
	}
}
