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

package supplements

import (
	"context"

	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// SupplementType represents the type of supplement resource
type SupplementType string

const (
	SupplementImporterPod SupplementType = "ImporterPod"
	SupplementUploaderPod SupplementType = "UploaderPod"
	SupplementBounderPod  SupplementType = "BounderPod"

	SupplementUploaderService SupplementType = "UploaderService"
	SupplementUploaderIngress SupplementType = "UploaderIngress"

	SupplementPVC        SupplementType = "PersistentVolumeClaim"
	SupplementDataVolume SupplementType = "DataVolume"

	SupplementDVCRAuthSecret        SupplementType = "DVCRAuthSecret"
	SupplementDVCRAuthSecretForDV   SupplementType = "DVCRAuthSecretForDV"
	SupplementDVCRCABundleConfigMap SupplementType = "DVCRCABundleConfigMapForDV"
	SupplementCABundleConfigMap     SupplementType = "CABundleConfigMap"
	SupplementImagePullSecret       SupplementType = "ImagePullSecret"
	SupplementUploaderTLSSecret     SupplementType = "UploaderTLSSecret"
)

// GetSupplementName returns the name for the requested supplement type
func GetSupplementName(gen Generator, supplementType SupplementType) types.NamespacedName {
	switch supplementType {
	case SupplementImporterPod:
		return gen.ImporterPod()
	case SupplementUploaderPod:
		return gen.UploaderPod()
	case SupplementBounderPod:
		return gen.BounderPod()

	case SupplementUploaderService:
		return gen.UploaderService()
	case SupplementUploaderIngress:
		return gen.UploaderIngress()

	case SupplementPVC:
		return gen.PersistentVolumeClaim()
	case SupplementDataVolume:
		return gen.DataVolume()

	case SupplementDVCRAuthSecret:
		return gen.DVCRAuthSecret()
	case SupplementDVCRAuthSecretForDV:
		return gen.DVCRAuthSecretForDV()
	case SupplementDVCRCABundleConfigMap:
		return gen.DVCRCABundleConfigMapForDV()
	case SupplementCABundleConfigMap:
		return gen.CABundleConfigMap()
	case SupplementImagePullSecret:
		return gen.ImagePullSecret()
	case SupplementUploaderTLSSecret:
		return gen.UploaderTLSSecretForIngress()

	default:
		// This should never happen if enum is used properly
		return types.NamespacedName{}
	}
}

// GetLegacySupplementName returns the legacy name for the requested supplement type
func GetLegacySupplementName(gen Generator, supplementType SupplementType) types.NamespacedName {
	switch supplementType {
	case SupplementImporterPod:
		return gen.LegacyImporterPod()
	case SupplementUploaderPod:
		return gen.LegacyUploaderPod()
	case SupplementBounderPod:
		return gen.LegacyBounderPod()

	case SupplementUploaderService:
		return gen.LegacyUploaderService()
	case SupplementUploaderIngress:
		return gen.LegacyUploaderIngress()

	case SupplementPVC:
		return gen.LegacyPersistentVolumeClaim()
	case SupplementDataVolume:
		return gen.LegacyDataVolume()

	case SupplementDVCRAuthSecret:
		return gen.LegacyDVCRAuthSecret()
	case SupplementDVCRAuthSecretForDV:
		return gen.LegacyDVCRAuthSecretForDV()
	case SupplementDVCRCABundleConfigMap:
		return gen.LegacyDVCRCABundleConfigMapForDV()
	case SupplementCABundleConfigMap:
		return gen.LegacyCABundleConfigMap()
	case SupplementImagePullSecret:
		return gen.LegacyImagePullSecret()
	case SupplementUploaderTLSSecret:
		return gen.LegacyUploaderTLSSecretForIngress()

	default:
		// This should never happen if enum is used properly
		return types.NamespacedName{}
	}
}

// FetchSupplement fetches a supplement resource with fallback to legacy naming
func FetchSupplement[T client.Object](
	ctx context.Context,
	c client.Client,
	gen Generator,
	supplementType SupplementType,
	obj T,
) (T, error) {
	var empty T

	newName := GetSupplementName(gen, supplementType)
	err := c.Get(ctx, newName, obj)
	if err == nil {
		return obj, nil
	}
	if !k8serrors.IsNotFound(err) {
		return empty, err
	}

	legacyName := GetLegacySupplementName(gen, supplementType)
	err = c.Get(ctx, legacyName, obj)
	if err != nil {
		if k8serrors.IsNotFound(err) {
			return empty, nil
		}
		return empty, err
	}

	return obj, nil
}
