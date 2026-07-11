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

package blockdevice

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/utils/ptr"

	vdbuilder "github.com/deckhouse/virtualization-controller/pkg/builder/vd"
	vmbuilder "github.com/deckhouse/virtualization-controller/pkg/builder/vm"
	"github.com/deckhouse/virtualization/test/e2e/internal/framework"
	"github.com/deckhouse/virtualization/test/e2e/internal/object"
	vdobs "github.com/deckhouse/virtualization/test/e2e/internal/observer/vd"
	viobs "github.com/deckhouse/virtualization/test/e2e/internal/observer/vi"
	"github.com/deckhouse/virtualization/test/e2e/internal/precheck"
	"github.com/deckhouse/virtualization/test/e2e/internal/util"
)

var _ = Describe("ImporterNetworkPolicy", Label(precheck.NoPrecheck), func() {
	const testName = "importer-network-policy"

	var (
		f   *framework.Framework
		ctx context.Context
	)
	BeforeEach(func() {
		ctx = context.Background()
		f = framework.NewFramework("")
		f.Before()
		DeferCleanup(f.After)
	})

	It("test network policy isolation for vi importer", func() {
		By("Create isolated project")
		project := object.NewIsolatedProject(testName, framework.NamespaceBasePrefix)
		err := f.CreateWithDeferredDeletion(ctx, project)
		Expect(err).NotTo(HaveOccurred())
		// EXCEPTION: Project (deckhouse.io) has no typed client in VirtClient and
		// therefore no Observer; wait for its state via the generic helper.
		util.UntilObjectState(ctx, "Deployed", framework.ShortTimeout, project)

		By("Create virtual image")
		vi := object.NewGeneratedHTTPVICustomBIOS("vi-", project.Name)
		err = f.CreateWithDeferredDeletion(ctx, vi)
		Expect(err).NotTo(HaveOccurred())

		By("Check VI reaches the Ready phase", func() {
			viObs := viobs.StartObserver(ctx, f, vi)
			viObs.Never(viobs.BeFailed())
			Expect(viObs.WaitFor(viobs.BeReady(), framework.LongTimeout)).To(Succeed())
		})
	})

	It("test network policy isolation for vd importer", func() {
		By("Create isolated project")
		project := object.NewIsolatedProject(testName, framework.NamespaceBasePrefix)
		err := f.CreateWithDeferredDeletion(ctx, project)
		Expect(err).NotTo(HaveOccurred())
		// EXCEPTION: Project (deckhouse.io) has no typed client in VirtClient and
		// therefore no Observer; wait for its state via the generic helper.
		util.UntilObjectState(ctx, "Deployed", framework.ShortTimeout, project)

		By("Create virtual disk")
		vd := object.NewHTTPVDCustomBIOS("vd", project.Name, vdbuilder.WithSize(ptr.To(resource.MustParse(vdCreationImageSize))))
		err = f.CreateWithDeferredDeletion(ctx, vd)
		Expect(err).NotTo(HaveOccurred())

		By("Create virtual machine")
		// The custom e2e-br image has no cloud-init; this VM is only the disk
		// consumer that unparks a WaitForFirstConsumer disk, so provision nothing.
		vm := object.NewMinimalVM("vm-", project.Name, vmbuilder.WithDisks(vd), vmbuilder.WithProvisioning(nil))
		err = f.CreateWithDeferredDeletion(ctx, vm)
		Expect(err).NotTo(HaveOccurred())

		By("Check VD reaches the Ready phase", func() {
			vdObs := vdobs.StartObserver(ctx, f, vd)
			vdObs.Never(vdobs.BeFailed())
			Expect(vdObs.WaitFor(vdobs.BeReady(), framework.LongTimeout)).To(Succeed())
		})
	})
})
