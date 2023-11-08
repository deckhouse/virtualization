package importer

import (
	"context"
	"errors"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	cc "github.com/deckhouse/virtualization-controller/pkg/controller/common"
	"github.com/deckhouse/virtualization-controller/pkg/sdk/framework/helper"
)

type SecretSettings struct {
	Name           string
	Namespace      string
	Data           map[string][]byte
	Type           corev1.SecretType
	OwnerReference metav1.OwnerReference
}

type Secret struct {
	Settings *SecretSettings
}

func NewSecret(settings *SecretSettings) *Secret {
	return &Secret{settings}
}

func (s Secret) Create(ctx context.Context, client client.Client) (*corev1.Secret, error) {
	secret := s.makeSpec()

	if err := client.Create(ctx, secret); err != nil {
		return nil, err
	}

	return secret, nil
}

func (s Secret) makeSpec() *corev1.Secret {
	secret := &corev1.Secret{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Secret",
			APIVersion: "v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      s.Settings.Name,
			Namespace: s.Settings.Namespace,
			Annotations: map[string]string{
				cc.AnnCreatedBy: "yes",
			},
			Labels: map[string]string{
				// TODO add labels
			},
			OwnerReferences: []metav1.OwnerReference{
				s.Settings.OwnerReference,
			},
		},
		Data: s.Settings.Data,
		Type: s.Settings.Type,
	}

	return secret
}

var ErrSecretNameNotFound = errors.New("secret name not found")

func FindSecret(ctx context.Context, client client.Client, obj metav1.Object) (*corev1.Secret, error) {
	secretName := obj.GetAnnotations()[cc.AnnAuthSecret]
	if secretName == "" {
		return nil, ErrSecretNameNotFound
	}

	secretNs := obj.GetAnnotations()[cc.AnnImporterNamespace]
	if secretNs == "" {
		secretNs = obj.GetNamespace()
	}

	objName := types.NamespacedName{Name: secretName, Namespace: secretNs}
	return helper.FetchObject(ctx, objName, client, &corev1.Secret{})
}
