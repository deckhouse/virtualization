package importer

import (
	"context"
	"fmt"
	"path"
	"strconv"

	corev1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	common "github.com/deckhouse/virtualization-controller/pkg/common"
	podutil "github.com/deckhouse/virtualization-controller/pkg/common/pod"
	cc "github.com/deckhouse/virtualization-controller/pkg/controller/common"
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
)

type Importer struct {
	PodSettings *PodSettings
	Settings    *Settings
	CABundle    *CABundleSettings
}

func NewImporter(podSettings *PodSettings, settings *Settings, caBundleSettings *CABundleSettings) *Importer {
	return &Importer{
		PodSettings: podSettings,
		Settings:    settings,
		CABundle:    caBundleSettings,
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

func (imp *Importer) EnsureCABundleConfigMap(ctx context.Context, client client.Client, pod *corev1.Pod) error {
	if imp.CABundle == nil {
		return nil
	}

	cm := makeCABundleConfigMap(pod, imp.CABundle)

	cc.SetRecommendedLabels(cm, imp.PodSettings.InstallerLabels, imp.PodSettings.ControllerName)

	err := client.Create(ctx, cm)
	if err != nil && !k8serrors.IsAlreadyExists(err) {
		return fmt.Errorf("create ConfigMap/%s with certs from dataSource.http.caBundle: %w", cm.Name, err)
	}

	return nil
}

// CleanupPod deletes importer Pod.
// No need to delete CABundle configmap. It has ownerRef and will be gc'ed.
func CleanupPod(ctx context.Context, client client.Client, pod *corev1.Pod) error {
	if pod == nil {
		return nil
	}

	if err := client.Delete(ctx, pod); cc.IgnoreNotFound(err) != nil {
		return err
	}

	return nil
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

	if imp.CABundle != nil {
		pod.ObjectMeta.Annotations[cc.AnnCABundleConfigMap] = imp.CABundle.ConfigMapName
	}

	cc.SetRecommendedLabels(pod, imp.PodSettings.InstallerLabels, imp.PodSettings.ControllerName)
	cc.SetRestrictedSecurityContext(&pod.Spec)

	container := imp.makeImporterContainerSpec()
	imp.addVolumes(pod, container)
	pod.Spec.Containers = append(pod.Spec.Containers, *container)

	return pod
}

func (imp *Importer) makeImporterContainerSpec() *corev1.Container {
	container := &corev1.Container{
		Name:            common.ImporterPodNamePrefix,
		Image:           imp.PodSettings.Image,
		ImagePullPolicy: corev1.PullPolicy(imp.PodSettings.PullPolicy),
		Command:         []string{"sh"},
		Args:            []string{"/entrypoint.sh", "-v=" + imp.Settings.Verbose},
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

	return container
}

// makeImporterEnvs returns the Env portion for the importer container.
func (imp *Importer) makeImporterContainerEnv() []corev1.EnvVar {
	env := []corev1.EnvVar{
		{
			Name:  common.ImporterSource,
			Value: imp.Settings.Source,
		},
		{
			Name:  common.ImporterEndpoint,
			Value: imp.Settings.Endpoint,
		},
		{
			Name:  common.ImporterContentType,
			Value: imp.Settings.ContentType,
		},
		{
			Name:  common.ImporterImageSize,
			Value: imp.Settings.ImageSize,
		},
		{
			Name:  common.OwnerUID,
			Value: string(imp.PodSettings.OwnerReference.UID),
		},
		{
			Name:  common.FilesystemOverheadVar,
			Value: imp.Settings.FilesystemOverhead,
		},
		{
			Name:  common.InsecureTLSVar,
			Value: strconv.FormatBool(imp.Settings.InsecureTLS),
		},
		{
			Name:  common.ImporterDiskID,
			Value: imp.Settings.DiskID,
		},
		{
			Name:  common.ImporterUUID,
			Value: imp.Settings.UUID,
		},
		{
			Name:  common.ImporterReadyFile,
			Value: imp.Settings.ReadyFile,
		},
		{
			Name:  common.ImporterDoneFile,
			Value: imp.Settings.DoneFile,
		},
		{
			Name:  common.ImporterBackingFile,
			Value: imp.Settings.BackingFile,
		},
		{
			Name:  common.ImporterThumbprint,
			Value: imp.Settings.Thumbprint,
		},
		{
			Name:  common.ImportProxyHTTP,
			Value: imp.Settings.HTTPProxy,
		},
		{
			Name:  common.ImportProxyHTTPS,
			Value: imp.Settings.HTTPSProxy,
		},
		{
			Name:  common.ImportProxyNoProxy,
			Value: imp.Settings.NoProxy,
		},
	}

	// Destination settings: endpoint, insecure flag.
	env = append(env, []corev1.EnvVar{
		{
			Name:  common.ImporterDestinationEndpoint,
			Value: imp.Settings.DestinationEndpoint,
		},
		{
			Name:  common.DestinationInsecureTLSVar,
			Value: imp.Settings.DestinationInsecureTLS,
		},
	}...)

	// HTTP source checksum settings: md5 and sha256.
	if imp.Settings.SHA256 != "" {
		env = append(env, corev1.EnvVar{
			Name:  common.ImporterSHA256Sum,
			Value: imp.Settings.SHA256,
		})
	}
	if imp.Settings.MD5 != "" {
		env = append(env, corev1.EnvVar{
			Name:  common.ImporterMD5Sum,
			Value: imp.Settings.MD5,
		})
	}

	// Pass basic auth configuration from Secret with downward API.
	if imp.Settings.SecretName != "" {
		env = append(env, corev1.EnvVar{
			Name: common.ImporterAccessKeyID,
			ValueFrom: &corev1.EnvVarSource{
				SecretKeyRef: &corev1.SecretKeySelector{
					LocalObjectReference: corev1.LocalObjectReference{
						Name: imp.Settings.SecretName,
					},
					Key: common.KeyAccess,
				},
			},
		}, corev1.EnvVar{
			Name: common.ImporterSecretKey,
			ValueFrom: &corev1.EnvVarSource{
				SecretKeyRef: &corev1.SecretKeySelector{
					LocalObjectReference: corev1.LocalObjectReference{
						Name: imp.Settings.SecretName,
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
	if imp.Settings.DestinationAuthSecret != "" {
		// Mount DVCR auth Secret and pass directory with mounted DVCR login config.
		podutil.AddVolume(pod, container,
			podutil.CreateSecretVolume(destinationAuthVol, imp.Settings.DestinationAuthSecret),
			podutil.CreateVolumeMount(destinationAuthVol, common.ImporterDestinationAuthConfigDir),
			corev1.EnvVar{
				Name:  common.ImporterDestinationAuthConfigVar,
				Value: common.ImporterDestinationAuthConfigFile,
			},
		)
	}

	// Volume with CA certificates either from caBundle field or from existing ConfigMap.
	if imp.Settings.CertConfigMap != "" {
		podutil.AddVolume(pod, container,
			podutil.CreateConfigMapVolume(certVolName, imp.Settings.CertConfigMap),
			podutil.CreateVolumeMount(certVolName, common.ImporterCertDir),
			corev1.EnvVar{
				Name:  common.ImporterCertDirVar,
				Value: common.ImporterCertDir,
			},
		)
	} else if imp.CABundle != nil {
		podutil.AddVolume(pod, container,
			podutil.CreateConfigMapVolume(caBundleVolName, imp.CABundle.ConfigMapName),
			podutil.CreateVolumeMount(caBundleVolName, common.ImporterCABundleDir),
			corev1.EnvVar{
				Name:  common.ImporterCertDirVar,
				Value: common.ImporterCABundleDir,
			},
		)
	}

	if imp.Settings.CertConfigMapProxy != "" {
		podutil.AddVolume(pod, container,
			podutil.CreateConfigMapVolume(proxyCertVolName, imp.Settings.CertConfigMapProxy), //  GetImportProxyConfigMapName(args.cvmi.Name)
			podutil.CreateVolumeMount(proxyCertVolName, common.ImporterProxyCertDir),
			corev1.EnvVar{
				Name:  common.ImporterProxyCertDirVar,
				Value: common.ImporterProxyCertDir,
			},
		)
	}

	// Mount extra headers Secrets.
	for index, header := range imp.Settings.SecretExtraHeaders {
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

// GetImportProxyConfigMapName return prefixed name.
// TODO add validation against name limitations
func GetImportProxyConfigMapName(suffix string) string {
	return fmt.Sprintf("import-proxy-cm-%s", suffix)
}

func GetDestinationImageNameFromPod(pod *corev1.Pod) string {
	if pod == nil || len(pod.Spec.Containers) == 0 {
		return ""
	}

	for _, envVar := range pod.Spec.Containers[0].Env {
		if envVar.Name == common.ImporterDestinationEndpoint {
			return envVar.Value
		}
	}

	return ""
}
