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

package service

import (
	"context"

	corev1 "k8s.io/api/core/v1"
	netv1 "k8s.io/api/networking/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/deckhouse/virtualization-controller/pkg/common/datasource"
	"github.com/deckhouse/virtualization-controller/pkg/controller/supplements"
	"github.com/deckhouse/virtualization-controller/pkg/controller/uploader"
	"github.com/deckhouse/virtualization-controller/pkg/dvcr"
)

type UploaderService struct {
	dvcrSettings   *dvcr.Settings
	client         client.Client
	image          string
	pullPolicy     string
	verbose        string
	controllerName string
	protection     *ProtectionService
}

func NewUploaderService(
	dvcrSettings *dvcr.Settings,
	client client.Client,
	image string,
	pullPolicy string,
	verbose string,
	controllerName string,
	protection *ProtectionService,
) *UploaderService {
	return &UploaderService{
		dvcrSettings:   dvcrSettings,
		client:         client,
		image:          image,
		pullPolicy:     pullPolicy,
		verbose:        verbose,
		controllerName: controllerName,
		protection:     protection,
	}
}

func (s UploaderService) Start(ctx context.Context, settings *uploader.Settings, obj ObjectKind, sup *supplements.Generator, caBundle *datasource.CABundle) error {
	ownerRef := metav1.NewControllerRef(obj, obj.GroupVersionKind())
	settings.Verbose = s.verbose

	pod, err := uploader.NewPod(s.getPodSettings(ownerRef, sup), settings).Create(ctx, s.client)
	if err != nil && !k8serrors.IsAlreadyExists(err) {
		return err
	}

	err = supplements.EnsureForPod(ctx, s.client, sup, pod, caBundle, s.dvcrSettings)
	if err != nil {
		return err
	}

	_, err = uploader.NewService(s.getServiceSettings(ownerRef, sup)).Create(ctx, s.client)
	if err != nil && !k8serrors.IsAlreadyExists(err) {
		return err
	}

	ing, err := uploader.NewIngress(s.getIngressSettings(ownerRef, sup)).Create(ctx, s.client)
	if err != nil && !k8serrors.IsAlreadyExists(err) {
		return err
	}

	return supplements.EnsureForIngress(ctx, s.client, sup, ing, s.dvcrSettings)
}

func (s UploaderService) CleanUp(ctx context.Context, sup *supplements.Generator) (bool, error) {
	return s.CleanUpSupplements(ctx, sup)
}

func (s UploaderService) CleanUpSupplements(ctx context.Context, sup *supplements.Generator) (bool, error) {
	pod, err := s.GetPod(ctx, sup)
	if err != nil {
		return false, err
	}
	svc, err := s.GetService(ctx, sup)
	if err != nil {
		return false, err
	}
	ing, err := s.GetIngress(ctx, sup)
	if err != nil {
		return false, err
	}

	err = s.protection.RemoveProtection(ctx, pod, svc, ing)
	if err != nil {
		return false, err
	}

	var haveDeleted bool

	if pod != nil {
		haveDeleted = true
		err = s.client.Delete(ctx, pod)
		if err != nil && !k8serrors.IsNotFound(err) {
			return false, err
		}
	}

	if svc != nil {
		haveDeleted = true
		err = s.client.Delete(ctx, svc)
		if err != nil && !k8serrors.IsNotFound(err) {
			return false, err
		}
	}

	if ing != nil {
		haveDeleted = true
		err = s.client.Delete(ctx, ing)
		if err != nil && !k8serrors.IsNotFound(err) {
			return false, err
		}
	}

	return haveDeleted, nil
}

func (s UploaderService) Protect(ctx context.Context, pod *corev1.Pod, svc *corev1.Service, ing *netv1.Ingress) error {
	return s.protection.AddProtection(ctx, pod, svc, ing)
}

func (s UploaderService) Unprotect(ctx context.Context, pod *corev1.Pod, svc *corev1.Service, ing *netv1.Ingress) error {
	return s.protection.RemoveProtection(ctx, pod, svc, ing)
}

func (s UploaderService) GetPod(ctx context.Context, sup *supplements.Generator) (*corev1.Pod, error) {
	pod, err := uploader.FindPod(ctx, s.client, sup)
	if err != nil {
		return nil, err
	}

	return pod, nil
}

func (s UploaderService) GetService(ctx context.Context, sup *supplements.Generator) (*corev1.Service, error) {
	svc, err := uploader.FindService(ctx, s.client, sup)
	if err != nil {
		return nil, err
	}

	return svc, nil
}

func (s UploaderService) GetIngress(ctx context.Context, sup *supplements.Generator) (*netv1.Ingress, error) {
	ing, err := uploader.FindIngress(ctx, s.client, sup)
	if err != nil {
		return nil, err
	}

	return ing, nil
}

func (s UploaderService) getPodSettings(ownerRef *metav1.OwnerReference, sup *supplements.Generator) *uploader.PodSettings {
	uploaderPod := sup.UploaderPod()
	uploaderSvc := sup.UploaderService()
	return &uploader.PodSettings{
		Name:            uploaderPod.Name,
		Image:           s.image,
		PullPolicy:      s.pullPolicy,
		Namespace:       uploaderPod.Namespace,
		OwnerReference:  *ownerRef,
		ControllerName:  s.controllerName,
		InstallerLabels: map[string]string{},
		ServiceName:     uploaderSvc.Name,
	}
}

func (s UploaderService) getServiceSettings(ownerRef *metav1.OwnerReference, sup *supplements.Generator) *uploader.ServiceSettings {
	uploaderSvc := sup.UploaderService()
	return &uploader.ServiceSettings{
		Name:           uploaderSvc.Name,
		Namespace:      uploaderSvc.Namespace,
		OwnerReference: *ownerRef,
	}
}

func (s UploaderService) getIngressSettings(ownerRef *metav1.OwnerReference, sup *supplements.Generator) *uploader.IngressSettings {
	uploaderIng := sup.UploaderIngress()
	uploaderSvc := sup.UploaderService()
	secretName := s.dvcrSettings.UploaderIngressSettings.TLSSecret
	if supplements.ShouldCopyUploaderTLSSecret(s.dvcrSettings, sup) {
		secretName = sup.UploaderTLSSecretForIngress().Name
	}
	var class *string
	if c := s.dvcrSettings.UploaderIngressSettings.Class; c != "" {
		class = &c
	}
	return &uploader.IngressSettings{
		Name:           uploaderIng.Name,
		Namespace:      uploaderIng.Namespace,
		Host:           s.dvcrSettings.UploaderIngressSettings.Host,
		TLSSecretName:  secretName,
		ServiceName:    uploaderSvc.Name,
		ClassName:      class,
		OwnerReference: *ownerRef,
	}
}
