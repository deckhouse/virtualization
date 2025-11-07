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
