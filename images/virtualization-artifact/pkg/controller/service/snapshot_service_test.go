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

package service

import (
	"context"
	"fmt"
	"reflect"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	virtv1 "kubevirt.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/deckhouse/virtualization-controller/pkg/common/annotations"
	v1alpha2clientsetcore "github.com/deckhouse/virtualization/api/client/generated/clientset/versioned/typed/core/v1alpha2"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
	subv1alpha2 "github.com/deckhouse/virtualization/api/subresources/v1alpha2"
)

var _ = Describe("SnapshotService", func() {
	var (
		ctx               context.Context
		snapshotter       *SnapshotService
		clientMock        *ClientMock
		virtClientMock    *VirtClientMock
		protectionService *ProtectionService
	)

	BeforeEach(func() {
		ctx = testContext()
		protectionService = NewProtectionService(clientMock, "finalizer")
		clientMock = &ClientMock{}
		virtClientMock = &VirtClientMock{}
		snapshotter = NewSnapshotService(virtClientMock, clientMock, protectionService)
	})

	Context("Filesystem", func() {
		DescribeTable("IsFrozen", func(kvvmi *virtv1.VirtualMachineInstance, isFrozen bool, specialErr error) {
			isFSFrozen, err := snapshotter.IsFrozen(kvvmi)
			if specialErr != nil {
				Expect(err).To(MatchError(err))
			} else {
				Expect(err).NotTo(HaveOccurred())
			}
			Expect(isFSFrozen).To(Equal(isFrozen))
		},
			Entry(
				"ShouldBeFrozen",
				&virtv1.VirtualMachineInstance{
					Status: virtv1.VirtualMachineInstanceStatus{
						FSFreezeStatus: FSFrozen,
					},
				},
				true, // isFrozen
				nil,  // specialErr
			),
			Entry(
				"ShouldBeThawed",
				&virtv1.VirtualMachineInstance{}, // kvvmi
				false,                            // isFrozen
				nil,                              // specialErr
			),
			Entry(
				"ShouldReturnErrUntrustedFilesystemFrozenCondition",
				&virtv1.VirtualMachineInstance{
					ObjectMeta: metav1.ObjectMeta{
						Annotations: map[string]string{
							annotations.AnnVMFilesystemRequest: RequestFSFreeze,
						},
					},
				},
				false,                                 // isFrozen
				ErrUntrustedFilesystemFrozenCondition, // specialErr
			),
		)

		DescribeTable("CanFreeze", func(kvvmi *virtv1.VirtualMachineInstance, canFreeze bool) {
			result, err := snapshotter.CanFreeze(ctx, kvvmi)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(Equal(canFreeze))
		},
			Entry(
				"VirtualMachineInstanceDoesNotExist",
				nil,   // kvvmi
				false, // canFreeze
			), Entry(
				"VirtualMachineInstanceIsNotRunning",
				&virtv1.VirtualMachineInstance{
					Status: virtv1.VirtualMachineInstanceStatus{
						Phase: virtv1.VmPhaseUnset,
					},
				},
				false, // canFreeze
			), Entry(
				"GuestAgentIsNotReady",
				&virtv1.VirtualMachineInstance{
					Status: virtv1.VirtualMachineInstanceStatus{
						Phase: virtv1.Running,
						Conditions: []virtv1.VirtualMachineInstanceCondition{
							{
								Type:   virtv1.VirtualMachineInstanceAgentConnected,
								Status: corev1.ConditionFalse,
							},
						},
					},
				},
				false, // canFreeze
			), Entry(
				"GuestAgentIsReadyAndFilesystemIsNotFrozen",
				&virtv1.VirtualMachineInstance{
					Status: virtv1.VirtualMachineInstanceStatus{
						Phase: virtv1.Running,
						Conditions: []virtv1.VirtualMachineInstanceCondition{
							{
								Type:   virtv1.VirtualMachineInstanceAgentConnected,
								Status: corev1.ConditionTrue,
							},
						},
					},
				},
				true, // canFreeze
			), Entry(
				"GuestAgentIsReadyAndFilesystemIsFrozen",
				&virtv1.VirtualMachineInstance{
					Status: virtv1.VirtualMachineInstanceStatus{
						Phase: virtv1.Running,
						Conditions: []virtv1.VirtualMachineInstanceCondition{
							{
								Type:   virtv1.VirtualMachineInstanceAgentConnected,
								Status: corev1.ConditionTrue,
							},
						},
						FSFreezeStatus: FSFrozen,
					},
				},
				false, // canFreeze
			),
		)

		DescribeTable("Freeze", func(kvvmi *virtv1.VirtualMachineInstance, specialErr error) {
			clientMock.UpdateFunc = func(_ context.Context, _ client.Object, _ ...client.UpdateOption) error {
				return nil
			}
			virtClientMock.VirtualMachinesFunc = func(namespace string) v1alpha2clientsetcore.VirtualMachineInterface {
				return &VirtualMachineInterfaceMock{}
			}
			err := snapshotter.Freeze(ctx, kvvmi)
			if specialErr != nil {
				Expect(err).To(MatchError(specialErr))
			} else {
				Expect(err).To(Not(HaveOccurred()))
			}
			Expect(kvvmi.Annotations).To(HaveKeyWithValue(annotations.AnnVMFilesystemRequest, RequestFSFreeze))
		},
			Entry(
				"ShoudBeSuccessful",
				&virtv1.VirtualMachineInstance{}, // kvvmi
				nil,                              // specialErr
			),
			Entry(
				"ShouldReturnErrUnexpectedFilesystemRequest",
				&virtv1.VirtualMachineInstance{
					ObjectMeta: metav1.ObjectMeta{
						Annotations: map[string]string{
							annotations.AnnVMFilesystemRequest: RequestFSFreeze,
						},
					},
				},
				ErrUnexpectedFilesystemRequest, // specialErr
			),
		)

		DescribeTable("CanUnfreezeWithVirtualDiskSnapshot", func(
			vdSnapshotName string,
			vm *v1alpha2.VirtualMachine,
			kvvmi *virtv1.VirtualMachineInstance,
			canUnfreeze, otherInProgressVirtualDiskSnapshots, otherInProgressVirtualMachineSnapshots bool,
		) {
			clientMock.ListFunc = func(ctx context.Context, list client.ObjectList, opts ...client.ListOption) error {
				listType := reflect.TypeOf(list)

				switch listType {
				case reflect.TypeOf(&v1alpha2.VirtualDiskSnapshotList{}):
					if otherInProgressVirtualDiskSnapshots {
						*list.(*v1alpha2.VirtualDiskSnapshotList) = v1alpha2.VirtualDiskSnapshotList{
							Items: []v1alpha2.VirtualDiskSnapshot{
								{
									ObjectMeta: metav1.ObjectMeta{Name: "vdsnapshot-root"},
									Spec: v1alpha2.VirtualDiskSnapshotSpec{
										VirtualDiskName: "vd-root",
									},
									Status: v1alpha2.VirtualDiskSnapshotStatus{
										Phase: v1alpha2.VirtualDiskSnapshotPhaseInProgress,
									},
								},
								{
									ObjectMeta: metav1.ObjectMeta{Name: "vdsnapshot-attach"},
									Spec: v1alpha2.VirtualDiskSnapshotSpec{
										VirtualDiskName: "vd-attach",
									},
									Status: v1alpha2.VirtualDiskSnapshotStatus{
										Phase: v1alpha2.VirtualDiskSnapshotPhaseInProgress,
									},
								},
							},
						}
					}
				case reflect.TypeOf(&v1alpha2.VirtualMachineSnapshotList{}):
					if otherInProgressVirtualMachineSnapshots {
						*list.(*v1alpha2.VirtualMachineSnapshotList) = v1alpha2.VirtualMachineSnapshotList{
							Items: []v1alpha2.VirtualMachineSnapshot{
								{
									Spec: v1alpha2.VirtualMachineSnapshotSpec{
										VirtualMachineName: "vm",
									},
									Status: v1alpha2.VirtualMachineSnapshotStatus{
										Phase: v1alpha2.VirtualMachineSnapshotPhaseInProgress,
									},
								},
							},
						}
					}
				default:
					return fmt.Errorf("unsupported list type: %s", listType)
				}

				return nil
			}
			result, err := snapshotter.CanUnfreezeWithVirtualDiskSnapshot(ctx, vdSnapshotName, vm, kvvmi)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(Equal(canUnfreeze))
		},
			Entry(
				"VirtualMachineDoesNotExist",
				"vdsnapshot-root", // vdSnapshotName
				nil,               // vm
				nil,               // kvvmi
				false,             // canUnfreeze
				false,             // otherInProgressVirtualDiskSnapshots
				false,             // otherInProgressVirtualMachineSnapshots
			),
			Entry(
				"FilesystemIsNotFrozen",
				"vdsnapshot-root",                // vdSnapshotName
				&v1alpha2.VirtualMachine{},       // vm
				&virtv1.VirtualMachineInstance{}, // kvvmi
				false,                            // canUnfreeze
				false,                            // otherInProgressVirtualDiskSnapshots
				false,                            // otherInProgressVirtualMachineSnapshots
			),
			Entry(
				"FilesystemIsFrozenAndThereAreInProgressVirtualDiskSnapshots",
				"vdsnapshot-root", // vdSnapshotName
				&v1alpha2.VirtualMachine{
					ObjectMeta: metav1.ObjectMeta{
						Name: "vm",
					},
					Status: v1alpha2.VirtualMachineStatus{
						BlockDeviceRefs: []v1alpha2.BlockDeviceStatusRef{
							{
								Kind: v1alpha2.DiskDevice,
								Name: "vd-root",
							},
							{
								Kind: v1alpha2.DiskDevice,
								Name: "vd-attach",
							},
						},
					},
				},
				&virtv1.VirtualMachineInstance{
					Status: virtv1.VirtualMachineInstanceStatus{
						FSFreezeStatus: FSFrozen,
					},
				},
				false, // canUnfreeze
				true,  // otherInProgressVirtualDiskSnapshots
				false, // otherInProgressVirtualMachineSnapshots
			),
			Entry(
				"FilesystemIsFrozenAndThereAreInProgressVirtualMachineSnapshots",
				"vdsnapshot-root", // vdSnapshotName
				&v1alpha2.VirtualMachine{
					ObjectMeta: metav1.ObjectMeta{
						Name: "vm",
					},
					Status: v1alpha2.VirtualMachineStatus{
						BlockDeviceRefs: []v1alpha2.BlockDeviceStatusRef{
							{
								Kind: v1alpha2.DiskDevice,
								Name: "vd-root",
							},
							{
								Kind: v1alpha2.DiskDevice,
								Name: "vd-attach",
							},
						},
					},
				},
				&virtv1.VirtualMachineInstance{
					Status: virtv1.VirtualMachineInstanceStatus{
						FSFreezeStatus: FSFrozen,
					},
				},
				false, // canUnfreeze
				false, // otherInProgressVirtualDiskSnapshots
				true,  // otherInProgressVirtualMachineSnapshots
			),
			Entry(
				"FilesystemIsFrozenAndThereAreNotInProgressSnapshots",
				"vdsnapshot-root", // vdSnapshotName
				&v1alpha2.VirtualMachine{
					ObjectMeta: metav1.ObjectMeta{
						Name: "vm",
					},
					Status: v1alpha2.VirtualMachineStatus{
						BlockDeviceRefs: []v1alpha2.BlockDeviceStatusRef{
							{
								Kind: v1alpha2.DiskDevice,
								Name: "vd-root",
							},
							{
								Kind: v1alpha2.DiskDevice,
								Name: "vd-attach",
							},
						},
					},
				},
				&virtv1.VirtualMachineInstance{
					Status: virtv1.VirtualMachineInstanceStatus{
						FSFreezeStatus: FSFrozen,
					},
				},
				true,  // canUnfreeze
				false, // otherInProgressVirtualDiskSnapshots
				false, // otherInProgressVirtualMachineSnapshots
			),
		)

		DescribeTable("CanUnfreezeWithVirtualMachineSnapshot", func(
			vmSnapshotName string,
			vm *v1alpha2.VirtualMachine,
			kvvmi *virtv1.VirtualMachineInstance,
			canUnfreeze, otherInProgressVirtualDiskSnapshots, otherInProgressVirtualMachineSnapshots bool,
		) {
			clientMock.ListFunc = func(ctx context.Context, list client.ObjectList, opts ...client.ListOption) error {
				listType := reflect.TypeOf(list)

				switch listType {
				case reflect.TypeOf(&v1alpha2.VirtualDiskSnapshotList{}):
					if otherInProgressVirtualDiskSnapshots {
						*list.(*v1alpha2.VirtualDiskSnapshotList) = v1alpha2.VirtualDiskSnapshotList{
							Items: []v1alpha2.VirtualDiskSnapshot{
								{
									ObjectMeta: metav1.ObjectMeta{Name: "vdsnapshot-root"},
									Spec: v1alpha2.VirtualDiskSnapshotSpec{
										VirtualDiskName: "vd-root",
									},
									Status: v1alpha2.VirtualDiskSnapshotStatus{
										Phase: v1alpha2.VirtualDiskSnapshotPhaseInProgress,
									},
								},
								{
									ObjectMeta: metav1.ObjectMeta{Name: "vdsnapshot-attach"},
									Spec: v1alpha2.VirtualDiskSnapshotSpec{
										VirtualDiskName: "vd-attach",
									},
									Status: v1alpha2.VirtualDiskSnapshotStatus{
										Phase: v1alpha2.VirtualDiskSnapshotPhaseInProgress,
									},
								},
							},
						}
					}
				case reflect.TypeOf(&v1alpha2.VirtualMachineSnapshotList{}):
					if otherInProgressVirtualMachineSnapshots {
						*list.(*v1alpha2.VirtualMachineSnapshotList) = v1alpha2.VirtualMachineSnapshotList{
							Items: []v1alpha2.VirtualMachineSnapshot{
								{
									ObjectMeta: metav1.ObjectMeta{Name: vmSnapshotName},
									Spec: v1alpha2.VirtualMachineSnapshotSpec{
										VirtualMachineName: "vm",
									},
									Status: v1alpha2.VirtualMachineSnapshotStatus{
										Phase: v1alpha2.VirtualMachineSnapshotPhaseInProgress,
									},
								},
								{
									ObjectMeta: metav1.ObjectMeta{Name: "other-vmsnapshot"},
									Spec: v1alpha2.VirtualMachineSnapshotSpec{
										VirtualMachineName: "vm",
									},
									Status: v1alpha2.VirtualMachineSnapshotStatus{
										Phase: v1alpha2.VirtualMachineSnapshotPhaseInProgress,
									},
								},
							},
						}
					}
				default:
					return fmt.Errorf("unsupported list type: %s", listType)
				}

				return nil
			}
			result, err := snapshotter.CanUnfreezeWithVirtualMachineSnapshot(ctx, vmSnapshotName, vm, kvvmi)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(Equal(canUnfreeze))
		},
			Entry(
				"VirtualMachineDoesNotExist",
				"vmsnapshot", // vmSnapshotName
				nil,          // vm
				nil,          // kvvmi
				false,        // canUnfreeze
				false,        // otherInProgressVirtualDiskSnapshots
				false,        // otherInProgressVirtualMachineSnapshots
			),
			Entry(
				"FilesystemIsNotFrozen",
				"vmsnapshot",                     // vmSnapshotName
				&v1alpha2.VirtualMachine{},       // vm
				&virtv1.VirtualMachineInstance{}, // kvvmi
				false,                            // canUnfreeze
				false,                            // otherInProgressVirtualDiskSnapshots
				false,                            // otherInProgressVirtualMachineSnapshots
			),
			Entry(
				"FilesystemIsFrozenAndThereAreInProgressVirtualDiskSnapshots",
				"vmsnapshot", // vmSnapshotName
				&v1alpha2.VirtualMachine{
					ObjectMeta: metav1.ObjectMeta{
						Name: "vm",
					},
					Status: v1alpha2.VirtualMachineStatus{
						BlockDeviceRefs: []v1alpha2.BlockDeviceStatusRef{
							{
								Kind: v1alpha2.DiskDevice,
								Name: "vd-root",
							},
							{
								Kind: v1alpha2.DiskDevice,
								Name: "vd-attach",
							},
						},
					},
				},
				&virtv1.VirtualMachineInstance{
					Status: virtv1.VirtualMachineInstanceStatus{
						FSFreezeStatus: FSFrozen,
					},
				},
				false, // canUnfreeze
				true,  // otherInProgressVirtualDiskSnapshots
				false, // otherInProgressVirtualMachineSnapshots
			),
			Entry(
				"FilesystemIsFrozenAndThereAreInProgressVirtualMachineSnapshots",
				"vmsnapshot", // vmSnapshotName
				&v1alpha2.VirtualMachine{
					ObjectMeta: metav1.ObjectMeta{
						Name: "vm",
					},
					Status: v1alpha2.VirtualMachineStatus{
						BlockDeviceRefs: []v1alpha2.BlockDeviceStatusRef{
							{
								Kind: v1alpha2.DiskDevice,
								Name: "vd-root",
							},
							{
								Kind: v1alpha2.DiskDevice,
								Name: "vd-attach",
							},
						},
					},
				},
				&virtv1.VirtualMachineInstance{
					Status: virtv1.VirtualMachineInstanceStatus{
						FSFreezeStatus: FSFrozen,
					},
				},
				false, // canUnfreeze
				false, // otherInProgressVirtualDiskSnapshots
				true,  // otherInProgressVirtualMachineSnapshots
			),
			Entry(
				"FilesystemIsFrozenAndThereAreNotInProgressSnapshots",
				"vmsnapshot", // vmSnapshotName
				&v1alpha2.VirtualMachine{
					ObjectMeta: metav1.ObjectMeta{
						Name: "vm",
					},
					Status: v1alpha2.VirtualMachineStatus{
						BlockDeviceRefs: []v1alpha2.BlockDeviceStatusRef{
							{
								Kind: v1alpha2.DiskDevice,
								Name: "vd-root",
							},
							{
								Kind: v1alpha2.DiskDevice,
								Name: "vd-attach",
							},
						},
					},
				},
				&virtv1.VirtualMachineInstance{
					Status: virtv1.VirtualMachineInstanceStatus{
						FSFreezeStatus: FSFrozen,
					},
				},
				true,  // canUnfreeze
				false, // otherInProgressVirtualDiskSnapshots
				false, // otherInProgressVirtualMachineSnapshots
			),
		)

		DescribeTable("Unfreeze", func(kvvmi *virtv1.VirtualMachineInstance, specialErr error) {
			clientMock.UpdateFunc = func(_ context.Context, _ client.Object, _ ...client.UpdateOption) error {
				return nil
			}
			virtClientMock.VirtualMachinesFunc = func(namespace string) v1alpha2clientsetcore.VirtualMachineInterface {
				return &VirtualMachineInterfaceMock{}
			}
			err := snapshotter.Unfreeze(ctx, kvvmi)
			if specialErr != nil {
				Expect(err).To(MatchError(specialErr))
			} else {
				Expect(err).To(Not(HaveOccurred()))
			}
			Expect(kvvmi.Annotations).To(HaveKeyWithValue(annotations.AnnVMFilesystemRequest, RequestFSUnfreeze))
		},
			Entry(
				"ShoudBeSuccessful",
				&virtv1.VirtualMachineInstance{}, // kvvmi
				nil,                              // specialErr
			),
			Entry(
				"ShouldReturnErrUnexpectedFilesystemRequest",
				&virtv1.VirtualMachineInstance{
					ObjectMeta: metav1.ObjectMeta{
						Annotations: map[string]string{
							annotations.AnnVMFilesystemRequest: RequestFSUnfreeze,
						},
					},
				},
				ErrUnexpectedFilesystemRequest, // specialErr
			),
		)

		DescribeTable("SyncFSFreezeRequest", func(kvvmi *virtv1.VirtualMachineInstance, annotationExistsAfterSync bool, specialErr error) {
			clientMock.UpdateFunc = func(_ context.Context, _ client.Object, _ ...client.UpdateOption) error {
				return nil
			}
			err := snapshotter.SyncFSFreezeRequest(ctx, kvvmi)
			if kvvmi != nil {
				_, ok := kvvmi.Annotations[annotations.AnnVMFilesystemRequest]
				Expect(ok).To(Equal(annotationExistsAfterSync))
			}
			if specialErr != nil {
				Expect(err).To(MatchError(specialErr))
			} else {
				Expect(err).To(Not(HaveOccurred()))
			}
		},
			Entry(
				"VirtualMachineInstanceDoesNotExist",
				nil,   // kvvmi
				false, // annotationExistsAfterSync
				nil,   // specialErr
			),
			Entry(
				"VirtualMachineFilesystemRequestAnnotationDoesNotExist",
				&virtv1.VirtualMachineInstance{}, // kvvmi
				false,                            // annotationExistsAfterSync
				nil,                              // specialErr
			),
			Entry(
				"VirtualMachineFilesystemRequestAnnotationIsFreezeAndFilesystemStatusIsFrozen",
				&virtv1.VirtualMachineInstance{
					ObjectMeta: metav1.ObjectMeta{
						Annotations: map[string]string{
							annotations.AnnVMFilesystemRequest: RequestFSFreeze,
						},
					},
					Status: virtv1.VirtualMachineInstanceStatus{
						FSFreezeStatus: FSFrozen,
					},
				},
				false, // annotationExistsAfterSync
				nil,   // specialErr
			),
			Entry(
				"VirtualMachineFilesystemRequestAnnotationIsUnfreezeAndFilesystemStatusIsThawed",
				&virtv1.VirtualMachineInstance{
					ObjectMeta: metav1.ObjectMeta{
						Annotations: map[string]string{
							annotations.AnnVMFilesystemRequest: RequestFSUnfreeze,
						},
					},
				},
				false, // annotationExistsAfterSync
				nil,   // specialErr
			),
			Entry(
				"VirtualMachineFilesystemRequestAnnotationAndFilesystemStatusDoNotMatch",
				&virtv1.VirtualMachineInstance{
					ObjectMeta: metav1.ObjectMeta{
						Annotations: map[string]string{
							annotations.AnnVMFilesystemRequest: RequestFSUnfreeze,
						},
					},
					Status: virtv1.VirtualMachineInstanceStatus{
						FSFreezeStatus: FSFrozen,
					},
				},
				true,                                  // annotationExistsAfterSync
				ErrUntrustedFilesystemFrozenCondition, // specialErr
			),
		)
	})
})

type VirtualMachineInterfaceMock struct {
	v1alpha2clientsetcore.VirtualMachineInterface
}

func (m *VirtualMachineInterfaceMock) Freeze(ctx context.Context, name string, opts subv1alpha2.VirtualMachineFreeze) error {
	return nil
}

func (m *VirtualMachineInterfaceMock) Unfreeze(ctx context.Context, name string) error {
	return nil
}
