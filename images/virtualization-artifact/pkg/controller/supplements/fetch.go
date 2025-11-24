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
	"fmt"

	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// SupplementType represents the type of supplement resource
type SupplementType string

const (
	// Pods
	SupplementImporterPod SupplementType = "ImporterPod"
	SupplementUploaderPod SupplementType = "UploaderPod"
	SupplementBounderPod  SupplementType = "BounderPod"

	// Network
	SupplementUploaderService SupplementType = "UploaderService"
	SupplementUploaderIngress SupplementType = "UploaderIngress"

	// Volumes
	SupplementPVC        SupplementType = "PersistentVolumeClaim"
	SupplementDataVolume SupplementType = "DataVolume"

	// ConfigMaps/Secrets
	SupplementDVCRAuthSecret        SupplementType = "DVCRAuthSecret"
	SupplementDVCRAuthSecretForDV   SupplementType = "DVCRAuthSecretForDV"
	SupplementDVCRCABundleConfigMap SupplementType = "DVCRCABundleConfigMapForDV"
	SupplementCABundleConfigMap     SupplementType = "CABundleConfigMap"
	SupplementImagePullSecret       SupplementType = "ImagePullSecret"
	SupplementUploaderTLSSecret     SupplementType = "UploaderTLSSecret"
)

// GetSupplementName returns the name for the requested supplement type
func GetSupplementName(gen Generator, supplementType SupplementType) (types.NamespacedName, error) {
	switch supplementType {
	// Pods
	case SupplementImporterPod:
		return gen.ImporterPod(), nil
	case SupplementUploaderPod:
		return gen.UploaderPod(), nil
	case SupplementBounderPod:
		return gen.BounderPod(), nil

	// Network
	case SupplementUploaderService:
		return gen.UploaderService(), nil
	case SupplementUploaderIngress:
		return gen.UploaderIngress(), nil

	// Volumes
	case SupplementPVC:
		return gen.PersistentVolumeClaim(), nil
	case SupplementDataVolume:
		return gen.DataVolume(), nil

	// ConfigMaps/Secrets
	case SupplementDVCRAuthSecret:
		return gen.DVCRAuthSecret(), nil
	case SupplementDVCRAuthSecretForDV:
		return gen.DVCRAuthSecretForDV(), nil
	case SupplementDVCRCABundleConfigMap:
		return gen.DVCRCABundleConfigMapForDV(), nil
	case SupplementCABundleConfigMap:
		return gen.CABundleConfigMap(), nil
	case SupplementImagePullSecret:
		return gen.ImagePullSecret(), nil
	case SupplementUploaderTLSSecret:
		return gen.UploaderTLSSecretForIngress(), nil

	default:
		return types.NamespacedName{}, fmt.Errorf("unknown supplement type: %s", supplementType)
	}
}

// GetLegacySupplementName returns the legacy name for the requested supplement type
func GetLegacySupplementName(gen Generator, supplementType SupplementType) (types.NamespacedName, error) {
	switch supplementType {
	// Pods
	case SupplementImporterPod:
		return gen.LegacyImporterPod(), nil
	case SupplementUploaderPod:
		return gen.LegacyUploaderPod(), nil
	case SupplementBounderPod:
		return gen.LegacyBounderPod(), nil

	// Network
	case SupplementUploaderService:
		return gen.LegacyUploaderService(), nil
	case SupplementUploaderIngress:
		return gen.LegacyUploaderIngress(), nil

	// Volumes
	case SupplementPVC:
		return gen.LegacyPersistentVolumeClaim(), nil
	case SupplementDataVolume:
		return gen.LegacyDataVolume(), nil

	// ConfigMaps/Secrets
	case SupplementDVCRAuthSecret:
		return gen.LegacyDVCRAuthSecret(), nil
	case SupplementDVCRAuthSecretForDV:
		return gen.LegacyDVCRAuthSecretForDV(), nil
	case SupplementDVCRCABundleConfigMap:
		return gen.LegacyDVCRCABundleConfigMapForDV(), nil
	case SupplementCABundleConfigMap:
		return gen.LegacyCABundleConfigMap(), nil
	case SupplementImagePullSecret:
		return gen.LegacyImagePullSecret(), nil
	case SupplementUploaderTLSSecret:
		return gen.LegacyUploaderTLSSecretForIngress(), nil

	default:
		return types.NamespacedName{}, fmt.Errorf("unknown supplement type: %s", supplementType)
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

	newName, err := GetSupplementName(gen, supplementType)
	if err != nil {
		return empty, err
	}

	err = c.Get(ctx, newName, obj)
	if err == nil {
		return obj, nil
	}
	if !k8serrors.IsNotFound(err) {
		return empty, err
	}

	legacyName, err := GetLegacySupplementName(gen, supplementType)
	if err != nil {
		return empty, err
	}

	err = c.Get(ctx, legacyName, obj)
	if err != nil {
		if k8serrors.IsNotFound(err) {
			return empty, nil
		}
		return empty, err
	}

	return obj, nil
}
