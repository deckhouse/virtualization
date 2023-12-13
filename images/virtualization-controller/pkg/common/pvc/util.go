package pvc

import (
	"os"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
)

// ForceDVReadWriteOnceVar enables accessMode ReadWriteOnce for testing in local environments.
const ForceDVReadWriteOnceVar = "FORCE_DV_READ_WRITE_ONCE"

// CreateSpecReadWriteMany returns pvc spec with accessMode ReadWriteMany and volumeMode Block.
func CreateSpecReadWriteMany(storageClassName *string, size resource.Quantity) *corev1.PersistentVolumeClaimSpec {
	mode := corev1.PersistentVolumeBlock

	return &corev1.PersistentVolumeClaimSpec{
		StorageClassName: storageClassName,
		AccessModes:      []corev1.PersistentVolumeAccessMode{corev1.ReadWriteMany},
		Resources: corev1.ResourceRequirements{
			Requests: corev1.ResourceList{
				corev1.ResourceStorage: size,
			},
		},
		VolumeMode: &mode,
	}
}

// CreateSpecReadWriteOnce returns pvc spec with accessMode ReadWriteOnce.
func CreateSpecReadWriteOnce(storageClassName *string, size resource.Quantity) *corev1.PersistentVolumeClaimSpec {
	return &corev1.PersistentVolumeClaimSpec{
		StorageClassName: storageClassName,
		AccessModes:      []corev1.PersistentVolumeAccessMode{corev1.ReadWriteOnce},
		Resources: corev1.ResourceRequirements{
			Requests: corev1.ResourceList{
				corev1.ResourceStorage: size,
			},
		},
	}
}

// CreateSpecForDataVolume returns pvc spec with accessMode ReadWriteMany or
// with accessMode ReadWriteOnce, depending on environment variable.
func CreateSpecForDataVolume(storageClassName *string, size resource.Quantity) *corev1.PersistentVolumeClaimSpec {
	if os.Getenv(ForceDVReadWriteOnceVar) == "yes" {
		return CreateSpecReadWriteOnce(storageClassName, size)
	}
	return CreateSpecReadWriteMany(storageClassName, size)
}
