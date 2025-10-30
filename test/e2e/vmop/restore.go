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

package vmop

import (
	"time"

	. "github.com/onsi/ginkgo/v2"
	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/utils/ptr"

	cvibuilder "github.com/deckhouse/virtualization-controller/pkg/builder/cvi"
	vibuilder "github.com/deckhouse/virtualization-controller/pkg/builder/vi"
	vmbuilder "github.com/deckhouse/virtualization-controller/pkg/builder/vm"
	vmbdabuilder "github.com/deckhouse/virtualization-controller/pkg/builder/vmbda"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/test/e2e/internal/framework"
	"github.com/deckhouse/virtualization/test/e2e/internal/object"
)

const (
	viURL              = "https://89d64382-20df-4581-8cc7-80df331f67fa.selstorage.ru/test/test.qcow2"
	cviURL             = "https://89d64382-20df-4581-8cc7-80df331f67fa.selstorage.ru/test/test.iso"
	defaultValue       = "value"
	changedValue       = "changed"
	testAnnotationName = "test-annotation"
	testLabelName      = "test-label"
)

var _ = Describe("VirtualMachineOperationRestore", func() {
	var (
		cvi                   *v1alpha2.ClusterVirtualImage
		vi                    *v1alpha2.VirtualImage
		vdRoot                *v1alpha2.VirtualDisk
		vdBlank               *v1alpha2.VirtualDisk
		vm                    *v1alpha2.VirtualMachine
		vmbda                 *v1alpha2.VirtualMachineBlockDeviceAttachment
		vmopRestoreDryRun     *v1alpha2.VirtualMachineOperation
		vmopRestoreStrict     *v1alpha2.VirtualMachineOperation
		vmopRestoreBestEffort *v1alpha2.VirtualMachineOperation
		vmsnapshot            *v1alpha2.VirtualMachineSnapshot

		generatedValue            string
		runningLastTransitionTime time.Time

		f = framework.NewFramework("vmop-restore")
	)

	BeforeEach(func() {
		DeferCleanup(f.After)

		f.Before()
	})

	It("restores a virtual machine from a snapshot", func() {
		By("Environment preparation", func() {
			initializeEnvironment(f.Namespace().Name, cvi, vi, vdRoot, vdBlank, vm, vmbda)
		})
	})
})

func initializeEnvironment(
	namespace string,
	cvi *v1alpha2.ClusterVirtualImage,
	vi *v1alpha2.VirtualImage, vdRoot *v1alpha2.VirtualDisk,
	vdBlank *v1alpha2.VirtualDisk,
	vm *v1alpha2.VirtualMachine,
	vmbda *v1alpha2.VirtualMachineBlockDeviceAttachment,
) {
	cvi = cvibuilder.New(
		cvibuilder.WithName("cvi-test"),
		cvibuilder.WithDataSourceHTTP(cviURL, nil, nil),
	)
	vi = vibuilder.New(
		vibuilder.WithName("vi-test"),
		vibuilder.WithNamespace(namespace),
		vibuilder.WithDataSourceHTTP(viURL, nil, nil),
	)
	vdRoot = object.NewGeneratedHTTPVDUbuntu("vd-root", namespace)
	vdBlank = object.NewBlankVD("vd-blank", namespace, nil, ptr.To(resource.MustParse("51Mi")))
	vm = object.NewMinimalVM(
		"", namespace,
		vmbuilder.WithName("vm"),
		vmbuilder.WithBlockDeviceRefs(
			v1alpha2.BlockDeviceSpecRef{
				Kind: v1alpha2.DiskDevice,
				Name: vdRoot.Name,
			},
		),
		vmbuilder.WithBlockDeviceRefs(
			v1alpha2.BlockDeviceSpecRef{
				Kind: v1alpha2.ClusterImageDevice,
				Name: cvi.Name,
			},
		),
		vmbuilder.WithBlockDeviceRefs(
			v1alpha2.BlockDeviceSpecRef{
				Kind: v1alpha2.ImageDevice,
				Name: vi.Name,
			},
		),
	)
	vmbda = vmbdabuilder.New(
		vmbdabuilder.WithName("vmbda"),
		vmbdabuilder.WithVirtualMachineName(vm.Name),
		vmbdabuilder.WithBlockDeviceRef(v1alpha2.VMBDAObjectRefKindVirtualDisk, vdRoot.Name),
	)
}
