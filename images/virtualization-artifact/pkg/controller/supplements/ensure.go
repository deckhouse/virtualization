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

package supplements

import (
	"context"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	netv1 "k8s.io/api/networking/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	cdiv1 "kubevirt.io/containerized-data-importer-api/pkg/apis/core/v1beta1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/deckhouse/virtualization-controller/pkg/common/datasource"
	dvutil "github.com/deckhouse/virtualization-controller/pkg/common/datavolume"
	ingutil "github.com/deckhouse/virtualization-controller/pkg/common/ingress"
	"github.com/deckhouse/virtualization-controller/pkg/common/object"
	podutil "github.com/deckhouse/virtualization-controller/pkg/common/pod"
	"github.com/deckhouse/virtualization-controller/pkg/controller/supplements/copier"
	"github.com/deckhouse/virtualization-controller/pkg/dvcr"
)

type DataSource interface {
	HasCABundle() bool
	GetCABundle() string
	GetContainerImage() *datasource.ContainerRegistry
}

// EnsureForPod make supplements for importer or uploader Pod:
// - It creates ConfigMap with caBundle for http and containerImage data sources.
// - It copies DVCR auth Secret to use DVCR as destination.
func EnsureForPod(ctx context.Context, client client.Client, supGen Generator, pod *corev1.Pod, ds DataSource, dvcrSettings *dvcr.Settings) error {
	// Create ConfigMap with caBundle.
	if ds.HasCABundle() {
		caBundleCM := supGen.CABundleConfigMap()
		caBundleCopier := copier.CABundleConfigMap{
			Destination:    caBundleCM,
			OwnerReference: podutil.MakeOwnerReference(pod),
		}
		_, err := caBundleCopier.Create(ctx, client, ds.GetCABundle())
		if err != nil {
			return fmt.Errorf("create ConfigMap/%s with ca bundle: %w", caBundleCM.Name, err)
		}
	}

	// Create Secret with auth config to use DVCR as destination.
	if ShouldCopyDVCRAuthSecret(dvcrSettings, supGen) {
		authSecret := supGen.DVCRAuthSecret()
		authCopier := copier.AuthSecret{
			Secret: copier.Secret{
				Source: types.NamespacedName{
					Name:      dvcrSettings.AuthSecret,
					Namespace: dvcrSettings.AuthSecretNamespace,
				},
				Destination:    authSecret,
				OwnerReference: podutil.MakeOwnerReference(pod),
			},
		}
		err := authCopier.Copy(ctx, client)
		if err != nil {
			return err
		}
	}

	// Copy imagePullSecret if namespaces are differ (e.g. CVMI).
	if ds != nil && ShouldCopyImagePullSecret(ds.GetContainerImage(), supGen.Namespace()) {
		imgPull := supGen.ImagePullSecret()
		imgPullCopier := copier.Secret{
			Source: types.NamespacedName{
				Namespace: ds.GetContainerImage().ImagePullSecret.Name,
				Name:      ds.GetContainerImage().ImagePullSecret.Namespace,
			},
			Destination:    imgPull,
			OwnerReference: podutil.MakeOwnerReference(pod),
		}
		err := imgPullCopier.Copy(ctx, client)
		if err != nil {
			return err
		}
	}

	// TODO(future): ensure ca ConfigMap and auth Secret for proxy.

	return nil
}

func ShouldCopyDVCRAuthSecret(dvcrSettings *dvcr.Settings, supGen Generator) bool {
	if dvcrSettings.AuthSecret == "" {
		return false
	}
	// Should copy if namespaces are different.
	return dvcrSettings.AuthSecretNamespace != supGen.Namespace()
}

func ShouldCopyUploaderTLSSecret(dvcrSettings *dvcr.Settings, supGen Generator) bool {
	if dvcrSettings.UploaderIngressSettings.TLSSecret == "" {
		return false
	}
	// Should copy if namespaces are different.
	return dvcrSettings.UploaderIngressSettings.TLSSecretNamespace != supGen.Namespace()
}

func ShouldCopyImagePullSecret(ctrImg *datasource.ContainerRegistry, targetNS string) bool {
	if ctrImg == nil || ctrImg.ImagePullSecret.Name == "" {
		return false
	}

	imgPullNS := ctrImg.ImagePullSecret.Namespace

	// Should copy imagePullSecret if namespace differs from the specified namespace.
	return imgPullNS != "" && imgPullNS != targetNS
}

func EnsureForDataVolume(ctx context.Context, client client.Client, supGen DataVolumeSupplement, dv *cdiv1.DataVolume, dvcrSettings *dvcr.Settings) error {
	if dvcrSettings.AuthSecret != "" {
		authSecret := supGen.DVCRAuthSecretForDV()
		authCopier := copier.AuthSecret{
			Secret: copier.Secret{
				Source: types.NamespacedName{
					Name:      dvcrSettings.AuthSecret,
					Namespace: dvcrSettings.AuthSecretNamespace,
				},
				Destination:    authSecret,
				OwnerReference: dvutil.MakeOwnerReference(dv),
			},
		}

		err := authCopier.CopyCDICompatible(ctx, client, dvcrSettings.RegistryURL)
		if err != nil {
			return err
		}
	}

	// CABundle needs transformation, so it always copied.
	if dvcrSettings.CertsSecret != "" {
		caBundleCM := supGen.DVCRCABundleConfigMapForDV()
		caBundleCopier := copier.CABundleConfigMap{
			SourceSecret: types.NamespacedName{
				Name:      dvcrSettings.CertsSecret,
				Namespace: dvcrSettings.CertsSecretNamespace,
			},
			Destination:    caBundleCM,
			OwnerReference: dvutil.MakeOwnerReference(dv),
		}

		return caBundleCopier.Copy(ctx, client)
	}

	return nil
}

func CleanupForDataVolume(ctx context.Context, client client.Client, supGen Generator, dvcrSettings *dvcr.Settings) error {
	// AuthSecret has type dockerconfigjson and should be transformed, so it always copied.
	if dvcrSettings.AuthSecret != "" {
		authSecret := supGen.DVCRAuthSecretForDV()
		err := object.CleanupByName(ctx, client, authSecret, &corev1.Secret{})
		if err != nil && !k8serrors.IsNotFound(err) {
			return err
		}
	}

	// CABundle needs transformation, so it always copied.
	if dvcrSettings.CertsSecret != "" {
		caBundleCM := supGen.DVCRCABundleConfigMapForDV()
		err := object.CleanupByName(ctx, client, caBundleCM, &corev1.ConfigMap{})
		if err != nil && !k8serrors.IsNotFound(err) {
			return err
		}
	}

	return nil
}

func EnsureForIngress(ctx context.Context, client client.Client, supGen Generator, ing *netv1.Ingress, dvcrSettings *dvcr.Settings) error {
	if ShouldCopyUploaderTLSSecret(dvcrSettings, supGen) {
		tlsSecret := supGen.UploaderTLSSecretForIngress()
		tlsCopier := copier.Secret{
			Source: types.NamespacedName{
				Name:      dvcrSettings.UploaderIngressSettings.TLSSecret,
				Namespace: dvcrSettings.UploaderIngressSettings.TLSSecretNamespace,
			},
			Destination:    tlsSecret,
			OwnerReference: ingutil.MakeOwnerReference(ing),
		}
		if err := tlsCopier.Copy(ctx, client); err != nil {
			return err
		}
	}
	return nil
}

type DataVolumeSupplement interface {
	DataVolume() types.NamespacedName
	DVCRAuthSecretForDV() types.NamespacedName
	DVCRCABundleConfigMapForDV() types.NamespacedName
	NetworkPolicy() types.NamespacedName
}
