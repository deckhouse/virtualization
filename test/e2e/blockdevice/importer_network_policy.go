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

	vmbuilder "github.com/deckhouse/virtualization-controller/pkg/builder/vm"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/test/e2e/internal/framework"
	"github.com/deckhouse/virtualization/test/e2e/internal/object"
	"github.com/deckhouse/virtualization/test/e2e/internal/util"
)

var _ = Describe("ImporterNetworkPolicy", func() {
	const testName = "importer-network-policy"
	f := framework.NewFramework("")

	BeforeEach(func() {
		f.Before()
		DeferCleanup(f.After)
	})

	It("test network policy isolation for vi importer", func() {
		By("Create isolated project")
		project := object.NewIsolatedProject(testName, framework.NamespaceBasePrefix)
		err := f.CreateWithDeferredDeletion(context.Background(), project)
		Expect(err).NotTo(HaveOccurred())
		util.UntilObjectState("Deployed", framework.ShortTimeout, project)

		By("Create virtual image")
		vi := object.NewGeneratedHTTPVIUbuntu("vi-", project.Name)
		err = f.CreateWithDeferredDeletion(context.Background(), vi)
		Expect(err).NotTo(HaveOccurred())

		By("Check VI will be in ready phase")
		util.UntilObjectPhase(string(v1alpha2.ImageReady), framework.LongTimeout, vi)
	})

	It("test network policy isolation for vd importer", func() {
		By("Create isolated project")
		project := object.NewIsolatedProject(testName, framework.NamespaceBasePrefix)
		err := f.CreateWithDeferredDeletion(context.Background(), project)
		Expect(err).NotTo(HaveOccurred())
		util.UntilObjectState("Deployed", framework.ShortTimeout, project)

		By("Create virtual disk")
		vd := object.NewGeneratedHTTPVDUbuntu("vd-", project.Name)
		err = f.CreateWithDeferredDeletion(context.Background(), vd)
		Expect(err).NotTo(HaveOccurred())

		By("Create virtual machine")
		vm := object.NewMinimalVM("vm-", project.Name, vmbuilder.WithDisks(vd))
		err = f.CreateWithDeferredDeletion(context.Background(), vm)
		Expect(err).NotTo(HaveOccurred())

		By("Check VD will be in ready phase")
		util.UntilObjectPhase(string(v1alpha2.DiskReady), framework.LongTimeout, vd)
	})
})
