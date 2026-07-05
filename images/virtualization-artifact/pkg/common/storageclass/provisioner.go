/*
Copyright 2026 Flant JSC

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

// Package storageclass provides helpers to resolve the CSI provisioner that
// backs the storage of virtualization sources (VirtualDisk, VirtualImage,
// VirtualDiskSnapshot). They are used by admission webhooks to forbid creating
// a block device from a source that lives on a different CSI driver.
package storageclass

import (
	"context"
	"fmt"

	vsv1 "github.com/kubernetes-csi/external-snapshotter/client/v6/apis/volumesnapshot/v1"
	corev1 "k8s.io/api/core/v1"
	storagev1 "k8s.io/api/storage/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/deckhouse/virtualization-controller/pkg/common/annotations"
	"github.com/deckhouse/virtualization-controller/pkg/common/object"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
)

// ProvisionerOf returns the provisioner (CSI driver) of the named StorageClass.
// An empty scName yields an empty provisioner without error.
func ProvisionerOf(ctx context.Context, c client.Client, scName string) (string, error) {
	if scName == "" {
		return "", nil
	}

	sc, err := object.FetchObject(ctx, types.NamespacedName{Name: scName}, c, &storagev1.StorageClass{})
	if err != nil {
		return "", fmt.Errorf("get storage class %q: %w", scName, err)
	}
	if sc == nil {
		return "", fmt.Errorf("storage class %q was not found", scName)
	}

	return sc.Provisioner, nil
}

// ProvisionerOfVirtualDiskSnapshot resolves the provisioner that backs the
// source PVC of a VirtualDiskSnapshot. The boolean is false when it cannot be
// determined (the snapshot is missing, not ready, or its source PVC is gone).
func ProvisionerOfVirtualDiskSnapshot(ctx context.Context, c client.Client, namespace, name string) (string, bool, error) {
	vdSnapshot, err := object.FetchObject(ctx, types.NamespacedName{Namespace: namespace, Name: name}, c, &v1alpha2.VirtualDiskSnapshot{})
	if err != nil {
		return "", false, err
	}
	if vdSnapshot == nil ||
		vdSnapshot.Status.Phase != v1alpha2.VirtualDiskSnapshotPhaseReady ||
		vdSnapshot.Status.VolumeSnapshotName == "" {
		return "", false, nil
	}

	vs, err := object.FetchObject(ctx, types.NamespacedName{Namespace: namespace, Name: vdSnapshot.Status.VolumeSnapshotName}, c, &vsv1.VolumeSnapshot{})
	if err != nil {
		return "", false, err
	}
	if vs == nil || vs.Spec.Source.PersistentVolumeClaimName == nil || *vs.Spec.Source.PersistentVolumeClaimName == "" {
		return "", false, nil
	}

	pvc, err := object.FetchObject(ctx, types.NamespacedName{Namespace: namespace, Name: *vs.Spec.Source.PersistentVolumeClaimName}, c, &corev1.PersistentVolumeClaim{})
	if err != nil {
		return "", false, err
	}
	if pvc == nil {
		return "", false, nil
	}

	provisioner := pvc.Annotations[annotations.AnnStorageProvisioner]
	if provisioner == "" {
		provisioner = pvc.Annotations[annotations.AnnStorageProvisionerDeprecated]
	}

	return provisioner, provisioner != "", nil
}
