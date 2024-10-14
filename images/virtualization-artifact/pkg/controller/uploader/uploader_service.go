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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/deckhouse/virtualization-controller/pkg/common"
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
			Labels: map[string]string{},
			OwnerReferences: []metav1.OwnerReference{
				s.Settings.OwnerReference,
			},
		},
		Spec: corev1.ServiceSpec{
			Ports: []corev1.ServicePort{
				{
					Name:     common.UploaderPortName,
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
			Type: corev1.ServiceTypeClusterIP,
		},
	}

	return service
}

type ServiceNamer interface {
	UploaderService() types.NamespacedName
}

func FindService(ctx context.Context, client client.Client, name ServiceNamer) (*corev1.Service, error) {
	return helper.FetchObject(ctx, name.UploaderService(), client, &corev1.Service{})
}
