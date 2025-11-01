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

package validators_test

import (
	"errors"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"sigs.k8s.io/controller-runtime/pkg/client"

	cvibuilder "github.com/deckhouse/virtualization-controller/pkg/builder/cvi"
	vdbuilder "github.com/deckhouse/virtualization-controller/pkg/builder/vd"
	vibuilder "github.com/deckhouse/virtualization-controller/pkg/builder/vi"
	vmbuilder "github.com/deckhouse/virtualization-controller/pkg/builder/vm"
	"github.com/deckhouse/virtualization-controller/pkg/common/testutil"
	"github.com/deckhouse/virtualization-controller/pkg/controller/supervm/internal/validators"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
)

var _ = DescribeTable("TestFirstBlockDeviceValidator", func(args firstBlockDeviceValidatorTestArgs) {
	objs := []client.Object{args.VM}
	objs = append(objs, args.Objects...)
	fakeClient := setupEnvironment(objs...)

	validator := validators.NewFirstDiskValidator(fakeClient)
	_, err := validator.Validate(testutil.ContextBackgroundWithNoOpLogger(), args.VM)
	if args.ExpectedError == nil {
		Expect(err).Should(BeNil())
	} else {
		Expect(err.Error()).Should(Equal(args.ExpectedError.Error()))
	}
},
	Entry("Has no block devices", firstBlockDeviceValidatorTestArgs{
		VM:            generateVM("vm1", "ns"),
		ExpectedError: nil,
	}),
	Entry("Has VD as first device", firstBlockDeviceValidatorTestArgs{
		VM: generateVM("vm1", "ns", v1alpha2.BlockDeviceSpecRef{
			Kind: v1alpha2.DiskDevice,
			Name: "vd1",
		}),
		Objects: []client.Object{
			generateVD("vd1", "ns"),
		},
		ExpectedError: nil,
	}),
	Entry("Has VI CDROM as first device", firstBlockDeviceValidatorTestArgs{
		VM: generateVM("vm1", "ns", v1alpha2.BlockDeviceSpecRef{
			Kind: v1alpha2.ImageDevice,
			Name: "vi1",
		}),
		Objects: []client.Object{
			generateVI("vi1", "ns", v1alpha2.ImageReady, true),
		},
		ExpectedError: nil,
	}),
	Entry("Has not CDROM VI as first device", firstBlockDeviceValidatorTestArgs{
		VM: generateVM("vm1", "ns", v1alpha2.BlockDeviceSpecRef{
			Kind: v1alpha2.ImageDevice,
			Name: "vi1",
		}),
		Objects: []client.Object{
			generateVI("vi1", "ns", v1alpha2.ImageReady, false),
		},
		ExpectedError: errors.New("a non-CDROM VirtualImage cannot occupy the first position in block devices: VirtualImage vi1 is not CDROM"),
	}),
	Entry("Has not ready VI as first device", firstBlockDeviceValidatorTestArgs{
		VM: generateVM("vm1", "ns", v1alpha2.BlockDeviceSpecRef{
			Kind: v1alpha2.ImageDevice,
			Name: "vi1",
		}),
		Objects: []client.Object{
			generateVI("vi1", "ns", v1alpha2.ImagePending, false),
		},
		ExpectedError: nil,
	}),
	Entry("Has not exists vi as first device", firstBlockDeviceValidatorTestArgs{
		VM: generateVM("vm1", "ns", v1alpha2.BlockDeviceSpecRef{
			Kind: v1alpha2.ImageDevice,
			Name: "vi1",
		}),
		ExpectedError: nil,
	}),
	Entry("Has CRDOM CVI as first device", firstBlockDeviceValidatorTestArgs{
		VM: generateVM("vm1", "ns", v1alpha2.BlockDeviceSpecRef{
			Kind: v1alpha2.ClusterImageDevice,
			Name: "cvi1",
		}),
		Objects: []client.Object{
			generateCVI("cvi1", v1alpha2.ImageReady, true),
		},
		ExpectedError: nil,
	}),
	Entry("Has not CDROM CVI as first device", firstBlockDeviceValidatorTestArgs{
		VM: generateVM("vm1", "ns", v1alpha2.BlockDeviceSpecRef{
			Kind: v1alpha2.ClusterImageDevice,
			Name: "cvi1",
		}),
		Objects: []client.Object{
			generateCVI("cvi1", v1alpha2.ImageReady, false),
		},
		ExpectedError: errors.New("a non-CDROM ClusterVirtualImage cannot occupy the first position in block devices: ClusterVirtualImage cvi1 is not CDROM"),
	}),
	Entry("Has not ready CVI as first device", firstBlockDeviceValidatorTestArgs{
		VM: generateVM("vm1", "ns1", v1alpha2.BlockDeviceSpecRef{
			Kind: v1alpha2.ClusterImageDevice,
			Name: "cvi1",
		}),
		Objects: []client.Object{
			generateCVI("cvi1", v1alpha2.ImagePending, false),
		},
		ExpectedError: nil,
	}),
	Entry("Has not exists CVI as first device", firstBlockDeviceValidatorTestArgs{
		VM: generateVM("vm", "ns", v1alpha2.BlockDeviceSpecRef{
			Kind: v1alpha2.ClusterImageDevice,
			Name: "cvi1",
		}),
		ExpectedError: nil,
	}),
)

func setupEnvironment(objs ...client.Object) client.Client {
	GinkgoHelper()

	var allObjects []client.Object
	allObjects = append(allObjects, objs...)

	fakeClient, err := testutil.NewFakeClientWithObjects(allObjects...)
	Expect(err).NotTo(HaveOccurred())

	return fakeClient
}

type firstBlockDeviceValidatorTestArgs struct {
	VM            *v1alpha2.VirtualMachine
	Objects       []client.Object
	ExpectedError error
}

func generateVM(name, namespace string, bdRefs ...v1alpha2.BlockDeviceSpecRef) *v1alpha2.VirtualMachine {
	return vmbuilder.New(
		vmbuilder.WithName(name),
		vmbuilder.WithNamespace(namespace),
		vmbuilder.WithBlockDeviceRefs(bdRefs...),
	)
}

func generateVI(name, namespace string, phase v1alpha2.ImagePhase, isCDROM bool) *v1alpha2.VirtualImage {
	return vibuilder.New(
		vibuilder.WithName(name),
		vibuilder.WithNamespace(namespace),
		vibuilder.WithPhase(phase),
		vibuilder.WithCDROM(isCDROM),
	)
}

func generateCVI(name string, phase v1alpha2.ImagePhase, isCDROM bool) *v1alpha2.ClusterVirtualImage {
	return cvibuilder.New(
		cvibuilder.WithName(name),
		cvibuilder.WithPhase(phase),
		cvibuilder.WithCDROM(isCDROM),
	)
}

func generateVD(name, namespace string) *v1alpha2.VirtualDisk {
	return vdbuilder.New(
		vdbuilder.WithName(name),
		vdbuilder.WithNamespace(namespace),
	)
}
