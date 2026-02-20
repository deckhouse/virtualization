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

package service

import (
	"context"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	service "github.com/deckhouse/virtualization-controller/pkg/controller/service"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
)

var _ = Describe("AttachmentService method IsConflictedAttachment", func() {
	var clientMock *service.ClientMock
	var vmbdaAlpha *v1alpha2.VirtualMachineBlockDeviceAttachment
	var vmbdaBeta *v1alpha2.VirtualMachineBlockDeviceAttachment

	spec := v1alpha2.VirtualMachineBlockDeviceAttachmentSpec{
		VirtualMachineName: "vm",
		BlockDeviceRef: v1alpha2.VMBDAObjectRef{
			Kind: v1alpha2.VMBDAObjectRefKindVirtualDisk,
			Name: "vd",
		},
	}

	BeforeEach(func() {
		vmbdaAlpha = &v1alpha2.VirtualMachineBlockDeviceAttachment{
			ObjectMeta: metav1.ObjectMeta{
				Name: "vmbda-a",
				CreationTimestamp: metav1.Time{
					Time: time.Now(),
				},
			},
			Spec: spec,
		}

		vmbdaBeta = &v1alpha2.VirtualMachineBlockDeviceAttachment{
			ObjectMeta: metav1.ObjectMeta{
				Name:              "vmbda-b",
				CreationTimestamp: vmbdaAlpha.CreationTimestamp,
			},
			Spec: spec,
		}

		clientMock = &service.ClientMock{}
	})

	// T1: -->VMBDA A Should be Conflicted
	// T1:    VMBDA B Phase: "Attached"
	It("Should be Conflicted: there is another vmbda that is not Failed", func() {
		vmbdaBeta.Status.Phase = v1alpha2.BlockDeviceAttachmentPhaseAttached
		clientMock.ListFunc = func(ctx context.Context, list client.ObjectList, opts ...client.ListOption) error {
			list.(*v1alpha2.VirtualMachineBlockDeviceAttachmentList).Items = []v1alpha2.VirtualMachineBlockDeviceAttachment{
				*vmbdaAlpha,
				*vmbdaBeta,
			}
			return nil
		}

		s := NewAttachmentService(clientMock, nil, "")
		isConflicted, conflictWithName, err := s.IsConflictedAttachment(context.Background(), vmbdaAlpha)
		Expect(err).To(BeNil())
		Expect(isConflicted).To(BeTrue())
		Expect(conflictWithName).To(Equal(vmbdaBeta.Name))
	})

	// T1: -->VMBDA A Should be Non-Conflicted
	// T1:    VMBDA B Phase: "Failed"
	It("Should be Non-Conflicted: there is another vmbda that is Failed", func() {
		vmbdaBeta.Status.Phase = v1alpha2.BlockDeviceAttachmentPhaseFailed
		clientMock.ListFunc = func(ctx context.Context, list client.ObjectList, opts ...client.ListOption) error {
			list.(*v1alpha2.VirtualMachineBlockDeviceAttachmentList).Items = []v1alpha2.VirtualMachineBlockDeviceAttachment{
				*vmbdaAlpha,
				*vmbdaBeta,
			}
			return nil
		}

		s := NewAttachmentService(clientMock, nil, "")
		isConflicted, conflictWithName, err := s.IsConflictedAttachment(context.Background(), vmbdaAlpha)
		Expect(err).To(BeNil())
		Expect(isConflicted).To(BeFalse())
		Expect(conflictWithName).To(BeEmpty())
	})

	// T1:    VMBDA B Phase: ""
	// T2: -->VMBDA A Should be Conflicted
	It("Should be Conflicted: there is another vmbda that created earlier", func() {
		vmbdaBeta.CreationTimestamp = metav1.Time{Time: vmbdaBeta.CreationTimestamp.Add(-time.Hour)}
		clientMock.ListFunc = func(ctx context.Context, list client.ObjectList, opts ...client.ListOption) error {
			list.(*v1alpha2.VirtualMachineBlockDeviceAttachmentList).Items = []v1alpha2.VirtualMachineBlockDeviceAttachment{
				*vmbdaAlpha,
				*vmbdaBeta,
			}
			return nil
		}

		s := NewAttachmentService(clientMock, nil, "")
		isConflicted, conflictWithName, err := s.IsConflictedAttachment(context.Background(), vmbdaAlpha)
		Expect(err).To(BeNil())
		Expect(isConflicted).To(BeTrue())
		Expect(conflictWithName).To(Equal(vmbdaBeta.Name))
	})

	// T1: -->VMBDA A Should be Non-Conflicted
	// T2:    VMBDA B Phase: ""
	It("Should be Non-Conflicted: there is another vmbda that created later", func() {
		vmbdaBeta.CreationTimestamp = metav1.Time{Time: vmbdaBeta.CreationTimestamp.Add(time.Hour)}
		clientMock.ListFunc = func(ctx context.Context, list client.ObjectList, opts ...client.ListOption) error {
			list.(*v1alpha2.VirtualMachineBlockDeviceAttachmentList).Items = []v1alpha2.VirtualMachineBlockDeviceAttachment{
				*vmbdaAlpha,
				*vmbdaBeta,
			}
			return nil
		}

		s := NewAttachmentService(clientMock, nil, "")
		isConflicted, conflictWithName, err := s.IsConflictedAttachment(context.Background(), vmbdaAlpha)
		Expect(err).To(BeNil())
		Expect(isConflicted).To(BeFalse())
		Expect(conflictWithName).To(BeEmpty())
	})

	// T1: -->VMBDA A Should be Non-Conflicted lexicographically
	// T1:    VMBDA B Phase: ""
	It("Should be Non-Conflicted lexicographically", func() {
		clientMock.ListFunc = func(ctx context.Context, list client.ObjectList, opts ...client.ListOption) error {
			list.(*v1alpha2.VirtualMachineBlockDeviceAttachmentList).Items = []v1alpha2.VirtualMachineBlockDeviceAttachment{
				*vmbdaAlpha,
				*vmbdaBeta,
			}
			return nil
		}

		s := NewAttachmentService(clientMock, nil, "")
		isConflicted, conflictWithName, err := s.IsConflictedAttachment(context.Background(), vmbdaAlpha)
		Expect(err).To(BeNil())
		Expect(isConflicted).To(BeFalse())
		Expect(conflictWithName).To(BeEmpty())
	})

	It("Only one vmbda", func() {
		clientMock.ListFunc = func(ctx context.Context, list client.ObjectList, opts ...client.ListOption) error {
			list.(*v1alpha2.VirtualMachineBlockDeviceAttachmentList).Items = []v1alpha2.VirtualMachineBlockDeviceAttachment{
				*vmbdaAlpha,
			}
			return nil
		}

		s := NewAttachmentService(clientMock, nil, "")
		isConflicted, conflictWithName, err := s.IsConflictedAttachment(context.Background(), vmbdaAlpha)
		Expect(err).To(BeNil())
		Expect(isConflicted).To(BeFalse())
		Expect(conflictWithName).To(BeEmpty())
	})
})
