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

	vdbuilder "github.com/deckhouse/virtualization-controller/pkg/builder/vd"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/test/e2e/internal/framework"
	"github.com/deckhouse/virtualization/test/e2e/internal/label"
	"github.com/deckhouse/virtualization/test/e2e/internal/object"
	"github.com/deckhouse/virtualization/test/e2e/internal/precheck"
	"github.com/deckhouse/virtualization/test/e2e/internal/util"
)

var _ = Describe("VirtualDiskPhaseTransitions", label.Slow(), Label(precheck.PrecheckImmediateStorageClass), func() {
	var f *framework.Framework

	BeforeEach(func() {
		f = framework.NewFramework("vd-phase-transitions")
		f.Before()
		DeferCleanup(f.After)
	})

	It("tracks phase transitions during export", func() {
		var vd *v1alpha2.VirtualDisk

		// Get immediate storage class for the test to ensure disk becomes Ready immediately
		sc := framework.GetConfig().StorageClass.ImmediateStorageClass
		Expect(sc).NotTo(BeNil(), "immediate storage class is required for this test")

		By("Creating VirtualDisk from CVI", func() {
			vd = object.NewVDFromCVI("vd-test", f.Namespace().Name, object.PrecreatedCVIAlpineBIOS,
				vdbuilder.WithPersistentVolumeClaim(&sc.Name, nil))

			err := f.CreateWithDeferredDeletion(context.Background(), vd)
			Expect(err).NotTo(HaveOccurred())
		})

		By("Waiting for VirtualDisk to become Ready", func() {
			util.UntilObjectPhase(string(v1alpha2.DiskReady), framework.LongTimeout, vd)
		})

		By("Starting event-based phase watcher", func() {
			watcher := util.WatchPhases(context.Background(), vd)
			Expect(watcher).NotTo(BeNil(), "failed to create event watcher for VirtualDisk")

			// Register defer early to ensure verification runs even if test fails later
			defer util.VerifyPhaseTransitions(watcher,
				string(v1alpha2.DiskReady),
				string(v1alpha2.DiskExporting),
				string(v1alpha2.DiskReady))

			By("Exporting VirtualDisk", func() {
				DataExport(f, "vd", vd.Name, "exported-disk-phases.img")
			})

			By("Waiting for VirtualDisk to return to Ready after export", func() {
				util.UntilObjectPhase(string(v1alpha2.DiskReady), framework.LongTimeout, vd)
			})
		})
	})
})
