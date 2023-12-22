package uploader

import (
	"context"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	"sigs.k8s.io/controller-runtime/pkg/client"

	cc "github.com/deckhouse/virtualization-controller/pkg/controller/common"
	"github.com/deckhouse/virtualization-controller/pkg/sdk/framework/helper"
)

type ServiceSettings struct {
	Name           string
	Namespace      string
	OwnerReference metav1.OwnerReference
}

type Service struct {
	Settings *ServiceSettings
}

func NewService(settings *ServiceSettings) *Service {
	return &Service{settings}
}

func (s *Service) Create(ctx context.Context, client client.Client) (*corev1.Service, error) {
	service := s.makeSpec()

	if err := client.Create(ctx, service); err != nil {
		return nil, err
	}

	return service, nil
}

func CleanupService(ctx context.Context, client client.Client, service *corev1.Service) error {
	return helper.CleanupObject(ctx, client, service)
}

func (s *Service) makeSpec() *corev1.Service {
	service := &corev1.Service{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Service",
			APIVersion: "v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      s.Settings.Name,
			Namespace: s.Settings.Namespace,
			Annotations: map[string]string{
				cc.AnnCreatedBy: "yes",
			},
			Labels: map[string]string{
				// TODO add labels
			},
			OwnerReferences: []metav1.OwnerReference{
				s.Settings.OwnerReference,
			},
		},
		Spec: corev1.ServiceSpec{
			Ports: []corev1.ServicePort{
				{
					Name:     "uploader",
					Protocol: "TCP",
					Port:     443,
					TargetPort: intstr.IntOrString{
						Type:   intstr.Int,
						IntVal: 8444,
					},
				},
			},
			Selector: map[string]string{
				cc.UploaderServiceLabel: s.Settings.Name,
			},
			Type:      corev1.ServiceTypeClusterIP,
			ClusterIP: corev1.ClusterIPNone,
		},
	}

	return service
}

func FindService(ctx context.Context, client client.Client, objName types.NamespacedName) (*corev1.Service, error) {
	return helper.FetchObject(ctx, objName, client, &corev1.Service{})
}
