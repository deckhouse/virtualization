package importer

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	podutil "github.com/deckhouse/virtualization-controller/pkg/common/pod"
	cc "github.com/deckhouse/virtualization-controller/pkg/controller/common"
)

type CABundleSettings struct {
	CABundle      string
	ConfigMapName string
}

func NewCABundleSettings(caBundle, caBundleConfigMapName string) *CABundleSettings {
	if caBundle == "" {
		return nil
	}
	return &CABundleSettings{
		CABundle:      caBundle,
		ConfigMapName: caBundleConfigMapName,
	}
}

// PrepareCABundleConfigMap returns a ConfigMap resource with certificates from caBundle string.
// ConfigMap is used for VirtualMachineDisk to be compatible with CDI DataVolume.
func makeCABundleConfigMap(pod *corev1.Pod, settings *CABundleSettings) *corev1.ConfigMap {
	return &corev1.ConfigMap{
		TypeMeta: metav1.TypeMeta{
			Kind:       "ConfigMap",
			APIVersion: "v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Namespace: pod.Namespace,
			Name:      settings.ConfigMapName,
			Annotations: map[string]string{
				cc.AnnCreatedByImporter: "yes",
			},
			Labels: map[string]string{},
			OwnerReferences: []metav1.OwnerReference{
				podutil.MakeOwnerReference(pod),
			},
		},
		Data: map[string]string{
			settings.ConfigMapName + ".pem": settings.CABundle,
		},
	}
}
