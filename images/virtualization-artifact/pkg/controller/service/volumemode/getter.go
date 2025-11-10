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

package volumemode

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"slices"

	corev1 "k8s.io/api/core/v1"
	storagev1 "k8s.io/api/storage/v1"
	"k8s.io/apimachinery/pkg/types"
	cdiv1 "kubevirt.io/containerized-data-importer-api/pkg/apis/core/v1beta1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/deckhouse/virtualization-controller/pkg/common/annotations"
	"github.com/deckhouse/virtualization-controller/pkg/common/object"
)

var ErrStorageProfileNotFound = errors.New("storage profile not found")

//go:generate go tool moq -rm -out mock.go . VolumeAndAccessModesGetter
type VolumeAndAccessModesGetter interface {
	GetVolumeAndAccessModes(ctx context.Context, obj client.Object, sc *storagev1.StorageClass) (corev1.PersistentVolumeMode, corev1.PersistentVolumeAccessMode, error)
}

func NewVolumeAndAccessModesGetter(client client.Client, storageProfileGetter func(ctx context.Context, name string) (*cdiv1.StorageProfile, error)) VolumeAndAccessModesGetter {
	getter := &volumeAndAccessModesGetter{
		client:               client,
		storageProfileGetter: storageProfileGetter,
	}
	if getter.storageProfileGetter == nil {
		getter.storageProfileGetter = getter.getStorageProfile
	}
	return getter
}

type volumeAndAccessModesGetter struct {
	client               client.Client
	storageProfileGetter func(ctx context.Context, name string) (*cdiv1.StorageProfile, error)
}

func (s volumeAndAccessModesGetter) GetVolumeAndAccessModes(ctx context.Context, obj client.Object, sc *storagev1.StorageClass) (corev1.PersistentVolumeMode, corev1.PersistentVolumeAccessMode, error) {
	if obj == nil {
		return "", "", errors.New("object is nil")
	}
	if sc == nil {
		return "", "", errors.New("storage class is nil")
	}

	// Priority: object > storage class > storage profile.

	// 1. Get modes from annotations on the object.
	accessMode, _ := s.parseAccessMode(obj)
	volumeMode, _ := s.parseVolumeMode(obj)

	if accessMode != "" && volumeMode != "" {
		return volumeMode, accessMode, nil
	}

	// 2. Get modes from annotations on the storage class.
	if m, exists := s.parseAccessMode(sc); accessMode == "" && exists {
		accessMode = m
	}
	if m, exists := s.parseVolumeMode(sc); volumeMode == "" && exists {
		volumeMode = m
	}

	if accessMode != "" && volumeMode != "" {
		return volumeMode, accessMode, nil
	}

	// 3. Get modes from storage profile.
	storageProfile, err := s.storageProfileGetter(ctx, sc.Name)
	if err != nil {
		return "", "", fmt.Errorf("get storage profile: %w", err)
	}

	if storageProfile == nil {
		return "", "", fmt.Errorf("storage profile %q not found: %w", sc.Name, ErrStorageProfileNotFound)
	}

	storageCaps := s.parseStorageCapabilities(storageProfile.Status)
	if accessMode == "" && storageCaps.AccessMode != "" {
		accessMode = storageCaps.AccessMode
	}
	if volumeMode == "" && storageCaps.VolumeMode != "" {
		volumeMode = storageCaps.VolumeMode
	}

	return volumeMode, accessMode, nil
}

var accessModeWeights = map[corev1.PersistentVolumeAccessMode]int{
	corev1.ReadOnlyMany:     0,
	corev1.ReadWriteOncePod: 1,
	corev1.ReadWriteOnce:    2,
	corev1.ReadWriteMany:    3,
}

var volumeModeWeights = map[corev1.PersistentVolumeMode]int{
	corev1.PersistentVolumeFilesystem: 0,
	corev1.PersistentVolumeBlock:      1,
}

func getAccessModeMax(modes []corev1.PersistentVolumeAccessMode) corev1.PersistentVolumeAccessMode {
	weight := -1
	var m corev1.PersistentVolumeAccessMode
	for _, mode := range modes {
		if accessModeWeights[mode] > weight {
			weight = accessModeWeights[mode]
			m = mode
		}
	}
	return m
}

func (s volumeAndAccessModesGetter) parseVolumeMode(obj client.Object) (corev1.PersistentVolumeMode, bool) {
	if obj == nil {
		return "", false
	}
	switch obj.GetAnnotations()[annotations.AnnVirtualDiskVolumeMode] {
	case string(corev1.PersistentVolumeBlock):
		return corev1.PersistentVolumeBlock, true
	case string(corev1.PersistentVolumeFilesystem):
		return corev1.PersistentVolumeFilesystem, true
	default:
		return "", false
	}
}

func (s volumeAndAccessModesGetter) parseAccessMode(obj client.Object) (corev1.PersistentVolumeAccessMode, bool) {
	if obj == nil {
		return "", false
	}
	switch obj.GetAnnotations()[annotations.AnnVirtualDiskAccessMode] {
	case string(corev1.ReadWriteOnce):
		return corev1.ReadWriteOnce, true
	case string(corev1.ReadWriteMany):
		return corev1.ReadWriteMany, true
	default:
		return "", false
	}
}

func (s volumeAndAccessModesGetter) parseStorageCapabilities(status cdiv1.StorageProfileStatus) StorageCapabilities {
	var storageCapabilities []StorageCapabilities
	for _, cp := range status.ClaimPropertySets {
		var mode corev1.PersistentVolumeMode
		if cp.VolumeMode == nil || *cp.VolumeMode == "" {
			mode = corev1.PersistentVolumeFilesystem
		} else {
			mode = *cp.VolumeMode
		}
		storageCapabilities = append(storageCapabilities, StorageCapabilities{
			AccessMode: getAccessModeMax(cp.AccessModes),
			VolumeMode: mode,
		})
	}
	slices.SortFunc(storageCapabilities, func(a, b StorageCapabilities) int {
		if c := cmp.Compare(accessModeWeights[a.AccessMode], accessModeWeights[b.AccessMode]); c != 0 {
			return c
		}
		return cmp.Compare(volumeModeWeights[a.VolumeMode], volumeModeWeights[b.VolumeMode])
	})

	if len(storageCapabilities) == 0 {
		return StorageCapabilities{
			AccessMode: corev1.ReadWriteOnce,
			VolumeMode: corev1.PersistentVolumeFilesystem,
		}
	}

	return storageCapabilities[len(storageCapabilities)-1]
}

func (s volumeAndAccessModesGetter) getStorageProfile(ctx context.Context, name string) (*cdiv1.StorageProfile, error) {
	return object.FetchObject(ctx, types.NamespacedName{Name: name}, s.client, &cdiv1.StorageProfile{})
}

type StorageCapabilities struct {
	AccessMode corev1.PersistentVolumeAccessMode
	VolumeMode corev1.PersistentVolumeMode
}

func (cp StorageCapabilities) IsEmpty() bool {
	return cp.AccessMode == "" && cp.VolumeMode == ""
}
