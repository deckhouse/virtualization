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
	"k8s.io/utils/ptr"
	virtv1 "kubevirt.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/deckhouse/virtualization-controller/pkg/controller/conditions"
	"github.com/deckhouse/virtualization-controller/pkg/controller/indexer"
	virtv2 "github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/cvicondition"
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

	cvi := &virtv2.ClusterVirtualImage{
		ObjectMeta: metav1.ObjectMeta{
			Name:              args.CVIName,
			DeletionTimestamp: args.DeletionTimestamp,
		},
		Status: virtv2.ClusterVirtualImageStatus{
			Conditions: []metav1.Condition{
				{
					Type:   cvicondition.ReadyType.String(),
					Reason: cvicondition.Ready.String(),
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
		indexer.IndexFieldVDByCVIDataSourceNotReady,
		indexer.IndexVDByVIDataSourceIndexerFunc,
	).WithIndex(
		&virtv2.VirtualImage{},
		indexer.IndexFieldVIByCVIDataSourceNotReady,
		indexer.IndexVIByVIDataSourceIndexerFunc,
	).WithIndex(
		&virtv2.ClusterVirtualImage{},
		indexer.IndexFieldCVIByCVIDataSourceNotReady,
		indexer.IndexCVIByVIDataSourceIndexerFunc,
	).Build()
	handler := NewInUseHandler(fakeClient)

	result, err := handler.Handle(context.Background(), cvi)
	Expect(err).To(BeNil())
	Expect(result).To(Equal(reconcile.Result{}))
	inUseCondition, ok := conditions.GetCondition(cvicondition.InUseType, cvi.Status.Conditions)
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
		VMs: []virtv2.VirtualMachine{
			generateVMForInUseTest("name", "ns", []virtv2.BlockDeviceStatusRef{}),
		},
		CVIName:                 "test",
		ExpectedConditionExists: false,
	}),
	Entry("has VirtualMachine but with no deleted CVI", inUseHandlerTestArgs{
		CVIName:           "test",
		DeletionTimestamp: ptr.To(metav1.Time{Time: time.Now()}),
		VMs: []virtv2.VirtualMachine{
			generateVMForInUseTest("name", "ns2", []virtv2.BlockDeviceStatusRef{}),
		},
		ExpectedConditionExists: false,
	}),
	Entry("has 1 VirtualMachine with connected terminating CVI", inUseHandlerTestArgs{
		CVIName:           "test",
		DeletionTimestamp: ptr.To(metav1.Time{Time: time.Now()}),
		VMs: []virtv2.VirtualMachine{
			generateVMForInUseTest("name", "ns", []virtv2.BlockDeviceStatusRef{
				{
					Kind: virtv2.ClusterVirtualImageKind,
					Name: "test",
				},
			}),
		},
		ExpectedConditionExists:  true,
		ExpectedConditionStatus:  metav1.ConditionTrue,
		ExpectedConditionReason:  cvicondition.InUse.String(),
		ExpectedConditionMessage: "The ClusterVirtualImage is currently attached to the VirtualMachine ns/name, ClusterVirtualImage is currently using in Namespace ns.",
	}),
	Entry("has 5 VirtualMachines with connected terminating CVI", inUseHandlerTestArgs{
		CVIName:           "test",
		DeletionTimestamp: ptr.To(metav1.Time{Time: time.Now()}),
		VMs: []virtv2.VirtualMachine{
			generateVMForInUseTest("name", "ns", []virtv2.BlockDeviceStatusRef{
				{
					Kind: virtv2.ClusterVirtualImageKind,
					Name: "test",
				},
			}),
			generateVMForInUseTest("name2", "ns", []virtv2.BlockDeviceStatusRef{
				{
					Kind: virtv2.ClusterVirtualImageKind,
					Name: "test",
				},
			}),
			generateVMForInUseTest("name3", "ns", []virtv2.BlockDeviceStatusRef{
				{
					Kind: virtv2.ClusterVirtualImageKind,
					Name: "test",
				},
			}),
			generateVMForInUseTest("name4", "ns", []virtv2.BlockDeviceStatusRef{
				{
					Kind: virtv2.ClusterVirtualImageKind,
					Name: "test",
				},
			}),
			generateVMForInUseTest("name5", "ns", []virtv2.BlockDeviceStatusRef{
				{
					Kind: virtv2.ClusterVirtualImageKind,
					Name: "test",
				},
			}),
		},
		ExpectedConditionExists:  true,
		ExpectedConditionStatus:  metav1.ConditionTrue,
		ExpectedConditionReason:  cvicondition.InUse.String(),
		ExpectedConditionMessage: "5 VirtualMachines are using the ClusterVirtualImage, ClusterVirtualImage is currently using in Namespace ns.",
	}),
	Entry("has 5 VirtualMachines with connected terminating CVI, 4 VD, 2 VI, 1 CVI", inUseHandlerTestArgs{
		CVIName:           "test",
		DeletionTimestamp: ptr.To(metav1.Time{Time: time.Now()}),
		VMs: []virtv2.VirtualMachine{
			generateVMForInUseTest("name", "ns", []virtv2.BlockDeviceStatusRef{
				{
					Kind: virtv2.ClusterVirtualImageKind,
					Name: "test",
				},
			}),
			generateVMForInUseTest("name2", "ns", []virtv2.BlockDeviceStatusRef{
				{
					Kind: virtv2.ClusterVirtualImageKind,
					Name: "test",
				},
			}),
			generateVMForInUseTest("name3", "ns", []virtv2.BlockDeviceStatusRef{
				{
					Kind: virtv2.ClusterVirtualImageKind,
					Name: "test",
				},
			}),
			generateVMForInUseTest("name4", "ns", []virtv2.BlockDeviceStatusRef{
				{
					Kind: virtv2.ClusterVirtualImageKind,
					Name: "test",
				},
			}),
			generateVMForInUseTest("name5", "ns", []virtv2.BlockDeviceStatusRef{
				{
					Kind: virtv2.ClusterVirtualImageKind,
					Name: "test",
				},
			}),
		},
		VDs: []virtv2.VirtualDisk{
			{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test",
					Namespace: "ns1",
				},
				Spec: virtv2.VirtualDiskSpec{
					DataSource: &virtv2.VirtualDiskDataSource{
						Type: virtv2.DataSourceTypeObjectRef,
						ObjectRef: &virtv2.VirtualDiskObjectRef{
							Kind: virtv2.ClusterVirtualImageKind,
							Name: "test",
						},
					},
				},
			},
			{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test2",
					Namespace: "ns2",
				},
				Spec: virtv2.VirtualDiskSpec{
					DataSource: &virtv2.VirtualDiskDataSource{
						Type: virtv2.DataSourceTypeObjectRef,
						ObjectRef: &virtv2.VirtualDiskObjectRef{
							Kind: virtv2.ClusterVirtualImageKind,
							Name: "test",
						},
					},
				},
			},
			{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test3",
					Namespace: "ns3",
				},
				Spec: virtv2.VirtualDiskSpec{
					DataSource: &virtv2.VirtualDiskDataSource{
						Type: virtv2.DataSourceTypeObjectRef,
						ObjectRef: &virtv2.VirtualDiskObjectRef{
							Kind: virtv2.ClusterVirtualImageKind,
							Name: "test",
						},
					},
				},
			},
			{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test4",
					Namespace: "ns4",
				},
				Spec: virtv2.VirtualDiskSpec{
					DataSource: &virtv2.VirtualDiskDataSource{
						Type: virtv2.DataSourceTypeObjectRef,
						ObjectRef: &virtv2.VirtualDiskObjectRef{
							Kind: virtv2.ClusterVirtualImageKind,
							Name: "test",
						},
					},
				},
			},
		},
		VIs: []virtv2.VirtualImage{
			{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test",
					Namespace: "ns",
				},
				Spec: virtv2.VirtualImageSpec{
					DataSource: virtv2.VirtualImageDataSource{
						Type: virtv2.DataSourceTypeObjectRef,
						ObjectRef: &virtv2.VirtualImageObjectRef{
							Kind: virtv2.ClusterVirtualImageKind,
							Name: "test",
						},
					},
				},
			},
			{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test2",
					Namespace: "ns5",
				},
				Spec: virtv2.VirtualImageSpec{
					DataSource: virtv2.VirtualImageDataSource{
						Type: virtv2.DataSourceTypeObjectRef,
						ObjectRef: &virtv2.VirtualImageObjectRef{
							Kind: virtv2.ClusterVirtualImageKind,
							Name: "test",
						},
					},
				},
			},
		},
		CVIs: []virtv2.ClusterVirtualImage{
			{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test2",
				},
				Spec: virtv2.ClusterVirtualImageSpec{
					DataSource: virtv2.ClusterVirtualImageDataSource{
						Type: virtv2.DataSourceTypeObjectRef,
						ObjectRef: &virtv2.ClusterVirtualImageObjectRef{
							Kind: virtv2.ClusterVirtualImageKind,
							Name: "test",
						},
					},
				},
			},
		},
		ExpectedConditionExists:  true,
		ExpectedConditionStatus:  metav1.ConditionTrue,
		ExpectedConditionReason:  cvicondition.InUse.String(),
		ExpectedConditionMessage: "5 VirtualMachines are using the ClusterVirtualImage, ClusterVirtualImage is currently using in Namespace ns.",
	}),
)

type inUseHandlerTestArgs struct {
	CVIName                  string
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
