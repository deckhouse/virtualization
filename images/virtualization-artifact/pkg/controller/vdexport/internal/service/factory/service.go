package factory

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"

	"github.com/deckhouse/virtualization-controller/pkg/common/annotations"
)

func (d defaultFactory) Service() *corev1.Service {
	service := &corev1.Service{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Service",
			APIVersion: "v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      d.sup.ExporterService().Name,
			Namespace: d.sup.ExporterService().Namespace,
			Annotations: map[string]string{
				annotations.AnnCreatedBy: "yes",
			},
			Labels: map[string]string{},
			OwnerReferences: []metav1.OwnerReference{
				d.makeOwnerReference(),
			},
		},
		Spec: corev1.ServiceSpec{
			Ports: []corev1.ServicePort{
				{
					Name:     exporterPortName,
					Protocol: "TCP",
					Port:     80,
					TargetPort: intstr.IntOrString{
						Type:   intstr.String,
						StrVal: exporterPortName,
					},
				},
			},
			Selector: d.podSelector(),
			Type:     corev1.ServiceTypeClusterIP,
		},
	}

	return service
}
