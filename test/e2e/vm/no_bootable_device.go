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

package vm

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"
	crclient "sigs.k8s.io/controller-runtime/pkg/client"

	vmbuilder "github.com/deckhouse/virtualization-controller/pkg/builder/vm"
	"github.com/deckhouse/virtualization-controller/pkg/controller/conditions"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/vmcondition"
	"github.com/deckhouse/virtualization/test/e2e/internal/framework"
	"github.com/deckhouse/virtualization/test/e2e/internal/object"
	"github.com/deckhouse/virtualization/test/e2e/internal/precheck"
	"github.com/deckhouse/virtualization/test/e2e/internal/util"
)

var _ = Describe("VirtualMachineNoBootableDevice", Label(precheck.NoPrecheck), func() {
	var (
		f   *framework.Framework
		ctx context.Context
	)

	BeforeEach(func() {
		ctx = context.Background()
		f = framework.NewFramework("vm-no-bootable-device")
		DeferCleanup(f.After)
		f.Before()
	})

	It("sets Running condition reason to NoBootableDevice", func() {
		By("Generating a blank disk and virtual machine with no bootable devices")
		vdBlank := object.NewBlankVD("vd-blank", f.Namespace().Name, nil, ptr.To(resource.MustParse("100Mi")))

		vm := object.NewMinimalVM("vm-", f.Namespace().Name,
			vmbuilder.WithBlockDeviceRefs(v1alpha2.BlockDeviceSpecRef{
				Kind: v1alpha2.DiskDevice,
				Name: vdBlank.Name,
			}),
		)

		By("Creating resources")
		err := f.CreateWithDeferredDeletion(ctx, vdBlank, vm)
		Expect(err).NotTo(HaveOccurred())

		By("Waiting for virtual machine to be Running")
		util.UntilObjectPhase(ctx, string(v1alpha2.MachineRunning), framework.LongTimeout, vm)

		By("Checking Running condition reason indicates no bootable device")
		Eventually(func(g Gomega) {
			err = f.GenericClient().Get(ctx, crclient.ObjectKeyFromObject(vm), vm)
			g.Expect(err).NotTo(HaveOccurred())

			runningCondition, found := conditions.GetCondition(vmcondition.TypeRunning, vm.Status.Conditions)
			g.Expect(found).To(BeTrue())
			g.Expect(runningCondition.Reason).To(Equal(vmcondition.ReasonNoBootableDeviceFound.String()))
			g.Expect(runningCondition.Status).To(Equal(metav1.ConditionTrue))
		}).WithTimeout(framework.LongTimeout).WithPolling(framework.PollingInterval).Should(Succeed())
	})
})
