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
	"context"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	apiruntime "k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/ptr"
	virtv1 "kubevirt.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/deckhouse/virtualization-controller/pkg/controller/conditions"
	"github.com/deckhouse/virtualization-controller/pkg/controller/indexer"
	virtv2 "github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/vicondition"
)

var _ = DescribeTable("InUseHandler Handle", func(args inUseHandlerTestArgs) {
	scheme := apiruntime.NewScheme()
	for _, f := range []func(*apiruntime.Scheme) error{
		virtv2.AddToScheme,
		virtv1.AddToScheme,
		corev1.AddToScheme,
	} {
		err := f(scheme)
		Expect(err).NotTo(HaveOccurred(), "failed to add scheme: %s", err)
	}

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

	for _, vd := range args.VDs {
		objects = append(objects, &vd)
	}

	for _, vi := range args.VIs {
		objects = append(objects, &vi)
	}

	for _, cvi := range args.CVIs {
		objects = append(objects, &cvi)
	}

	fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(objects...).WithIndex(
		&virtv2.VirtualDisk{},
		indexer.IndexFieldVDByVIDataSourceNotReady,
		indexer.IndexVDByVIDataSourceIndexerFunc,
	).WithIndex(
		&virtv2.VirtualImage{},
		indexer.IndexFieldVIByVIDataSourceNotReady,
		indexer.IndexVIByVIDataSourceIndexerFunc,
	).WithIndex(
		&virtv2.ClusterVirtualImage{},
		indexer.IndexFieldCVIByVIDataSourceNotReady,
		indexer.IndexCVIByVIDataSourceIndexerFunc,
	).Build()
	handler := NewInUseHandler(fakeClient)

	result, err := handler.Handle(context.Background(), vi)
	Expect(err).To(BeNil())
	Expect(result).To(Equal(reconcile.Result{}))
	inUseCondition, ok := conditions.GetCondition(vicondition.InUseType, vi.Status.Conditions)
	if args.ExpectedConditionExists {
		Expect(ok).To(BeTrue())
		Expect(inUseCondition.Status).To(Equal(args.ExpectedConditionStatus))
		Expect(inUseCondition.Reason).To(Equal(args.ExpectedConditionReason))
		Expect(inUseCondition.Message).To(Equal(args.ExpectedConditionMessage))
	} else {
		Expect(ok).To(BeFalse())
	}
},
	Entry("deletionTimestamp not exists", inUseHandlerTestArgs{
		VMs: []virtv2.VirtualMachine{},
		VINamespacedName: types.NamespacedName{
			Name:      "test",
			Namespace: "ns",
		},
		ExpectedConditionExists: false,
	}),
	Entry("deletionTimestamp exists but no one uses VI", inUseHandlerTestArgs{
		VMs: []virtv2.VirtualMachine{},
		VINamespacedName: types.NamespacedName{
			Name:      "test",
			Namespace: "ns",
		},
		DeletionTimestamp:       ptr.To(metav1.Time{Time: time.Now()}),
		ExpectedConditionExists: false,
	}),
	Entry("has VirtualMachine but with no deleted VI", inUseHandlerTestArgs{
		VINamespacedName: types.NamespacedName{
			Name:      "test",
			Namespace: "ns",
		},
		VMs: []virtv2.VirtualMachine{
			generateVMForInUseTest("test-vm", "ns", []virtv2.BlockDeviceStatusRef{
				{
					Kind: virtv2.VirtualImageKind,
					Name: "test123",
				},
			}),
		},
		ExpectedConditionExists: false,
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
		ExpectedConditionExists:  true,
		ExpectedConditionStatus:  metav1.ConditionTrue,
		ExpectedConditionReason:  vicondition.InUse.String(),
		ExpectedConditionMessage: "The VirtualImage is currently attached to the VirtualMachine test-vm.",
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
		ExpectedConditionExists:  true,
		ExpectedConditionStatus:  metav1.ConditionTrue,
		ExpectedConditionReason:  vicondition.InUse.String(),
		ExpectedConditionMessage: "The VirtualImage is currently attached to the VirtualMachines: test-vm, test-vm2.",
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
		},
		ExpectedConditionExists:  true,
		ExpectedConditionStatus:  metav1.ConditionTrue,
		ExpectedConditionReason:  vicondition.InUse.String(),
		ExpectedConditionMessage: "5 VirtualMachines are using the VirtualImage.",
	}),
	Entry("has 5 VM with connected terminating VI, 4 VD, 2 CVI, 1 VI", inUseHandlerTestArgs{
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
			{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test1",
					Namespace: "ns",
				},
				Spec: virtv2.VirtualDiskSpec{
					DataSource: &virtv2.VirtualDiskDataSource{
						Type: virtv2.DataSourceTypeObjectRef,
						ObjectRef: &virtv2.VirtualDiskObjectRef{
							Kind: virtv2.VirtualImageKind,
							Name: "test",
						},
					},
				},
			},
			{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test2",
					Namespace: "ns",
				},
				Spec: virtv2.VirtualDiskSpec{
					DataSource: &virtv2.VirtualDiskDataSource{
						Type: virtv2.DataSourceTypeObjectRef,
						ObjectRef: &virtv2.VirtualDiskObjectRef{
							Kind: virtv2.VirtualImageKind,
							Name: "test",
						},
					},
				},
			},
			{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test3",
					Namespace: "ns",
				},
				Spec: virtv2.VirtualDiskSpec{
					DataSource: &virtv2.VirtualDiskDataSource{
						Type: virtv2.DataSourceTypeObjectRef,
						ObjectRef: &virtv2.VirtualDiskObjectRef{
							Kind: virtv2.VirtualImageKind,
							Name: "test",
						},
					},
				},
			},
			{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test4",
					Namespace: "ns",
				},
				Spec: virtv2.VirtualDiskSpec{
					DataSource: &virtv2.VirtualDiskDataSource{
						Type: virtv2.DataSourceTypeObjectRef,
						ObjectRef: &virtv2.VirtualDiskObjectRef{
							Kind: virtv2.VirtualImageKind,
							Name: "test",
						},
					},
				},
			},
		},
		VIs: []virtv2.VirtualImage{
			{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test1",
					Namespace: "ns",
				},
				Spec: virtv2.VirtualImageSpec{
					DataSource: virtv2.VirtualImageDataSource{
						Type: virtv2.DataSourceTypeObjectRef,
						ObjectRef: &virtv2.VirtualImageObjectRef{
							Kind: virtv2.VirtualImageKind,
							Name: "test",
						},
					},
				},
			},
		},
		CVIs: []virtv2.ClusterVirtualImage{
			{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test1",
				},
				Spec: virtv2.ClusterVirtualImageSpec{
					DataSource: virtv2.ClusterVirtualImageDataSource{
						Type: virtv2.DataSourceTypeObjectRef,
						ObjectRef: &virtv2.ClusterVirtualImageObjectRef{
							Kind:      virtv2.VirtualImageKind,
							Name:      "test",
							Namespace: "ns",
						},
					},
				},
			},
			{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test2",
				},
				Spec: virtv2.ClusterVirtualImageSpec{
					DataSource: virtv2.ClusterVirtualImageDataSource{
						Type: virtv2.DataSourceTypeObjectRef,
						ObjectRef: &virtv2.ClusterVirtualImageObjectRef{
							Kind:      virtv2.VirtualImageKind,
							Name:      "test",
							Namespace: "ns",
						},
					},
				},
			},
			{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test3",
				},
				Spec: virtv2.ClusterVirtualImageSpec{
					DataSource: virtv2.ClusterVirtualImageDataSource{
						Type: virtv2.DataSourceTypeObjectRef,
						ObjectRef: &virtv2.ClusterVirtualImageObjectRef{
							Kind:      virtv2.VirtualImageKind,
							Name:      "test",
							Namespace: "test",
						},
					},
				},
			},
		},
		ExpectedConditionExists:  true,
		ExpectedConditionStatus:  metav1.ConditionTrue,
		ExpectedConditionReason:  vicondition.InUse.String(),
		ExpectedConditionMessage: "5 VirtualMachines are using the VirtualImage, VirtualImage is used to create 4 VirtualDisks, VirtualImage is currently being used to create the VirtualImage test1, VirtualImage is currently being used to create the ClusterVirtualImages: test1, test2.",
	}),
)

type inUseHandlerTestArgs struct {
	VINamespacedName         types.NamespacedName
	DeletionTimestamp        *metav1.Time
	VMs                      []virtv2.VirtualMachine
	VDs                      []virtv2.VirtualDisk
	VIs                      []virtv2.VirtualImage
	CVIs                     []virtv2.ClusterVirtualImage
	ExpectedConditionExists  bool
	ExpectedConditionReason  string
	ExpectedConditionMessage string
	ExpectedConditionStatus  metav1.ConditionStatus
}

func generateVMForInUseTest(name, namespace string, blockDeviceRefs []virtv2.BlockDeviceStatusRef) virtv2.VirtualMachine {
	return virtv2.VirtualMachine{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Status: virtv2.VirtualMachineStatus{
			BlockDeviceRefs: blockDeviceRefs,
		},
	}
}
