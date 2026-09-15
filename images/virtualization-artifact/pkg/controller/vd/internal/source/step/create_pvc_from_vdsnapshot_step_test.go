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

package step

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"

	"github.com/deckhouse/virtualization/api/core/v1alpha2"
)

// getUnifiedPVCSize reads nothing off the step itself, so a zero value is a complete receiver here.
var sizeStep = CreatePVCFromVDSnapshotStep{}

func vdWithSize(size *resource.Quantity) *v1alpha2.VirtualDisk {
	vd := &v1alpha2.VirtualDisk{ObjectMeta: metav1.ObjectMeta{Name: "vd"}}
	vd.Spec.PersistentVolumeClaim.Size = size
	return vd
}

func snapshotWith(capturedSize, capturedClass, dataSize, dataClass string) *v1alpha2.VirtualDiskSnapshot {
	vds := &v1alpha2.VirtualDiskSnapshot{
		ObjectMeta: metav1.ObjectMeta{Name: "vds"},
		Status: v1alpha2.VirtualDiskSnapshotStatus{
			PersistentVolumeClaimSize: capturedSize,
			StorageClassName:          capturedClass,
		},
	}
	if dataSize != "" || dataClass != "" {
		vds.Status.Data = &v1alpha2.UnifiedSnapshotterDataBinding{
			Size:             dataSize,
			StorageClassName: dataClass,
		}
	}
	return vds
}

var _ = Describe("CreatePVCFromVDSnapshotStep sizing a restore", func() {
	It("prefers the size the virtual disk asks for", func() {
		size, err := sizeStep.getUnifiedPVCSize(
			vdWithSize(ptr.To(resource.MustParse("10Gi"))),
			snapshotWith("64Mi", "sc-a", "69580Ki", "sc-b"),
		)
		Expect(err).NotTo(HaveOccurred())
		Expect(size.String()).To(Equal("10Gi"))
	})

	It("falls back to what a captured snapshot recorded", func() {
		size, err := sizeStep.getUnifiedPVCSize(vdWithSize(nil), snapshotWith("64Mi", "sc-a", "", ""))
		Expect(err).NotTo(HaveOccurred())
		Expect(size.String()).To(Equal("64Mi"))
	})

	// An import captures nothing, so this module writes neither of its own status fields — only the
	// core's status.data is filled. Without this fallback a VirtualDisk restored from an archive that
	// carries no explicit size (which is normal: a disk provisioned from a snapshot has an empty
	// spec.persistentVolumeClaim) could not be provisioned at all.
	It("falls back to the core's data binding for an imported snapshot", func() {
		size, err := sizeStep.getUnifiedPVCSize(vdWithSize(nil), snapshotWith("", "", "69580Ki", "linstor-thin-r1"))
		Expect(err).NotTo(HaveOccurred())
		Expect(size.String()).To(Equal("69580Ki"))
	})

	It("refuses when neither the disk nor the snapshot says how big it should be", func() {
		_, err := sizeStep.getUnifiedPVCSize(vdWithSize(nil), snapshotWith("", "", "", ""))
		Expect(err).To(MatchError(ContainSubstring("cannot determine the size to restore into")))
	})

	// A size the webhook cannot reject any more (it only guards create) must not reach
	// storage-foundation as storage: "0" and provision nothing.
	It("refuses a non-positive size on the disk", func() {
		_, err := sizeStep.getUnifiedPVCSize(
			vdWithSize(ptr.To(resource.MustParse("0"))),
			snapshotWith("64Mi", "sc-a", "", ""),
		)
		Expect(err).To(MatchError(ContainSubstring("must be greater than 0")))
	})
})
