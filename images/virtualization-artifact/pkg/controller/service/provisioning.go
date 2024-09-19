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

var (
	ErrSecretIsNotValid = errors.New("secret is not valid")
)

type ErrSecretNotFound string

func (e ErrSecretNotFound) Error() string {
	return fmt.Sprintf("secret %s not found", e)
}

type ErrUnexpectedSecretType string

func (e ErrUnexpectedSecretType) Error() string {
	return fmt.Sprintf("unexpected secret type %s", e)
}

var cloudInitCheckKeys = []string{
	"userdata",
	"userData",
}

func NewProvisioningValidator(reader client.Reader) *ProvisioningValidator {
	return &ProvisioningValidator{}
}

type ProvisioningValidator struct {
	reader client.Reader
}

func (v ProvisioningValidator) Validate(ctx context.Context, key types.NamespacedName) error {
	secret := &corev1.Secret{}
	err := v.reader.Get(ctx, key, secret)
	if err != nil {
		if k8serrors.IsNotFound(err) {
			return ErrSecretNotFound(key.String())
		}
		return err
	}
	switch secret.Type {
	case v1alpha2.SecretTypeCloudInit:
		return v.validateCloudInitSecret(secret)
	case v1alpha2.SecretTypeSysprep:
		return v.validateSysprepSecret(secret)
	default:
		return ErrUnexpectedSecretType(secret.Type)
	}
}

func (v ProvisioningValidator) validateCloudInitSecret(secret *corev1.Secret) error {
	if !v.keysIsExist(secret, cloudInitCheckKeys...) {
		return fmt.Errorf("secret should has one of data fields %v: %w", cloudInitCheckKeys, ErrSecretIsNotValid)
	}
	return nil
}

func (v ProvisioningValidator) validateSysprepSecret(_ *corev1.Secret) error {
	return nil
}

func (v ProvisioningValidator) keysIsExist(secret *corev1.Secret, checkKeys ...string) bool {
	validate := len(checkKeys) == 0
	for _, key := range checkKeys {
		if _, ok := secret.Data[key]; ok {
			validate = true
			break
		}
	}
	return validate
}
