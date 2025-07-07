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

package restorer

import (
	"bytes"
	"context"
	"fmt"
	"maps"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/deckhouse/virtualization-controller/pkg/common/object"
	virtv2 "github.com/deckhouse/virtualization/api/core/v1alpha2"
)

type ProvisionerOverrideValidator struct {
	secret *corev1.Secret
	client client.Client
}

func NewProvisionerOverrideValidator(secretTmpl *corev1.Secret, client client.Client) *ProvisionerOverrideValidator {
	return &ProvisionerOverrideValidator{
		secret: &corev1.Secret{
			TypeMeta: metav1.TypeMeta{
				Kind:       secretTmpl.Kind,
				APIVersion: secretTmpl.APIVersion,
			},
			ObjectMeta: metav1.ObjectMeta{
				Name:        secretTmpl.Name,
				Namespace:   secretTmpl.Namespace,
				Annotations: secretTmpl.Annotations,
				Labels:      secretTmpl.Labels,
			},
			Immutable:  secretTmpl.Immutable,
			Data:       secretTmpl.Data,
			StringData: secretTmpl.StringData,
			Type:       secretTmpl.Type,
		},
		client: client,
	}
}

func (v *ProvisionerOverrideValidator) Override(rules []virtv2.NameReplacement) {
	v.secret.Name = overrideName(v.secret.Kind, v.secret.Name, rules)
}

func (v *ProvisionerOverrideValidator) Validate(ctx context.Context) error {
	secretKey := types.NamespacedName{Namespace: v.secret.Namespace, Name: v.secret.Name}
	existed, err := object.FetchObject(ctx, secretKey, v.client, &corev1.Secret{})
	if err != nil {
		return err
	}

	if existed == nil {
		return nil
	}

	if !maps.EqualFunc(existed.Data, v.secret.Data, bytes.Equal) {
		return fmt.Errorf("the provisioner secret %q %w and has not the same data content", secretKey.Name, ErrAlreadyExists)
	}

	return nil
}

func (v *ProvisionerOverrideValidator) Object() client.Object {
	return v.secret
}
