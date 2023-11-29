package copier

import (
	"context"

	corev1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/deckhouse/virtualization-controller/pkg/common"
	cc "github.com/deckhouse/virtualization-controller/pkg/controller/common"
	"github.com/deckhouse/virtualization-controller/pkg/sdk/framework/helper"
)

type CABundleConfigMap struct {
	SourceSecret   types.NamespacedName
	Destination    types.NamespacedName
	OwnerReference metav1.OwnerReference
	Annotations    map[string]string
	Labels         map[string]string
}

const (
	CAFieldName = "ca.crt"
)

// Create creates Destination ConfigMap with CABundle string.
func (c CABundleConfigMap) Create(ctx context.Context, client client.Client, caBundle string) (*corev1.ConfigMap, error) {
	destObj := c.makeConfigMap(caBundle)

	err := client.Create(ctx, destObj)
	// Ignore if ConfigMap is already exists.
	if err != nil && k8serrors.IsAlreadyExists(err) {
		return destObj, nil
	}
	return destObj, err
}

// Copy creates Destination ConfigMap from SourceSecret or from SourceConfigMap
func (c CABundleConfigMap) Copy(ctx context.Context, client client.Client) error {
	var caBundle string

	srcObj, err := helper.FetchObject(ctx, c.SourceSecret, client, &corev1.Secret{})
	if err != nil {
		return err
	}
	if srcObj != nil {
		caBundle = string(srcObj.Data[CAFieldName])
	}

	destObj := c.makeConfigMap(caBundle)

	err = client.Create(ctx, destObj)
	// Ignore if ConfigMap is already exists.
	if err != nil && k8serrors.IsAlreadyExists(err) {
		return nil
	}
	return err
}

// makeConfigMap create CDI compatible ConfigMap resource with CA bundle
// in the field named as "ca.crt".
func (c CABundleConfigMap) makeConfigMap(caBundle string) *corev1.ConfigMap {
	cm := &corev1.ConfigMap{
		TypeMeta: metav1.TypeMeta{
			Kind:       "ConfigMap",
			APIVersion: "v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      c.Destination.Name,
			Namespace: c.Destination.Namespace,
			Annotations: map[string]string{
				cc.AnnCreatedBy: "yes",
			},
			Labels: map[string]string{},
			OwnerReferences: []metav1.OwnerReference{
				c.OwnerReference,
			},
		},
		Data: map[string]string{
			// CA bundle should have .crt extension.
			// See https://github.com/kubevirt/containerized-data-importer/pull/2987
			CAFieldName: caBundle,
		},
	}

	if c.Labels != nil {
		cm.Labels = common.MergeLabels(cm.GetLabels(), c.Labels)
	}

	if c.Annotations != nil {
		cm.Annotations = common.MergeLabels(cm.GetAnnotations(), c.Annotations)
	}

	return cm
}
