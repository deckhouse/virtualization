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
	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/utils/ptr"

	vdbuilder "github.com/deckhouse/virtualization-controller/pkg/builder/vd"
	vibuilder "github.com/deckhouse/virtualization-controller/pkg/builder/vi"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/test/e2e/internal/framework"
	"github.com/deckhouse/virtualization/test/e2e/internal/object"
	"github.com/deckhouse/virtualization/test/e2e/internal/precheck"
)

// VirtualDiskFormat verifies how disk image formats are handled when the source is a
// precreated ClusterVirtualImage:
//   - a qcow2 image can back a VirtualDisk, and a VirtualMachine boots from that disk;
//   - an ISO image cannot back a VirtualDisk, so it is consumed as a VirtualImage and a
//     VirtualMachine boots it as a CD-ROM instead.
//
// The qcow2 spec provisions its main VirtualDisk on the WFFC StorageClass, so the precheck
// label is declared on the Describe (the spec-label validator only reads container-hierarchy
// labels, not leaf It labels).
var _ = Describe("VirtualDiskFormat", Label(precheck.PrecheckDefaultStorageClass), func() {
	var (
		f   *framework.Framework
		ctx context.Context
	)

	BeforeEach(func() {
		ctx = context.Background()
		f = framework.NewFramework("")
		f.Before()
		DeferCleanup(f.After)
		setupProject(ctx, f, "vd-format")
	})

	It("provisions a VirtualDisk from a qcow2 ClusterVirtualImage and runs a VirtualMachine with a ready agent", func() {
		// The disk under test is the scenario's main resource, so it lives on the WFFC
		// storage class.
		vd := object.NewVDFromCVI("vd-qcow2", f.Namespace().Name, object.PrecreatedCVIAlpineBIOS,
			vdbuilder.WithStorageClass(defaultStorageClass()),
			vdbuilder.WithSize(ptr.To(resource.MustParse("350Mi"))))

		createVirtualDiskAndRunVM(ctx, f, vd)
	})

	It("runs a VirtualMachine from an iso ClusterVirtualImage through a VirtualImage", func() {
		// A VirtualDisk cannot be created from an ISO image, so the ISO is consumed as a
		// VirtualImage and the VirtualMachine boots it as a CD-ROM. A fixed name is used
		// (the spec has its own Project) so the observer, which is started before the
		// VirtualImage is created, watches the right object instead of an empty name.
		vi := newVirtualImageOnDVCR("vi-iso",
			vibuilder.WithDataSourceObjectRef(v1alpha2.VirtualImageObjectRefKindClusterVirtualImage, object.PrecreatedCVIUbuntuISO),
		)

		createVirtualImageAndWait(ctx, f, vi)

		runVirtualMachineFromImageUntilRunning(ctx, f, vi)
	})
})
