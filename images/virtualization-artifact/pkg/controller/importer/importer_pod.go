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
	"fmt"
	"path"
	"strconv"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	common "github.com/deckhouse/virtualization-controller/pkg/common"
	podutil "github.com/deckhouse/virtualization-controller/pkg/common/pod"
	cc "github.com/deckhouse/virtualization-controller/pkg/controller/common"
	"github.com/deckhouse/virtualization-controller/pkg/sdk/framework/helper"
)

const (
	// CertVolName is the name of the volume containing certs
	certVolName = "cert-vol"

	// CABundleVolName is the name of the volume containing certs from dataSource.http.caBundle field.
	caBundleVolName = "ca-bundle-vol"

	// AnnOwnerRef is used when owner is in a different namespace
	AnnOwnerRef = cc.AnnAPIGroup + "/storage.ownerRef"

	// PodRunningReason is const that defines the pod was started as a reason
	// PodRunningReason = "Pod is running"

	// ProxyCertVolName is the name of the volumecontaining certs
	proxyCertVolName = "cdi-proxy-cert-vol"

	// secretExtraHeadersVolumeName is the format string that specifies where extra HTTP header secrets will be mounted
	secretExtraHeadersVolumeName = "import-extra-headers-vol-%d"

	// destinationAuthVol is the name of the volume containing DVCR docker auth config.
	destinationAuthVol = "dvcr-secret-vol"

	// sourceRegistryAuthVol is the name of the volume containing source registry docker auth config.
	sourceRegistryAuthVol = "source-registry-secret-vol"
)

type Importer struct {
	PodSettings  *PodSettings
	EnvSettings  *Settings
	pvcName      string
	pvcNamespace string
}

func NewImporter(podSettings *PodSettings, envSettings *Settings, pvcName string, pvcNamespace string) *Importer {
	return &Importer{
		PodSettings:  podSettings,
		EnvSettings:  envSettings,
		pvcName:      pvcName,
		pvcNamespace: pvcNamespace,
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
	// workloadNodePlacement   *sdkapi.NodePlacement
}

// CreatePod creates and returns a pointer to a pod which is created based on the passed-in endpoint, secret
// name, etc. A nil secret means the endpoint credentials are not passed to the
// importer pod.
func (imp *Importer) CreatePod(ctx context.Context, client client.Client) (*corev1.Pod, error) {
	var err error
	// args.ResourceRequirements, err = cc.GetDefaultPodResourceRequirements(client)
	// if err != nil {
	//	return nil, err
	// }

	// args.ImagePullSecrets, err = cc.GetImagePullSecrets(client)
	// if err != nil {
	//	return nil, err
	// }

	// args.workloadNodePlacement, err = cc.GetWorkloadNodePlacement(ctx, client)
	// if err != nil {
	//	return nil, err
	// }

	pod := imp.makeImporterPodSpec()

	if err = client.Create(ctx, pod); err != nil {
		return nil, err
	}

	return pod, nil
}

// CleanupPod deletes importer Pod.
// No need to delete CABundle configmap and auth Secret. They have ownerRef and will be gc'ed.
func CleanupPod(ctx context.Context, client client.Client, pod *corev1.Pod) error {
	if pod == nil {
		return nil
	}

	return helper.CleanupObject(ctx, client, pod)
}

// makeImporterPodSpec creates and return the importer pod spec based on the passed-in endpoint, secret and pvc.
func (imp *Importer) makeImporterPodSpec() *corev1.Pod {
	pod := &corev1.Pod{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Pod",
			APIVersion: "v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      imp.PodSettings.Name,
			Namespace: imp.PodSettings.Namespace,
			Annotations: map[string]string{
				cc.AnnCreatedBy: "yes",
			},
			// Labels: map[string]string{
			//	common.CDILabelKey:        common.CDILabelValue,
			//	common.CDIComponentLabel:  common.ImporterPodNamePrefix,
			//	common.PrometheusLabelKey: common.PrometheusLabelValue,
			// },
			OwnerReferences: []metav1.OwnerReference{
				imp.PodSettings.OwnerReference,
			},
		},
		Spec: corev1.PodSpec{
			// Container and volumes will be added later.
			Containers:    []corev1.Container{},
			Volumes:       []corev1.Volume{},
			RestartPolicy: corev1.RestartPolicyOnFailure,
			// NodeSelector:      args.workloadNodePlacement.NodeSelector,
			// Tolerations:       args.workloadNodePlacement.Tolerations,
			// Affinity:          args.workloadNodePlacement.Affinity,
			PriorityClassName: imp.PodSettings.PriorityClassName,
			ImagePullSecrets:  imp.PodSettings.ImagePullSecrets,
		},
	}

	cc.SetRecommendedLabels(pod, imp.PodSettings.InstallerLabels, imp.PodSettings.ControllerName)
	cc.SetRestrictedSecurityContext(&pod.Spec)

	if imp.pvcName != "" {
		pod.Spec.Volumes = []corev1.Volume{
			{
				Name: "volume",
				VolumeSource: corev1.VolumeSource{
					PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{
						ClaimName: imp.pvcName,
					},
				},
			},
		}
	}

	// if imp.pvcNamespace != "" {
	// 	pod.ObjectMeta.Namespace = imp.pvcNamespace
	// }

	container := imp.makeImporterContainerSpec()
	imp.addVolumes(pod, container)
	pod.Spec.Containers = append(pod.Spec.Containers, *container)

	return pod
}

func (imp *Importer) makeImporterContainerSpec() *corev1.Container {
	container := &corev1.Container{
		Name:            common.ImporterContainerName,
		Image:           imp.PodSettings.Image,
		ImagePullPolicy: corev1.PullPolicy(imp.PodSettings.PullPolicy),
		Command:         []string{"sh"},
		Args:            []string{"/importer_entrypoint.sh", "-v=" + imp.EnvSettings.Verbose},
		Ports: []corev1.ContainerPort{
			{
				Name:          "metrics",
				ContainerPort: 8443,
				Protocol:      corev1.ProtocolTCP,
			},
		},
		Env: imp.makeImporterContainerEnv(),
	}

	if imp.PodSettings.ResourceRequirements != nil {
		container.Resources = *imp.PodSettings.ResourceRequirements
	}

	if imp.pvcName != "" {
		container.VolumeDevices = []corev1.VolumeDevice{
			{
				Name:       "volume",
				DevicePath: "/dev/xvda",
			},
		}
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
					Key: common.KeyAccess,
				},
			},
		}, corev1.EnvVar{
			Name: common.ImporterSecretKey,
			ValueFrom: &corev1.EnvVarSource{
				SecretKeyRef: &corev1.SecretKeySelector{
					LocalObjectReference: corev1.LocalObjectReference{
						Name: imp.EnvSettings.SecretName,
					},
					Key: common.KeySecret,
				},
			},
		})
	}

	return env
}

// addVolumes fills Volumes in Pod spec and VolumeMounts and envs in container spec.
func (imp *Importer) addVolumes(pod *corev1.Pod, container *corev1.Container) {
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

	// Mount extra headers Secrets.
	for index, header := range imp.EnvSettings.SecretExtraHeaders {
		volName := fmt.Sprintf(secretExtraHeadersVolumeName, index)
		mountPath := path.Join(common.ImporterSecretExtraHeadersDir, fmt.Sprint(index))
		envName := fmt.Sprintf("%s%d", common.ImporterExtraHeader, index)
		podutil.AddVolume(pod, container,
			podutil.CreateSecretVolume(volName, header),
			podutil.CreateVolumeMount(volName, mountPath),
			corev1.EnvVar{
				Name:  envName,
				Value: header,
			},
		)
	}
}

type PodNamer interface {
	ImporterPod() types.NamespacedName
}

func FindPod(ctx context.Context, client client.Client, name PodNamer) (*corev1.Pod, error) {
	return helper.FetchObject(ctx, name.ImporterPod(), client, &corev1.Pod{})
}
