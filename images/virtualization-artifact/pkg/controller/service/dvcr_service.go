/*
Copyright 2025 Flant JSC

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

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/deckhouse/virtualization-controller/pkg/common/annotations"
	"github.com/deckhouse/virtualization-controller/pkg/dvcr"
)

type DVCRService struct {
	client       client.Client
	dvcrSettings *dvcr.Settings
}

func NewDVCRService(
	client client.Client,
	dvcrSettings *dvcr.Settings,
) *DVCRService {
	return &DVCRService{
		client:       client,
		dvcrSettings: dvcrSettings,
	}
}

const (
	moduleNamespace           = "d8-virtualization"
	dvcrDeploymentName        = "dvcr"
	maintenanceModeSecretName = "dvcr-maintenance"
)

func (d *DVCRService) CreateMaintenanceModeSecret(ctx context.Context) error {
	secret := &corev1.Secret{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Secret",
			APIVersion: "v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Namespace: moduleNamespace,
			Name:      maintenanceModeSecretName,
		},
	}
	return d.client.Create(ctx, secret)
}

func (d *DVCRService) IsMaintenanceModeEnabled(ctx context.Context) (bool, error) {
	secret, err := d.GetMaintenanceSecret(ctx)
	if secret == nil {
		return false, err
	}
	return true, err
}

func (d *DVCRService) IsAutoCleanupEnabled(ctx context.Context) (bool, error) {
	secret, err := d.GetMaintenanceSecret(ctx)
	if secret == nil {
		return false, err
	}
	_, ok := secret.GetAnnotations()[annotations.AnnDVCRDeploymentSwitchToMaintenanceMode]
	return ok, nil
}

func (d *DVCRService) EnableAutoCleanup(ctx context.Context) error {
	secret, err := d.GetMaintenanceSecret(ctx)
	if secret == nil {
		return fmt.Errorf("get maintenance secret to update: %w", err)
	}

	objAnnotations := secret.GetAnnotations()
	if objAnnotations == nil {
		objAnnotations = make(map[string]string)
	}
	objAnnotations[annotations.AnnDVCRDeploymentSwitchToMaintenanceMode] = ""
	secret.SetAnnotations(objAnnotations)
	return d.client.Update(ctx, secret)
}

func (d *DVCRService) GetMaintenanceSecret(ctx context.Context) (*corev1.Secret, error) {
	var secret corev1.Secret
	secretKey := types.NamespacedName{
		Namespace: moduleNamespace,
		Name:      maintenanceModeSecretName,
	}
	err := d.client.Get(ctx, secretKey, &secret)
	if err != nil {
		if k8serrors.IsNotFound(err) {
			return nil, nil
		}
		return nil, err
	}

	return &secret, nil
}

func (d *DVCRService) RemoveMaintenanceModeSecret(ctx context.Context) error {
	secret := &corev1.Secret{}
	secret.SetNamespace(moduleNamespace)
	secret.SetName(maintenanceModeSecretName)
	err := d.client.Delete(ctx, secret)
	return client.IgnoreNotFound(err)
}

func (d *DVCRService) GetDeployment(ctx context.Context) (*appsv1.Deployment, error) {
	var dvcrDeployment appsv1.Deployment
	dvcrKey := types.NamespacedName{
		Namespace: moduleNamespace,
		Name:      dvcrDeploymentName,
	}
	err := d.client.Get(ctx, dvcrKey, &dvcrDeployment)
	if err != nil {
		if k8serrors.IsNotFound(err) {
			return nil, nil
		}
		return nil, err
	}

	return &dvcrDeployment, nil
}

func (d *DVCRService) UpdateDeploymentMaintenanceConditions(ctx context.Context, conditions []appsv1.DeploymentCondition) (*appsv1.Deployment, error) {
	var dvcrDeployment appsv1.Deployment
	dvcrKey := types.NamespacedName{
		Namespace: moduleNamespace,
		Name:      dvcrDeploymentName,
	}
	err := d.client.Get(ctx, dvcrKey, &dvcrDeployment)
	if err != nil {
		if k8serrors.IsNotFound(err) {
			return nil, nil
		}
		return nil, err
	}

	return &dvcrDeployment, nil
}
