/*
Copyright 2024 Flant JSC

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

package internal

import (
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	cvibuilder "github.com/deckhouse/virtualization-controller/pkg/builder/cvi"
	vdbuilder "github.com/deckhouse/virtualization-controller/pkg/builder/vd"
	vibuilder "github.com/deckhouse/virtualization-controller/pkg/builder/vi"
	vmbdabuilder "github.com/deckhouse/virtualization-controller/pkg/builder/vmbda"
	"github.com/deckhouse/virtualization-controller/pkg/common/testutil"
	"github.com/deckhouse/virtualization-controller/pkg/controller/conditions"
	virtv2 "github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/cvicondition"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/vicondition"
)

var _ = DescribeTable("InUseHandler Handle", func(args inUseHandlerTestArgs) {
	vi := &virtv2.VirtualImage{
		ObjectMeta: metav1.ObjectMeta{
			Name:              args.VINamespacedName.Name,
			Namespace:         args.VINamespacedName.Namespace,
			DeletionTimestamp: args.DeletionTimestamp,
		},
		Status: virtv2.VirtualImageStatus{
			Conditions: []metav1.Condition{
				{
					Type:   vicondition.ReadyType.String(),
					Reason: vicondition.Ready.String(),
					Status: metav1.ConditionTrue,
				},
			},
		},
	}

	var objects []client.Object
	for _, vm := range args.VMs {
		objects = append(objects, &vm)
	}

	for _, vmbda := range args.VMBDAs {
		objects = append(objects, &vmbda)
	}

	for _, vd := range args.VDs {
		objects = append(objects, &vd)
	}

	for _, vi := range args.VIs {
		objects = append(objects, &vi)
	}

	for _, cvi := range args.CVIs {
		objects = append(objects, &cvi)
	}

	fakeClient, err := testutil.NewFakeClientWithObjects(objects...)
	Expect(err).ShouldNot(HaveOccurred())
	handler := NewInUseHandler(fakeClient)

	result, err := handler.Handle(testutil.ContextBackgroundWithNoOpLogger(), vi)
	Expect(err).To(BeNil())
	Expect(result).To(Equal(reconcile.Result{}))
	inUseCondition, _ := conditions.GetCondition(vicondition.InUseType, vi.Status.Conditions)
	Expect(inUseCondition.Status).To(Equal(args.ExpectedConditionStatus))
	Expect(inUseCondition.Reason).To(Equal(args.ExpectedConditionReason))
},
	Entry("deletionTimestamp exists but no one uses VI", inUseHandlerTestArgs{
		VMs: []virtv2.VirtualMachine{},
		VINamespacedName: types.NamespacedName{
			Name:      "test",
			Namespace: "ns",
		},
		DeletionTimestamp:       ptr.To(metav1.Time{Time: time.Now()}),
		ExpectedConditionStatus: metav1.ConditionFalse,
		ExpectedConditionReason: vicondition.NotInUse.String(),
	}),
	Entry("has 1 VirtualMachine with connected terminating VI", inUseHandlerTestArgs{
		VINamespacedName: types.NamespacedName{
			Name:      "test",
			Namespace: "ns",
		},
		DeletionTimestamp: ptr.To(metav1.Time{Time: time.Now()}),
		VMs: []virtv2.VirtualMachine{
			generateVMForInUseTest("test-vm", "ns", []virtv2.BlockDeviceStatusRef{
				{
					Kind: virtv2.VirtualImageKind,
					Name: "test",
				},
			}),
		},
		ExpectedConditionStatus: metav1.ConditionTrue,
		ExpectedConditionReason: vicondition.InUse.String(),
	}),
	Entry("has 2 VirtualMachines with connected terminating VI", inUseHandlerTestArgs{
		VINamespacedName: types.NamespacedName{
			Name:      "test",
			Namespace: "ns",
		},
		DeletionTimestamp: ptr.To(metav1.Time{Time: time.Now()}),
		VMs: []virtv2.VirtualMachine{
			generateVMForInUseTest("test-vm", "ns", []virtv2.BlockDeviceStatusRef{
				{
					Kind: virtv2.VirtualImageKind,
					Name: "test",
				},
			}),
			generateVMForInUseTest("test-vm2", "ns", []virtv2.BlockDeviceStatusRef{
				{
					Kind: virtv2.VirtualImageKind,
					Name: "test",
				},
			}),
		},
		ExpectedConditionStatus: metav1.ConditionTrue,
		ExpectedConditionReason: vicondition.InUse.String(),
	}),
	Entry("has 5 VirtualMachines with connected terminating VI", inUseHandlerTestArgs{
		VINamespacedName: types.NamespacedName{
			Name:      "test",
			Namespace: "ns",
		},
		DeletionTimestamp: ptr.To(metav1.Time{Time: time.Now()}),
		VMs: []virtv2.VirtualMachine{
			generateVMForInUseTest("test-vm", "ns", []virtv2.BlockDeviceStatusRef{
				{
					Kind: virtv2.VirtualImageKind,
					Name: "test",
				},
			}),
			generateVMForInUseTest("test-vm2", "ns", []virtv2.BlockDeviceStatusRef{
				{
					Kind: virtv2.VirtualImageKind,
					Name: "test",
				},
			}),
			generateVMForInUseTest("test-vm3", "ns", []virtv2.BlockDeviceStatusRef{
				{
					Kind: virtv2.VirtualImageKind,
					Name: "test",
				},
			}),
			generateVMForInUseTest("test-vm4", "ns", []virtv2.BlockDeviceStatusRef{
				{
					Kind: virtv2.VirtualImageKind,
					Name: "test",
				},
			}),
			generateVMForInUseTest("test-vm5", "ns", []virtv2.BlockDeviceStatusRef{
				{
					Kind: virtv2.VirtualImageKind,
					Name: "test",
				},
			}),
			generateVMForInUseTest("test-vm-imposter", "ns-imposter", []virtv2.BlockDeviceStatusRef{
				{
					Kind: virtv2.VirtualImageKind,
					Name: "test",
				},
			}),
			{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-vm-stopped",
					Namespace: "ns",
				},
				TypeMeta: metav1.TypeMeta{
					Kind: virtv2.VirtualMachineKind,
				},
				Status: virtv2.VirtualMachineStatus{
					Phase: virtv2.MachineStopped,
					BlockDeviceRefs: []virtv2.BlockDeviceStatusRef{
						{
							Kind: virtv2.VirtualImageKind,
							Name: "test",
						},
					},
				},
			},
		},
		ExpectedConditionStatus: metav1.ConditionTrue,
		ExpectedConditionReason: vicondition.InUse.String(),
	}),
	Entry("has 5 VM with connected terminating VI, 1 VMBDA, 4 VD, 2 CVI, 1 VI", inUseHandlerTestArgs{
		VINamespacedName: types.NamespacedName{
			Name:      "test",
			Namespace: "ns",
		},
		DeletionTimestamp: ptr.To(metav1.Time{Time: time.Now()}),
		VMs: []virtv2.VirtualMachine{
			generateVMForInUseTest("test-vm", "ns", []virtv2.BlockDeviceStatusRef{
				{
					Kind: virtv2.VirtualImageKind,
					Name: "test",
				},
			}),
			generateVMForInUseTest("test-vm2", "ns", []virtv2.BlockDeviceStatusRef{
				{
					Kind: virtv2.VirtualImageKind,
					Name: "test",
				},
			}),
			generateVMForInUseTest("test-vm3", "ns", []virtv2.BlockDeviceStatusRef{
				{
					Kind: virtv2.VirtualImageKind,
					Name: "test",
				},
			}),
			generateVMForInUseTest("test-vm4", "ns", []virtv2.BlockDeviceStatusRef{
				{
					Kind: virtv2.VirtualImageKind,
					Name: "test",
				},
			}),
			generateVMForInUseTest("test-vm5", "ns", []virtv2.BlockDeviceStatusRef{
				{
					Kind: virtv2.VirtualImageKind,
					Name: "test",
				},
			}),
		},
		VDs: []virtv2.VirtualDisk{
			generateVDForInUseTest("test1", "ns", virtv2.VirtualDiskDataSource{
				Type: virtv2.DataSourceTypeObjectRef,
				ObjectRef: &virtv2.VirtualDiskObjectRef{
					Kind: virtv2.VirtualImageKind,
					Name: "test",
				},
			}),
			generateVDForInUseTest("test2", "ns", virtv2.VirtualDiskDataSource{
				Type: virtv2.DataSourceTypeObjectRef,
				ObjectRef: &virtv2.VirtualDiskObjectRef{
					Kind: virtv2.VirtualImageKind,
					Name: "test",
				},
			}),
			generateVDForInUseTest("test3", "ns", virtv2.VirtualDiskDataSource{
				Type: virtv2.DataSourceTypeObjectRef,
				ObjectRef: &virtv2.VirtualDiskObjectRef{
					Kind: virtv2.VirtualImageKind,
					Name: "test",
				},
			}),
			generateVDForInUseTest("test4", "ns", virtv2.VirtualDiskDataSource{
				Type: virtv2.DataSourceTypeObjectRef,
				ObjectRef: &virtv2.VirtualDiskObjectRef{
					Kind: virtv2.VirtualImageKind,
					Name: "test",
				},
			}),
			generateVDForInUseTest("test5", "ns2", virtv2.VirtualDiskDataSource{
				Type: virtv2.DataSourceTypeObjectRef,
				ObjectRef: &virtv2.VirtualDiskObjectRef{
					Kind: virtv2.VirtualImageKind,
					Name: "test",
				},
			}),
		},
		VIs: []virtv2.VirtualImage{
			generateVIForInUseTest("test1", "ns", virtv2.VirtualImageDataSource{
				Type: virtv2.DataSourceTypeObjectRef,
				ObjectRef: &virtv2.VirtualImageObjectRef{
					Kind: virtv2.VirtualImageKind,
					Name: "test",
				},
			}),
		},
		CVIs: []virtv2.ClusterVirtualImage{
			generateCVIForInUseTest("test1", virtv2.ClusterVirtualImageDataSource{
				Type: virtv2.DataSourceTypeObjectRef,
				ObjectRef: &virtv2.ClusterVirtualImageObjectRef{
					Kind:      virtv2.VirtualImageKind,
					Name:      "test",
					Namespace: "ns",
				},
			}),
			generateCVIForInUseTest("test2", virtv2.ClusterVirtualImageDataSource{
				Type: virtv2.DataSourceTypeObjectRef,
				ObjectRef: &virtv2.ClusterVirtualImageObjectRef{
					Kind:      virtv2.VirtualImageKind,
					Name:      "test",
					Namespace: "ns",
				},
			}),
			generateCVIForInUseTest("test3", virtv2.ClusterVirtualImageDataSource{
				Type: virtv2.DataSourceTypeObjectRef,
				ObjectRef: &virtv2.ClusterVirtualImageObjectRef{
					Kind:      virtv2.VirtualImageKind,
					Name:      "test",
					Namespace: "test",
				},
			}),
			*cvibuilder.New(
				cvibuilder.WithName("test322"),
				cvibuilder.WithPhase(virtv2.ImageReady),
				cvibuilder.WithCondition(metav1.Condition{
					Status: metav1.ConditionTrue,
					Reason: cvicondition.ReadyType.String(),
				}),
				cvibuilder.WithDatasource(virtv2.ClusterVirtualImageDataSource{
					Type: virtv2.DataSourceTypeObjectRef,
					ObjectRef: &virtv2.ClusterVirtualImageObjectRef{
						Kind:      virtv2.VirtualImageKind,
						Name:      "test",
						Namespace: "ns",
					},
				}),
			),
		},
		VMBDAs: []virtv2.VirtualMachineBlockDeviceAttachment{
			generateVMBDAForInUseTest(
				"test1",
				"ns",
				virtv2.VMBDAObjectRef{
					Kind: virtv2.VirtualImageKind,
					Name: "test",
				},
				"test-vm",
			),
		},
		ExpectedConditionStatus: metav1.ConditionTrue,
		ExpectedConditionReason: vicondition.InUse.String(),
	}),
)

type inUseHandlerTestArgs struct {
	VINamespacedName        types.NamespacedName
	DeletionTimestamp       *metav1.Time
	VMs                     []virtv2.VirtualMachine
	VDs                     []virtv2.VirtualDisk
	VIs                     []virtv2.VirtualImage
	CVIs                    []virtv2.ClusterVirtualImage
	VMBDAs                  []virtv2.VirtualMachineBlockDeviceAttachment
	ExpectedConditionReason string
	ExpectedConditionStatus metav1.ConditionStatus
}

func generateVMForInUseTest(name, namespace string, blockDeviceRefs []virtv2.BlockDeviceStatusRef) virtv2.VirtualMachine {
	return virtv2.VirtualMachine{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		TypeMeta: metav1.TypeMeta{
			Kind: virtv2.VirtualMachineKind,
		},
		Status: virtv2.VirtualMachineStatus{
			BlockDeviceRefs: blockDeviceRefs,
		},
	}
}

func generateVIForInUseTest(name, namespace string, datasource virtv2.VirtualImageDataSource) virtv2.VirtualImage {
	return *vibuilder.New(
		vibuilder.WithName(name),
		vibuilder.WithNamespace(namespace),
		vibuilder.WithDatasource(datasource),
	)
}

func generateCVIForInUseTest(name string, datasource virtv2.ClusterVirtualImageDataSource) virtv2.ClusterVirtualImage {
	return *cvibuilder.New(
		cvibuilder.WithName(name),
		cvibuilder.WithDatasource(datasource),
	)
}

func generateVDForInUseTest(name, namespace string, datasource virtv2.VirtualDiskDataSource) virtv2.VirtualDisk {
	return *vdbuilder.New(
		vdbuilder.WithName(name),
		vdbuilder.WithNamespace(namespace),
		vdbuilder.WithDatasource(&datasource),
	)
}

func generateVMBDAForInUseTest(name, namespace string, bdRef virtv2.VMBDAObjectRef, vmName string) virtv2.VirtualMachineBlockDeviceAttachment {
	return *vmbdabuilder.New(
		vmbdabuilder.WithName(name),
		vmbdabuilder.WithNamespace(namespace),
		vmbdabuilder.WithBlockDeviceRef(bdRef),
		vmbdabuilder.WithVMName(vmName),
	)
}
