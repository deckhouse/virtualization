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

package blockdevice

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	storagev1 "k8s.io/api/storage/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"
	crclient "sigs.k8s.io/controller-runtime/pkg/client"

	storagev1alpha1 "github.com/deckhouse/virtualization-controller/pkg/apis/storage/v1alpha1"
	"github.com/deckhouse/virtualization-controller/pkg/common/imageformat"
	"github.com/deckhouse/virtualization-controller/pkg/controller/service/volumemode"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/test/e2e/internal/framework"
	"github.com/deckhouse/virtualization/test/e2e/internal/object"
	"github.com/deckhouse/virtualization/test/e2e/internal/rewrite"
)

// expectedFormatForStorageClass returns the on-disk image format for a PVC backed by
// scName. Block storage classes hold a flat raw disk; filesystem storage classes keep
// a qcow2 file. The same rule applies to VirtualImages and VirtualDisks on PVC.
func expectedFormatForStorageClass(ctx context.Context, f *framework.Framework, scName string) string {
	GinkgoHelper()

	if storageClassVolumeMode(ctx, f, scName) == corev1.PersistentVolumeBlock {
		return imageformat.FormatRAW
	}
	return imageformat.FormatQCOW2
}

// expectedVirtualImageFormat returns the image format actually stored for vi.
// On PVC the format follows the target storage class volume mode. On DVCR,
// object-ref imports that copy from a block volume inherit the source storage
// class format; file-based imports (HTTP, registry, CVI, upload) keep qcow2.
func expectedVirtualImageFormat(ctx context.Context, f *framework.Framework, vi *v1alpha2.VirtualImage) string {
	GinkgoHelper()

	if vi.Spec.DataSource.HTTP != nil && vi.Spec.DataSource.HTTP.URL == object.ImageURLUbuntuISO {
		return imageformat.FormatISO
	}
	if vi.Spec.DataSource.ObjectRef != nil &&
		vi.Spec.DataSource.ObjectRef.Kind == v1alpha2.VirtualImageObjectRefKindClusterVirtualImage &&
		vi.Spec.DataSource.ObjectRef.Name == object.PrecreatedCVIUbuntuISO {
		return imageformat.FormatISO
	}

	if vi.Spec.Storage == v1alpha2.StoragePersistentVolumeClaim {
		return expectedFormatForStorageClass(ctx, f, ptr.Deref(vi.Spec.PersistentVolumeClaim.StorageClass, ""))
	}

	if scName := virtualImageSourceStorageClassName(ctx, f, vi); scName != "" {
		return expectedFormatForStorageClass(ctx, f, scName)
	}

	return imageformat.FormatQCOW2
}

func virtualImageSourceStorageClassName(ctx context.Context, f *framework.Framework, vi *v1alpha2.VirtualImage) string {
	GinkgoHelper()

	ref := vi.Spec.DataSource.ObjectRef
	if ref == nil {
		return ""
	}

	switch ref.Kind {
	case v1alpha2.VirtualImageObjectRefKindVirtualDiskSnapshot:
		vdSnapshot := &v1alpha2.VirtualDiskSnapshot{}
		err := f.Clients.GenericClient().Get(ctx, crclient.ObjectKey{Namespace: vi.Namespace, Name: ref.Name}, vdSnapshot)
		Expect(err).NotTo(HaveOccurred())
		return virtualDiskStorageClassName(ctx, f, vdSnapshot.Namespace, vdSnapshot.Spec.VirtualDiskName)

	case v1alpha2.VirtualImageObjectRefKindVirtualDisk:
		return virtualDiskStorageClassName(ctx, f, vi.Namespace, ref.Name)

	case v1alpha2.VirtualImageObjectRefKindVirtualImage:
		sourceVI := &v1alpha2.VirtualImage{}
		err := f.Clients.GenericClient().Get(ctx, crclient.ObjectKey{Namespace: vi.Namespace, Name: ref.Name}, sourceVI)
		Expect(err).NotTo(HaveOccurred())
		if sourceVI.Spec.Storage != v1alpha2.StoragePersistentVolumeClaim {
			return ""
		}
		return ptr.Deref(sourceVI.Spec.PersistentVolumeClaim.StorageClass, "")

	default:
		return ""
	}
}

func virtualDiskStorageClassName(ctx context.Context, f *framework.Framework, namespace, name string) string {
	GinkgoHelper()

	vd := &v1alpha2.VirtualDisk{}
	err := f.Clients.GenericClient().Get(ctx, crclient.ObjectKey{Namespace: namespace, Name: name}, vd)
	Expect(err).NotTo(HaveOccurred())

	if vd.Status.StorageClassName != "" {
		return vd.Status.StorageClassName
	}
	return ptr.Deref(vd.Spec.PersistentVolumeClaim.StorageClass, "")
}

func storageClassVolumeMode(ctx context.Context, f *framework.Framework, scName string) corev1.PersistentVolumeMode {
	GinkgoHelper()

	sc := storageClassByName(ctx, f, scName)
	modeGetter := volumemode.NewVolumeAndAccessModesGetter(f.GenericClient(), storageProfileGetter(f))
	mode, _, err := modeGetter.GetVolumeAndAccessModes(ctx, sc, sc)
	Expect(err).NotTo(HaveOccurred(), "failed to resolve volume mode for StorageClass %q", sc.Name)
	return mode
}

func storageClassByName(ctx context.Context, f *framework.Framework, name string) *storagev1.StorageClass {
	GinkgoHelper()

	if name == "" {
		sc := framework.GetConfig().StorageClass.DefaultStorageClass
		Expect(sc).NotTo(BeNil(), "default StorageClass not found")
		return sc
	}

	got, err := f.KubeClient().StorageV1().StorageClasses().Get(ctx, name, metav1.GetOptions{})
	Expect(err).NotTo(HaveOccurred(), "failed to get StorageClass %q", name)
	return got
}

func storageProfileGetter(f *framework.Framework) func(ctx context.Context, name string) (*storagev1alpha1.StorageProfile, error) {
	return func(ctx context.Context, name string) (*storagev1alpha1.StorageProfile, error) {
		obj := &rewrite.StorageProfile{}
		err := f.RewriteClient().Get(ctx, name, obj)
		if err != nil {
			return nil, err
		}
		return obj.StorageProfile, nil
	}
}
