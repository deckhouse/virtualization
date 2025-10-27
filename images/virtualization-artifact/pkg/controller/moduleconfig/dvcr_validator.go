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

package moduleconfig

import (
	"context"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	"github.com/deckhouse/virtualization-controller/pkg/common/object"
	mcapi "github.com/deckhouse/virtualization-controller/pkg/controller/moduleconfig/api"
)

const (
	dvcrPVCName        = "dvcr"
	dvcrNamespace      = "d8-virtualization"
	dvcrPVCStorageType = "PersistentVolumeClaim"

	// Settings field names
	dvcrField                  = "dvcr"
	storageField               = "storage"
	typeField                  = "type"
	persistentVolumeClaimField = "persistentVolumeClaim"
	storageClassNameField      = "storageClassName"
	sizeField                  = "size"
)

type dvcrValidator struct {
	client client.Client
}

func newDvcrValidator(client client.Client) *dvcrValidator {
	return &dvcrValidator{
		client: client,
	}
}

func (v dvcrValidator) ValidateUpdate(ctx context.Context, oldMC, newMC *mcapi.ModuleConfig) (admission.Warnings, error) {
	oldDvcr, err := parseDvcrSettings(oldMC.Spec.Settings)
	if err != nil {
		return nil, err
	}
	newDvcr, err := parseDvcrSettings(newMC.Spec.Settings)
	if err != nil {
		return nil, err
	}

	// Only validate PersistentVolumeClaim settings
	if newDvcr.StorageType != dvcrPVCStorageType {
		return nil, nil
	}

	// Check if DVCR PVC exists
	pvcExists, err := v.checkPVCExists(ctx)
	if err != nil {
		return nil, fmt.Errorf("internal error: unable to check DVCR PVC existence: %w", err)
	}

	// Only validate if PVC exists
	if !pvcExists {
		return nil, nil
	}

	// Validate storageClassName hasn't changed
	if oldDvcr.StorageClassName != newDvcr.StorageClassName {
		return nil, fmt.Errorf(
			"changing storageClassName for DVCR is forbidden when PVC already exists: old=%q, new=%q",
			oldDvcr.StorageClassName,
			newDvcr.StorageClassName,
		)
	}

	// Validate size hasn't been reduced
	oldSize, err := resource.ParseQuantity(oldDvcr.Size)
	if err != nil {
		return nil, fmt.Errorf("failed to parse old DVCR size %q: %w", oldDvcr.Size, err)
	}

	newSize, err := resource.ParseQuantity(newDvcr.Size)
	if err != nil {
		return nil, fmt.Errorf("failed to parse new DVCR size %q: %w", newDvcr.Size, err)
	}

	if newSize.Cmp(oldSize) < 0 {
		return nil, fmt.Errorf(
			"reducing DVCR size is forbidden when PVC already exists: old=%s, new=%s",
			oldDvcr.Size,
			newDvcr.Size,
		)
	}

	return nil, nil
}

func (v dvcrValidator) checkPVCExists(ctx context.Context) (bool, error) {
	pvc, err := object.FetchObject(ctx, types.NamespacedName{
		Name:      dvcrPVCName,
		Namespace: dvcrNamespace,
	}, v.client, &corev1.PersistentVolumeClaim{})
	if err != nil {
		return false, err
	}

	return pvc != nil, nil
}

type dvcrSettings struct {
	StorageType      string
	StorageClassName string
	Size             string
}

func parseDvcrSettings(settings mcapi.SettingsValues) (*dvcrSettings, error) {
	result := &dvcrSettings{}

	dvcr, ok := settings[dvcrField].(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("failed to parse %s settings", dvcrField)
	}

	storage, ok := dvcr[storageField].(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("failed to parse %s.%s settings", dvcrField, storageField)
	}

	storageType, ok := storage[typeField].(string)
	if !ok {
		return nil, fmt.Errorf("failed to parse %s.%s.%s", dvcrField, storageField, typeField)
	}
	result.StorageType = storageType

	// Only parse PVC fields if type is PersistentVolumeClaim
	if storageType == dvcrPVCStorageType {
		pvc, ok := storage[persistentVolumeClaimField].(map[string]interface{})
		if !ok {
			return nil, fmt.Errorf("failed to parse %s.%s.%s", dvcrField, storageField, persistentVolumeClaimField)
		}

		// storageClassName is optional
		if storageClassName, ok := pvc[storageClassNameField].(string); ok {
			result.StorageClassName = storageClassName
		}

		// size is required by OpenAPI schema
		size, ok := pvc[sizeField].(string)
		if !ok {
			return nil, fmt.Errorf("failed to parse %s.%s.%s.%s", dvcrField, storageField, persistentVolumeClaimField, sizeField)
		}
		result.Size = size
	}

	return result, nil
}
