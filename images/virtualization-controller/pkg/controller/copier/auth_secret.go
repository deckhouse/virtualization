package copier

import (
	"context"

	corev1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/deckhouse/virtualization-controller/pkg/auth"
	"github.com/deckhouse/virtualization-controller/pkg/common"
	cc "github.com/deckhouse/virtualization-controller/pkg/controller/common"
	"github.com/deckhouse/virtualization-controller/pkg/sdk/framework/helper"
)

// AuthSecret copies auth credentials from the source Secret into
// Destination Secret and ensure its data is CDI compatible:
// type: Opaque
// data:
//
//	accessKeyId: ""  # <optional: your key or username, base64 encoded>
//	secretKey:   "" # <optional: your secret or password, base64 encoded>
//
// Additionally OwnerRef, Annotations, and Labels may be passed.
type AuthSecret struct {
	Source         types.NamespacedName
	Destination    types.NamespacedName
	OwnerReference metav1.OwnerReference
	Annotations    map[string]string
	Labels         map[string]string
}

func (a *AuthSecret) Create(ctx context.Context, client client.Client, data map[string][]byte, secretType corev1.SecretType) (*corev1.Secret, error) {
	destObj := a.makeSecret(data, secretType)

	err := client.Create(ctx, destObj)
	// Ignore if Secret is already exists.
	if err != nil && k8serrors.IsAlreadyExists(err) {
		return destObj, nil
	}
	return destObj, err
}

// Copy copies source Secret data and type as-is.
func (a *AuthSecret) Copy(ctx context.Context, client client.Client) error {
	srcObj, err := helper.FetchObject(ctx, a.Source, client, &corev1.Secret{})
	if err != nil {
		return err
	}

	_, err = a.Create(ctx, client, srcObj.Data, srcObj.Type)
	return err
}

// CopyCDICompatible transforms auth credentials in dockerconfigjson format into CDI compatible:
// a Secret with two fields: accessKeyId and secretKey.
// ref is registry url or image name. It is used to select a desired auth pair from the config.
func (a *AuthSecret) CopyCDICompatible(ctx context.Context, client client.Client, ref string) error {
	srcObj, err := helper.FetchObject(ctx, a.Source, client, &corev1.Secret{})
	if err != nil {
		return err
	}

	destData := srcObj.Data
	destType := srcObj.Type
	if srcObj.Type == corev1.SecretTypeDockerConfigJson {
		cfg, err := auth.ReadDockerConfigJSON(srcObj.Data[corev1.DockerConfigJsonKey])
		if err != nil {
			return err
		}
		username, password, err := auth.CredsFromRegistryAuthFile(cfg, ref)
		if err != nil {
			return err
		}
		destData = map[string][]byte{
			"accessKeyId": []byte(username),
			"secretKey":   []byte(password),
		}
		destType = corev1.SecretTypeOpaque
	}

	_, err = a.Create(ctx, client, destData, destType)
	return err
}

func (a *AuthSecret) makeSecret(data map[string][]byte, secretType corev1.SecretType) *corev1.Secret {
	secret := &corev1.Secret{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Secret",
			APIVersion: "v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      a.Destination.Name,
			Namespace: a.Destination.Namespace,
			Annotations: map[string]string{
				cc.AnnCreatedBy: "yes",
			},
			Labels: map[string]string{},
			OwnerReferences: []metav1.OwnerReference{
				a.OwnerReference,
			},
		},
		Data: data,
		Type: secretType,
	}

	if a.Labels != nil {
		secret.Labels = common.MergeLabels(secret.GetLabels(), a.Labels)
	}

	if a.Annotations != nil {
		secret.Annotations = common.MergeLabels(secret.GetAnnotations(), a.Annotations)
	}

	return secret
}
