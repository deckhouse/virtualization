package uploader

import (
	"context"
	"errors"
	"fmt"
	"path"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/deckhouse/virtualization-controller/pkg/common"
	podutil "github.com/deckhouse/virtualization-controller/pkg/common/pod"
	cc "github.com/deckhouse/virtualization-controller/pkg/controller/common"
	"github.com/deckhouse/virtualization-controller/pkg/sdk/framework/helper"
)

const (
	// secretExtraHeadersVolumeName is the format string that specifies where extra HTTP header secrets will be mounted
	secretExtraHeadersVolumeName = "import-extra-headers-vol-%d"

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
}

// Create creates and returns a pointer to a pod which is created based on the passed-in endpoint, secret
// name, etc. A nil secret means the endpoint credentials are not passed to the uploader pod.
func (p *Pod) Create(ctx context.Context, client client.Client) (*corev1.Pod, error) {
	pod := p.makeSpec()

	if err := client.Create(ctx, pod); err != nil {
		return nil, err
	}

	return pod, nil
}

func CleanupPod(ctx context.Context, client client.Client, pod *corev1.Pod) error {
	return helper.CleanupObject(ctx, client, pod)
}

func (p *Pod) makeSpec() *corev1.Pod {
	pod := &corev1.Pod{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Pod",
			APIVersion: "v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      p.PodSettings.Name,
			Namespace: p.PodSettings.Namespace,
			Annotations: map[string]string{
				cc.AnnCreatedBy: "yes",
			},
			Labels: map[string]string{
				cc.UploaderServiceLabel: p.PodSettings.ServiceName,
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

	cc.SetRecommendedLabels(pod, p.PodSettings.InstallerLabels, p.PodSettings.ControllerName)
	cc.SetRestrictedSecurityContext(&pod.Spec)

	container := p.makeUploaderContainerSpec()
	p.addVolumes(pod, container)
	pod.Spec.Containers = append(pod.Spec.Containers, *container)

	return pod
}

func (p *Pod) makeUploaderContainerSpec() *corev1.Container {
	container := &corev1.Container{
		Name:            common.UploaderContainerName,
		Image:           p.PodSettings.Image,
		ImagePullPolicy: corev1.PullPolicy(p.PodSettings.PullPolicy),
		Command:         []string{"sh"},
		Args:            []string{"/uploader_entrypoint.sh", "-v=" + p.Settings.Verbose},
		Ports: []corev1.ContainerPort{
			{
				Name:          "metrics",
				ContainerPort: 8443,
				Protocol:      corev1.ProtocolTCP,
			},
		},
		Env: p.makeUploaderContainerEnv(),
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

	// Mount extra headers Secrets.
	for index, header := range p.Settings.SecretExtraHeaders {
		volName := fmt.Sprintf(secretExtraHeadersVolumeName, index)
		mountPath := path.Join(common.UploaderSecretExtraHeadersDir, fmt.Sprint(index))
		envName := fmt.Sprintf("%s%d", common.UploaderExtraHeader, index)
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

func GetDestinationImageNameFromPod(pod *corev1.Pod) string {
	if pod == nil || len(pod.Spec.Containers) == 0 {
		return ""
	}

	for _, envVar := range pod.Spec.Containers[0].Env {
		if envVar.Name == common.UploaderDestinationEndpoint {
			return envVar.Value
		}
	}

	return ""
}

var ErrPodNameNotFound = errors.New("pod name not found")

func FindPod(ctx context.Context, client client.Client, obj metav1.Object) (*corev1.Pod, error) {
	// Extract namespace and name of the importer Pod from annotations.
	podName := obj.GetAnnotations()[cc.AnnUploadPodName]
	if podName == "" {
		return nil, ErrPodNameNotFound
	}

	// Get namespace from annotations (for cluster-wide resources, e.g. ClusterVirtualMachineImage).
	// Default is namespace of the input object.
	podNS := obj.GetAnnotations()[cc.AnnUploaderNamespace]
	if podNS == "" {
		podNS = obj.GetNamespace()
	}

	objName := types.NamespacedName{Name: podName, Namespace: podNS}

	return helper.FetchObject(ctx, objName, client, &corev1.Pod{})
}
