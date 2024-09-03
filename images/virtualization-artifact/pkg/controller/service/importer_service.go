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
	"fmt"

	corev1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/deckhouse/virtualization-controller/pkg/common/datasource"
	"github.com/deckhouse/virtualization-controller/pkg/controller/importer"
	"github.com/deckhouse/virtualization-controller/pkg/controller/supplements"
	"github.com/deckhouse/virtualization-controller/pkg/dvcr"
)

type ImporterService struct {
	dvcrSettings   *dvcr.Settings
	client         client.Client
	image          string
	requirements   corev1.ResourceRequirements
	pullPolicy     string
	verbose        string
	controllerName string
	protection     *ProtectionService
}

func NewImporterService(
	dvcrSettings *dvcr.Settings,
	client client.Client,
	image string,
	requirements corev1.ResourceRequirements,
	pullPolicy string,
	verbose string,
	controllerName string,
	protection *ProtectionService,
) *ImporterService {
	return &ImporterService{
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

func (s ImporterService) Start(ctx context.Context, settings *importer.Settings, obj ObjectKind, sup *supplements.Generator, caBundle *datasource.CABundle) error {
	ownerRef := metav1.NewControllerRef(obj, obj.GroupVersionKind())
	settings.Verbose = s.verbose

	pod, err := importer.NewImporter(s.getPodSettings(ownerRef, sup), settings, "", "").CreatePod(ctx, s.client)
	if err != nil && !k8serrors.IsAlreadyExists(err) {
		return err
	}

	return supplements.EnsureForPod(ctx, s.client, sup, pod, caBundle, s.dvcrSettings)
}

func (s ImporterService) StartFromPVC(ctx context.Context, settings *importer.Settings, obj ObjectKind, sup *supplements.Generator, caBundle *datasource.CABundle, pvcName string, pvcNamespace string) error {
	ownerRef := metav1.NewControllerRef(obj, obj.GroupVersionKind())
	settings.Verbose = s.verbose

	pod, err := importer.NewImporter(s.getPodSettings(ownerRef, sup), settings, pvcName, pvcNamespace).CreatePod(ctx, s.client)
	if err != nil && !k8serrors.IsAlreadyExists(err) {
		return err
	}

	return supplements.EnsureForPod(ctx, s.client, sup, pod, caBundle, s.dvcrSettings)
}

func (s ImporterService) CleanUp(ctx context.Context, sup *supplements.Generator) (bool, error) {
	return s.CleanUpSupplements(ctx, sup)
}

func (s ImporterService) CleanUpSupplements(ctx context.Context, sup *supplements.Generator) (bool, error) {
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

func (s ImporterService) Protect(ctx context.Context, pod *corev1.Pod) error {
	err := s.protection.AddProtection(ctx, pod)
	if err != nil {
		return fmt.Errorf("failed to add protection for importer's supplements: %w", err)
	}

	return nil
}

func (s ImporterService) Unprotect(ctx context.Context, pod *corev1.Pod) error {
	err := s.protection.RemoveProtection(ctx, pod)
	if err != nil {
		return fmt.Errorf("failed to remove protection for importer's supplements: %w", err)
	}

	return nil
}

func (s ImporterService) GetPod(ctx context.Context, sup *supplements.Generator) (*corev1.Pod, error) {
	pod, err := importer.FindPod(ctx, s.client, sup)
	if err != nil {
		return nil, err
	}

	return pod, nil
}

func (s ImporterService) getPodSettings(ownerRef *metav1.OwnerReference, sup *supplements.Generator) *importer.PodSettings {
	importerPod := sup.ImporterPod()
	return &importer.PodSettings{
		Name:                 importerPod.Name,
		Namespace:            importerPod.Namespace,
		Image:                s.image,
		PullPolicy:           s.pullPolicy,
		OwnerReference:       *ownerRef,
		ControllerName:       s.controllerName,
		InstallerLabels:      map[string]string{},
		ResourceRequirements: &s.requirements,
	}
}
