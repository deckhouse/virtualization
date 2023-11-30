package supplements

import (
	"context"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	cdiv1 "kubevirt.io/containerized-data-importer-api/pkg/apis/core/v1beta1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	virtv2 "github.com/deckhouse/virtualization-controller/api/v2alpha1"
	dsutil "github.com/deckhouse/virtualization-controller/pkg/common/datasource"
	dvutil "github.com/deckhouse/virtualization-controller/pkg/common/datavolume"
	podutil "github.com/deckhouse/virtualization-controller/pkg/common/pod"
	"github.com/deckhouse/virtualization-controller/pkg/controller/supplements/copier"
	"github.com/deckhouse/virtualization-controller/pkg/dvcr"
	"github.com/deckhouse/virtualization-controller/pkg/sdk/framework/helper"
)

// EnsureForPod make supplements for importer or uploader Pod:
// - It creates ConfigMap with caBundle for http and containerImage data sources.
// - It copies DVCR auth Secret to use DVCR as destination.
func EnsureForPod(ctx context.Context, client client.Client, supGen *Generator, pod *corev1.Pod, ds *virtv2.DataSource, dvcrSettings *dvcr.Settings) error {
	// Create ConfigMap with caBundle.
	if dsutil.HasCABundle(ds) {
		caBundleCM := supGen.CABundleConfigMap()
		caBundleCopier := copier.CABundleConfigMap{
			Destination:    caBundleCM,
			OwnerReference: podutil.MakeOwnerReference(pod),
		}
		_, err := caBundleCopier.Create(ctx, client, dsutil.GetCABundle(ds))
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
	if ds != nil && ShouldCopyImagePullSecret(ds.ContainerImage, supGen.Namespace) {
		imgPull := supGen.ImagePullSecret()
		imgPullCopier := copier.Secret{
			Source: types.NamespacedName{
				Namespace: ds.ContainerImage.ImagePullSecret.Name,
				Name:      ds.ContainerImage.ImagePullSecret.Namespace,
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

func ShouldCopyDVCRAuthSecret(dvcrSettings *dvcr.Settings, supGen *Generator) bool {
	if dvcrSettings.AuthSecret == "" {
		return false
	}
	// Should copy if namespaces are different.
	return dvcrSettings.AuthSecretNamespace != supGen.Namespace
}

func ShouldCopyImagePullSecret(ctrImg *virtv2.DataSourceContainerRegistry, targetNS string) bool {
	if ctrImg == nil || ctrImg.ImagePullSecret.Name == "" {
		return false
	}

	imgPullNS := ctrImg.ImagePullSecret.Namespace

	// Should copy imagePullSecret if namespace differs from the specified namespace.
	return imgPullNS != "" && imgPullNS != targetNS
}

func EnsureForDataVolume(ctx context.Context, client client.Client, supGen *Generator, dv *cdiv1.DataVolume, dvcrSettings *dvcr.Settings) error {
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

func CleanupForDataVolume(ctx context.Context, client client.Client, supGen *Generator, dvcrSettings *dvcr.Settings) error {
	// AuthSecret has type dockerconfigjson and should be transformed, so it always copied.
	if dvcrSettings.AuthSecret != "" {
		authSecret := supGen.DVCRAuthSecretForDV()
		err := helper.CleanupByName(ctx, client, authSecret, &corev1.Secret{})
		if err != nil && !k8serrors.IsNotFound(err) {
			return err
		}
	}

	// CABundle needs transformation, so it always copied.
	if dvcrSettings.CertsSecret != "" {
		caBundleCM := supGen.DVCRCABundleConfigMapForDV()
		err := helper.CleanupByName(ctx, client, caBundleCM, &corev1.ConfigMap{})
		if err != nil && !k8serrors.IsNotFound(err) {
			return err
		}
	}

	return nil
}
