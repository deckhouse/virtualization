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

	"github.com/deckhouse/virtualization-controller/pkg/common/annotations"
	"github.com/deckhouse/virtualization-controller/pkg/common/object"
	"github.com/deckhouse/virtualization-controller/pkg/controller/service/restorer/common"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
)

type ProvisionerHandler struct {
	secret     *corev1.Secret
	client     client.Client
	restoreUID string
}

func NewProvisionerHandler(client client.Client, secretTmpl corev1.Secret, vmRestoreUID string) *ProvisionerHandler {
	if secretTmpl.Annotations == nil {
		secretTmpl.Annotations = make(map[string]string)
	}
	secretTmpl.Annotations[annotations.AnnVMOPRestore] = vmRestoreUID
	return &ProvisionerHandler{
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
		client:     client,
		restoreUID: vmRestoreUID,
	}
}

func (v *ProvisionerHandler) Override(rules []v1alpha2.NameReplacement) {
	v.secret.Name = common.OverrideName(v.secret.Kind, v.secret.Name, rules)
}

func (v *ProvisionerHandler) Customize(prefix, suffix string) {
	v.secret.Name = common.ApplyNameCustomization(v.secret.Name, prefix, suffix)
}

func (v *ProvisionerHandler) ValidateRestore(ctx context.Context) error {
	secretKey := types.NamespacedName{Namespace: v.secret.Namespace, Name: v.secret.Name}
	existed, err := object.FetchObject(ctx, secretKey, v.client, &corev1.Secret{})
	if err != nil {
		return err
	}

	if existed == nil {
		return nil
	}

	if value, ok := existed.Annotations[annotations.AnnVMOPRestore]; ok && value == v.restoreUID {
		return nil
	}

	if !maps.EqualFunc(existed.Data, v.secret.Data, bytes.Equal) {
		return fmt.Errorf("the provisioner secret %q %w", secretKey.Name, common.ErrSecretHasDifferentData)
	}

	return nil
}

func (v *ProvisionerHandler) ValidateClone(ctx context.Context) error {
	if err := common.ValidateResourceNameLength(v.secret.Name); err != nil {
		return err
	}

	secretKey := types.NamespacedName{Namespace: v.secret.Namespace, Name: v.secret.Name}
	existed, err := object.FetchObject(ctx, secretKey, v.client, &corev1.Secret{})
	if err != nil {
		return err
	}

	if existed != nil {
		if !maps.EqualFunc(existed.Data, v.secret.Data, bytes.Equal) {
			return common.FormatSecretContentDifferentError(v.secret.Name)
		}
	}

	return nil
}

func (v *ProvisionerHandler) ProcessRestore(ctx context.Context) error {
	err := v.ValidateRestore(ctx)
	if err != nil {
		return err
	}

	secretKey := types.NamespacedName{Namespace: v.secret.Namespace, Name: v.secret.Name}
	existed, err := object.FetchObject(ctx, secretKey, v.client, &corev1.Secret{})
	if err != nil {
		return err
	}

	if existed != nil {
		if value, ok := existed.Annotations[annotations.AnnVMOPRestore]; ok && value == v.restoreUID {
			return nil
		}
	} else {
		err = v.client.Create(ctx, v.secret)
		if err != nil {
			return fmt.Errorf("failed to create the `Secret`: %w", err)
		}
	}

	return nil
}

func (v *ProvisionerHandler) ProcessClone(ctx context.Context) error {
	err := v.ValidateClone(ctx)
	if err != nil {
		return err
	}

	err = v.client.Create(ctx, v.secret)
	if err != nil {
		return fmt.Errorf("failed to create the `Secret`: %w", err)
	}

	return nil
}

func (v *ProvisionerHandler) Object() client.Object {
	return v.secret
}
