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

package copier

import (
	"bytes"
	"context"
	"fmt"
	"maps"

	corev1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/deckhouse/virtualization-controller/pkg/common/annotations"
	"github.com/deckhouse/virtualization-controller/pkg/common/merger"
	"github.com/deckhouse/virtualization-controller/pkg/common/object"
)

// Secret copies or creates Secret from Source to Destination.
// Additionally, OwnerRef, Annotations, and Labels may be passed.
type Secret struct {
	Source         types.NamespacedName
	Destination    types.NamespacedName
	OwnerReference metav1.OwnerReference
	Annotations    map[string]string
	Labels         map[string]string
}

func (s Secret) Create(ctx context.Context, client client.Client, data map[string][]byte, secretType corev1.SecretType) (*corev1.Secret, error) {
	destObj := s.makeSecret(data, secretType)

	err := client.Create(ctx, destObj)
	// Ignore if Secret is already exists.
	if err != nil && k8serrors.IsAlreadyExists(err) {
		return destObj, nil
	}
	return destObj, err
}

// Copy copies source Secret data and type as-is.
func (s Secret) Copy(ctx context.Context, client client.Client) error {
	srcObj, err := object.FetchObject(ctx, s.Source, client, &corev1.Secret{})
	if err != nil {
		return err
	}

	_, err = s.Create(ctx, client, srcObj.Data, srcObj.Type)
	return err
}

// CopyOrUpdate copies source Secret data as-is and, unlike Copy, refreshes the
// data of an existing destination Secret so copies follow source rotation. If
// the destination exists but is not yet owned by s.OwnerReference, the owner
// is added (as a non-controller reference when a controller is already set),
// so a secret shared by several pods lives until the last of them is gone.
func (s Secret) CopyOrUpdate(ctx context.Context, client client.Client) error {
	srcObj, err := object.FetchObject(ctx, s.Source, client, &corev1.Secret{})
	if err != nil {
		return err
	}
	if srcObj == nil {
		return fmt.Errorf("source secret %s not found", s.Source)
	}

	destObj, err := object.FetchObject(ctx, s.Destination, client, &corev1.Secret{})
	if err != nil {
		return err
	}
	if destObj == nil {
		_, err = s.Create(ctx, client, srcObj.Data, srcObj.Type)
		return err
	}

	var changed bool
	if !maps.EqualFunc(destObj.Data, srcObj.Data, bytes.Equal) {
		destObj.Data = srcObj.Data
		changed = true
	}
	if s.mergeOwnerReference(destObj) {
		changed = true
	}
	if !changed {
		return nil
	}
	return client.Update(ctx, destObj)
}

func (s Secret) mergeOwnerReference(destObj *corev1.Secret) bool {
	if s.OwnerReference.UID == "" {
		return false
	}
	hasController := false
	for _, ref := range destObj.OwnerReferences {
		if ref.UID == s.OwnerReference.UID {
			return false
		}
		if ref.Controller != nil && *ref.Controller {
			hasController = true
		}
	}
	ref := s.OwnerReference
	if hasController && ref.Controller != nil && *ref.Controller {
		ref.Controller = ptr.To(false)
	}
	destObj.OwnerReferences = append(destObj.OwnerReferences, ref)
	return true
}

func (s Secret) makeSecret(data map[string][]byte, secretType corev1.SecretType) *corev1.Secret {
	secret := &corev1.Secret{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Secret",
			APIVersion: "v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      s.Destination.Name,
			Namespace: s.Destination.Namespace,
			Annotations: map[string]string{
				annotations.AnnCreatedBy: "yes",
			},
			Labels: map[string]string{},
		},
		Data: data,
		Type: secretType,
	}

	if s.OwnerReference.Name != "" {
		secret.OwnerReferences = []metav1.OwnerReference{
			s.OwnerReference,
		}
	}

	if s.Labels != nil {
		secret.Labels = merger.MergeLabels(secret.GetLabels(), s.Labels)
	}

	if s.Annotations != nil {
		secret.Annotations = merger.MergeLabels(secret.GetAnnotations(), s.Annotations)
	}

	return secret
}
