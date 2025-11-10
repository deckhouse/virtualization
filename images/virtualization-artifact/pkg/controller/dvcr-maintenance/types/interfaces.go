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

package types

import (
	"context"

	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type DVCRService interface {
	GetMaintenanceSecret(ctx context.Context) (*corev1.Secret, error)
	DeleteMaintenanceSecret(ctx context.Context) error
	InitiateMaintenanceMode(ctx context.Context) error
	SwitchToMaintenanceMode(ctx context.Context) error

	// IsMaintenanceSecretExist(secret *corev1.Secret) bool

	IsMaintenanceInitiatedNotStarted(secret *corev1.Secret) bool
	IsMaintenanceStarted(secret *corev1.Secret) bool
	IsMaintenanceDone(secret *corev1.Secret) bool
}

type ProvisioningLister interface {
	ListAllInProvisioning(ctx context.Context) ([]client.Object, error)
}
