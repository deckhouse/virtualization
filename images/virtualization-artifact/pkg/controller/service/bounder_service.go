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
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/deckhouse/virtualization-controller/pkg/controller/bounder"
	"github.com/deckhouse/virtualization-controller/pkg/controller/supplements"
	"github.com/deckhouse/virtualization-controller/pkg/dvcr"
)

type BounderPodService struct {
	dvcrSettings   *dvcr.Settings
	client         client.Client
	image          string
	requirements   corev1.ResourceRequirements
	pullPolicy     string
	verbose        string
	controllerName string
	protection     *ProtectionService
}

func NewBounderPodService(
	dvcrSettings *dvcr.Settings,
	client client.Client,
	image string,
	requirements corev1.ResourceRequirements,
	pullPolicy string,
	verbose string,
	controllerName string,
	protection *ProtectionService,
) *BounderPodService {
	return &BounderPodService{
		dvcrSettings:   dvcrSettings,
		client:         client,
		image:          image,
		requirements:   requirements,
		pullPolicy:     pullPolicy,
		verbose:        verbose,
		controllerName: controllerName,
		protection:     protection,
	}
}

func (s BounderPodService) Start(ctx context.Context, ownerRef *metav1.OwnerReference, sup supplements.Generator, opts ...Option) error {
	options := newGenericOptions(opts...)

	podSettings := s.GetPodSettings(ownerRef, sup)

	if options.nodePlacement != nil {
		podSettings.NodePlacement = options.nodePlacement
	}

	_, err := bounder.NewBounder(podSettings).CreatePod(ctx, s.client)
	if err != nil && !k8serrors.IsAlreadyExists(err) {
		return err
	}

	return nil
}

func (s BounderPodService) CleanUp(ctx context.Context, sup supplements.Generator) (bool, error) {
	return s.CleanUpSupplements(ctx, sup)
}

func (s BounderPodService) CleanUpSupplements(ctx context.Context, sup supplements.Generator) (bool, error) {
	pod, err := s.GetPod(ctx, sup)
	if err != nil {
		return false, err
	}

	err = s.protection.RemoveProtection(ctx, pod)
	if err != nil {
		return false, err
	}

	var hasDeleted bool

	if pod != nil {
		hasDeleted = true
		err = s.client.Delete(ctx, pod)
		if err != nil && !k8serrors.IsNotFound(err) {
			return false, err
		}
	}

	return hasDeleted, nil
}

func (s BounderPodService) GetPod(ctx context.Context, sup supplements.Generator) (*corev1.Pod, error) {
	return supplements.FetchSupplement(ctx, s.client, sup, supplements.SupplementBounderPod, &corev1.Pod{})
}

func (s BounderPodService) GetPodSettings(ownerRef *metav1.OwnerReference, sup supplements.Generator) *bounder.PodSettings {
	bounderPod := sup.BounderPod()
	return &bounder.PodSettings{
		Name:                 bounderPod.Name,
		Namespace:            bounderPod.Namespace,
		Image:                s.image,
		PullPolicy:           s.pullPolicy,
		OwnerReference:       *ownerRef,
		ControllerName:       s.controllerName,
		InstallerLabels:      map[string]string{},
		ResourceRequirements: &s.requirements,
		PVCName:              sup.PersistentVolumeClaim().Name,
		Finalizer:            s.protection.GetFinalizer(),
	}
}
