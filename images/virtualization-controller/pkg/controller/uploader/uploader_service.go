package uploader

import (
	"context"
	"errors"

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
		},
	}

	return service
}

var ErrServiceNameNotFound = errors.New("service name not found")

func FindService(ctx context.Context, client client.Client, obj metav1.Object) (*corev1.Service, error) {
	// Extract namespace and name of the importer Pod from annotations.
	serviceName := obj.GetAnnotations()[cc.AnnUploadServiceName]
	if serviceName == "" {
		return nil, ErrServiceNameNotFound
	}

	// Get namespace from annotations (for cluster-wide resources, e.g. ClusterVirtualMachineImage).
	// Default is namespace of the input object.
	serviceNS := obj.GetAnnotations()[cc.AnnUploaderNamespace]
	if serviceNS == "" {
		serviceNS = obj.GetNamespace()
	}

	objName := types.NamespacedName{Name: serviceName, Namespace: serviceNS}

	return helper.FetchObject(ctx, objName, client, &corev1.Service{})
}
