/*
Copyright 2026 Flant JSC

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
	"errors"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/deckhouse/virtualization/api/core/v1alpha2"
)

const cloudInitUserMaxLen = 2048

var (
	ErrUserDataEmpty   = errors.New("provisioning userdata is defined, but it is empty")
	ErrUserDataTooLong = fmt.Errorf("userdata exceeds %d byte limit; should use userDataRef for larger data", cloudInitUserMaxLen)
)

type ProvisioningService struct {
	reader client.Reader
}

func NewProvisioningService(reader client.Reader) *ProvisioningService {
	return &ProvisioningService{
		reader: reader,
	}
}

var ErrSecretIsNotValid = errors.New("secret is not valid")

type SecretNotFoundError string

func (e SecretNotFoundError) Error() string {
	return fmt.Sprintf("secret %s not found", string(e))
}

type UnexpectedSecretTypeError string

func (e UnexpectedSecretTypeError) Error() string {
	return fmt.Sprintf("unexpected secret type: %s", string(e))
}

var cloudInitCheckKeys = []string{
	"userdata",
	"userData",
}

func (p *ProvisioningService) Validate(ctx context.Context, key types.NamespacedName) error {
	secret := &corev1.Secret{}
	err := p.reader.Get(ctx, key, secret)
	if err != nil {
		if k8serrors.IsNotFound(err) {
			return SecretNotFoundError(key.String())
		}
		return err
	}
	switch secret.Type {
	case v1alpha2.SecretTypeCloudInit:
		return p.validateCloudInitSecret(secret)
	case v1alpha2.SecretTypeSysprep:
		return p.validateSysprepSecret(secret)
	default:
		return UnexpectedSecretTypeError(secret.Type)
	}
}

func (p *ProvisioningService) validateCloudInitSecret(secret *corev1.Secret) error {
	if !p.hasOneOfKeys(secret, cloudInitCheckKeys...) {
		return fmt.Errorf("the secret should have one of data fields %v: %w", cloudInitCheckKeys, ErrSecretIsNotValid)
	}
	return nil
}

func (p *ProvisioningService) validateSysprepSecret(_ *corev1.Secret) error {
	return nil
}

func (p *ProvisioningService) hasOneOfKeys(secret *corev1.Secret, checkKeys ...string) bool {
	validate := len(checkKeys) == 0
	for _, key := range checkKeys {
		if _, ok := secret.Data[key]; ok {
			validate = true
			break
		}
	}
	return validate
}

func (p *ProvisioningService) ValidateUserDataLen(userData string) error {
	if userData == "" {
		return ErrUserDataEmpty
	}

	if len(userData) > cloudInitUserMaxLen {
		return ErrUserDataTooLong
	}

	return nil
}
