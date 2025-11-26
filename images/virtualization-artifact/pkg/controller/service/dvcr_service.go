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
	"encoding/json"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/deckhouse/virtualization-controller/pkg/common/annotations"
	dvcrdeploymentcondition "github.com/deckhouse/virtualization/api/core/v1alpha2/dvcr-deployment-condition"
)

type DVCRService struct {
	client client.Client
}

func NewDVCRService(client client.Client) *DVCRService {
	return &DVCRService{
		client: client,
	}
}

const (
	moduleNamespace                 = "d8-virtualization"
	garbageCollectionModeSecretName = "dvcr-garbage-collection"
)

func (d *DVCRService) CreateGarbageCollectionSecret(ctx context.Context) error {
	secret := &corev1.Secret{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Secret",
			APIVersion: "v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Namespace: moduleNamespace,
			Name:      garbageCollectionModeSecretName,
		},
	}
	return d.client.Create(ctx, secret)
}

// IsGarbageCollectionSecretExist returns true if garbage collection secret exists.
func (d *DVCRService) IsGarbageCollectionSecretExist(ctx context.Context) (bool, error) {
	secret, err := d.GetGarbageCollectionSecret(ctx)
	return secret != nil, err
}

// IsGarbageCollectionStarted returns true if switch to garbage collection mode is on.
// Use it to determine "wait" state.
func (d *DVCRService) IsGarbageCollectionStarted(secret *corev1.Secret) bool {
	if secret == nil {
		return false
	}
	_, switched := secret.GetAnnotations()[annotations.AnnDVCRDeploymentSwitchToGarbageCollectionMode]
	return switched
}

// IsGarbageCollectionInitiatedOrInProgress returns true if secret exists but
// garbage collection is not done yet. (Use it to postpone rw operations with registry).
func (d *DVCRService) IsGarbageCollectionInitiatedOrInProgress(secret *corev1.Secret) bool {
	if secret == nil {
		return false
	}
	_, done := secret.GetAnnotations()[annotations.AnnDVCRGarbageCollectionDone]
	return !done
}

// IsGarbageCollectionDone returns true if secret exists and annotated with
// "done" annotation.
func (d *DVCRService) IsGarbageCollectionDone(secret *corev1.Secret) bool {
	if secret == nil {
		return false
	}
	_, done := secret.GetAnnotations()[annotations.AnnDVCRGarbageCollectionDone]
	return done
}

func (d *DVCRService) InitiateGarbageCollectionMode(ctx context.Context) error {
	secret, err := d.GetGarbageCollectionSecret(ctx)
	if err != nil {
		return fmt.Errorf("get garbage collection secret: %w", err)
	}
	if secret == nil {
		return d.CreateGarbageCollectionSecret(ctx)
	}

	// Update existing secret to initial state: remove annotations and data.
	secret.SetAnnotations(nil)
	secret.Data = nil
	return d.client.Update(ctx, secret)
}

func (d *DVCRService) SwitchToGarbageCollectionMode(ctx context.Context) error {
	secret, err := d.GetGarbageCollectionSecret(ctx)
	if secret == nil {
		return fmt.Errorf("get garbage collection secret to update: %w", err)
	}

	objAnnotations := secret.GetAnnotations()
	if objAnnotations == nil {
		objAnnotations = make(map[string]string)
	}
	objAnnotations[annotations.AnnDVCRDeploymentSwitchToGarbageCollectionMode] = ""
	secret.SetAnnotations(objAnnotations)
	return d.client.Update(ctx, secret)
}

func (d *DVCRService) GetGarbageCollectionSecret(ctx context.Context) (*corev1.Secret, error) {
	var secret corev1.Secret
	secretKey := types.NamespacedName{
		Namespace: moduleNamespace,
		Name:      garbageCollectionModeSecretName,
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

func (d *DVCRService) DeleteGarbageCollectionSecret(ctx context.Context) error {
	secret := &corev1.Secret{}
	secret.SetNamespace(moduleNamespace)
	secret.SetName(garbageCollectionModeSecretName)
	err := d.client.Delete(ctx, secret)
	return client.IgnoreNotFound(err)
}

func (d *DVCRService) GetGarbageCollectionResult(secret *corev1.Secret) string {
	if secret == nil {
		return ""
	}
	return string(secret.Data["result"])
}

type GarbageCollectionResult struct {
	Result  string `json:"result"`
	Error   string `json:"error"`
	Message string `json:"message"`
}

func (d *DVCRService) ParseGarbageCollectionResult(secret *corev1.Secret) (reason dvcrdeploymentcondition.GarbageCollectionReason, message string, err error) {
	var gcResult GarbageCollectionResult
	err = json.Unmarshal(secret.Data["result"], &gcResult)
	if err != nil {
		return "", "", fmt.Errorf("parse garbage collection result '%s': %w", string(secret.Data["result"]), err)
	}

	switch gcResult.Result {
	case "success":
		return dvcrdeploymentcondition.Completed, gcResult.Message, nil
	case "fail":
		return dvcrdeploymentcondition.Error, gcResult.Error, nil
	}

	// Unexpected format. It should not happen, but we need to show something if it happens.
	return dvcrdeploymentcondition.Completed, string(secret.Data["result"]), nil
}
