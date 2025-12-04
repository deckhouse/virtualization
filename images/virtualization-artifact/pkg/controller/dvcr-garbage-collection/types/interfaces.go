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

	dvcrdeploymentcondition "github.com/deckhouse/virtualization/api/core/v1alpha2/dvcr-deployment-condition"
)

type DVCRService interface {
	GetGarbageCollectionSecret(ctx context.Context) (*corev1.Secret, error)
	DeleteGarbageCollectionSecret(ctx context.Context) error
	InitiateGarbageCollectionMode(ctx context.Context) error
	SwitchToGarbageCollectionMode(ctx context.Context) error

	IsGarbageCollectionStarted(secret *corev1.Secret) bool
	IsGarbageCollectionDone(secret *corev1.Secret) bool

	GetGarbageCollectionResult(secret *corev1.Secret) string
	ParseGarbageCollectionResult(secret *corev1.Secret) (reason dvcrdeploymentcondition.GarbageCollectionReason, message string, err error)
}

type ProvisioningLister interface {
	ListAllInProvisioning(ctx context.Context) ([]client.Object, error)
}
