package factory

import (
	corev1 "k8s.io/api/core/v1"
	netv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/deckhouse/virtualization-controller/pkg/common/annotations"
	"github.com/deckhouse/virtualization-controller/pkg/controller/supplements"
)

const (
	destinationAuthVol = "dvcr-secret-vol"
	exporterPortName   = "exporter"
	exporterPort       = 8444
)

type Factory interface {
	Pod() *corev1.Pod
	Service() *corev1.Service
	Ingress() *netv1.Ingress
}

type defaultFactory struct {
	sup    *supplements.Generator
	parent client.Object

	controllerName string
	image          string
	dvcrAuthSecret string

	host string

	priorityClassName    string
	pullPolicy           corev1.PullPolicy
	imagePullSecrets     []corev1.LocalObjectReference
	resourceRequirements *corev1.ResourceRequirements
	extraLabels          map[string]string
	tolerations          []corev1.Toleration

	className     *string
	tlsSecretName *string
}

func (d defaultFactory) makeOwnerReference() metav1.OwnerReference {
	return *metav1.NewControllerRef(d.parent, d.parent.GetObjectKind().GroupVersionKind())
}

func (d defaultFactory) podSelector() map[string]string {
	return map[string]string{
		annotations.LabelVirtualDataExportUID: string(d.parent.GetUID()),
	}
}
