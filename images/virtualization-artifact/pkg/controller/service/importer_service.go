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
	netv1 "k8s.io/api/networking/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/deckhouse/virtualization-controller/pkg/common/annotations"
	"github.com/deckhouse/virtualization-controller/pkg/common/datasource"
	networkpolicy "github.com/deckhouse/virtualization-controller/pkg/common/network_policy"
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

func (s ImporterService) Start(
	ctx context.Context,
	settings *importer.Settings,
	obj client.Object,
	sup supplements.Generator,
	caBundle *datasource.CABundle,
	opts ...Option,
) error {
	options := newGenericOptions(opts...)
	ownerRef := metav1.NewControllerRef(obj, obj.GetObjectKind().GroupVersionKind())
	settings.Verbose = s.verbose

	podSettings := s.getPodSettings(ownerRef, sup)
	if options.nodePlacement != nil {
		podSettings.NodePlacement = options.nodePlacement
	}

	pod, err := importer.NewImporter(podSettings, settings).GetOrCreatePod(ctx, s.client)
	if err != nil {
		return err
	}

	err = networkpolicy.CreateNetworkPolicy(ctx, s.client, pod, sup, s.protection.GetFinalizer())
	if err != nil {
		return fmt.Errorf("failed to create NetworkPolicy: %w", err)
	}

	return supplements.EnsureForPod(ctx, s.client, sup, pod, caBundle, s.dvcrSettings)
}

func (s ImporterService) StartWithPodSetting(
	ctx context.Context,
	settings *importer.Settings,
	sup supplements.Generator,
	caBundle *datasource.CABundle,
	podSettings *importer.PodSettings,
	opts ...Option,
) error {
	options := newGenericOptions(opts...)
	settings.Verbose = s.verbose

	podSettings.Finalizer = s.protection.finalizer
	if options.nodePlacement != nil {
		podSettings.NodePlacement = options.nodePlacement
	}

	pod, err := importer.NewImporter(podSettings, settings).GetOrCreatePod(ctx, s.client)
	if err != nil {
		return err
	}

	return supplements.EnsureForPod(ctx, s.client, sup, pod, caBundle, s.dvcrSettings)
}

func (s ImporterService) CleanUp(ctx context.Context, sup supplements.Generator) (bool, error) {
	return s.CleanUpSupplements(ctx, sup)
}

func (s ImporterService) DeletePod(ctx context.Context, obj client.Object, controllerName string, sup supplements.Generator) (bool, error) {
	labelSelector := client.MatchingLabels{annotations.AppKubernetesManagedByLabel: controllerName}

	podList := &corev1.PodList{}
	if err := s.client.List(ctx, podList, labelSelector); err != nil {
		return false, err
	}

	for _, pod := range podList.Items {
		for _, ownerRef := range pod.OwnerReferences {
			if ownerRef.Kind == obj.GetObjectKind().GroupVersionKind().Kind && ownerRef.Name == obj.GetName() && ownerRef.UID == obj.GetUID() {
				networkPolicy, err := networkpolicy.GetNetworkPolicyFromObject(ctx, s.client, &pod, sup)
				if err != nil {
					return false, err
				}

				if networkPolicy != nil {
					err = s.client.Delete(ctx, networkPolicy)
					if err != nil && !k8serrors.IsNotFound(err) {
						return false, err
					}
				}

				err = s.protection.RemoveProtection(ctx, &pod)
				if err != nil {
					return false, err
				}

				err = s.client.Delete(ctx, &pod)
				if err != nil {
					if k8serrors.IsNotFound(err) {
						return false, nil
					}

					return false, err
				}

				return true, nil
			}
		}
	}

	return false, nil
}

func (s ImporterService) CleanUpSupplements(ctx context.Context, sup supplements.Generator) (bool, error) {
	networkPolicy, err := networkpolicy.GetNetworkPolicy(ctx, s.client, sup.LegacyImporterPod(), sup)
	if err != nil {
		return false, err
	}

	if networkPolicy != nil {
		err = s.protection.RemoveProtection(ctx, networkPolicy)
		if err != nil {
			return false, err
		}

		err = s.client.Delete(ctx, networkPolicy)
		if err != nil && !k8serrors.IsNotFound(err) {
			return false, err
		}
	}

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

func (s ImporterService) Protect(ctx context.Context, pod *corev1.Pod, sup supplements.Generator) (err error) {
	var networkPolicy *netv1.NetworkPolicy

	if pod != nil {
		networkPolicy, err = networkpolicy.GetNetworkPolicyFromObject(ctx, s.client, pod, sup)
		if err != nil {
			return fmt.Errorf("failed to get networkPolicy for importer's supplements protection: %w", err)
		}
	}

	err = s.protection.AddProtection(ctx, networkPolicy, pod)
	if err != nil {
		return fmt.Errorf("failed to add protection for importer's supplements: %w", err)
	}

	return nil
}

func (s ImporterService) Unprotect(ctx context.Context, pod *corev1.Pod, sup supplements.Generator) (err error) {
	var networkPolicy *netv1.NetworkPolicy

	if pod != nil {
		networkPolicy, err = networkpolicy.GetNetworkPolicyFromObject(ctx, s.client, pod, sup)
		if err != nil {
			return fmt.Errorf("failed to get networkPolicy for removing importer's supplements protection: %w", err)
		}
	}

	err = s.protection.RemoveProtection(ctx, networkPolicy, pod)
	if err != nil {
		return fmt.Errorf("failed to remove protection for importer's supplements: %w", err)
	}

	return nil
}

func (s ImporterService) GetPod(ctx context.Context, sup supplements.Generator) (*corev1.Pod, error) {
	return supplements.FetchSupplement(ctx, s.client, sup, supplements.SupplementImporterPod, &corev1.Pod{})
}

func (s ImporterService) getPodSettings(ownerRef *metav1.OwnerReference, sup supplements.Generator) *importer.PodSettings {
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
		Finalizer:            s.protection.GetFinalizer(),
	}
}

func (s ImporterService) GetPodSettingsWithPVC(ownerRef *metav1.OwnerReference, sup supplements.Generator, pvcName, pvcNamespace string) *importer.PodSettings {
	importerPod := sup.ImporterPod()
	return &importer.PodSettings{
		Name:                 importerPod.Name,
		Namespace:            pvcNamespace,
		Image:                s.image,
		PullPolicy:           s.pullPolicy,
		OwnerReference:       *ownerRef,
		ControllerName:       s.controllerName,
		InstallerLabels:      map[string]string{},
		ResourceRequirements: &s.requirements,
		PVCName:              pvcName,
	}
}
