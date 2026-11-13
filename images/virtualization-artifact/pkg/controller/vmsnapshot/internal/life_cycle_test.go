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
	"errors"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/ptr"
	virtv1 "kubevirt.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/deckhouse/virtualization-controller/pkg/common/testutil"
	"github.com/deckhouse/virtualization-controller/pkg/common/vmaccess"
	"github.com/deckhouse/virtualization-controller/pkg/controller/accessclaim"
	"github.com/deckhouse/virtualization-controller/pkg/controller/conditions"
	"github.com/deckhouse/virtualization-controller/pkg/eventrecord"
	coordinationv1alpha1 "github.com/deckhouse/virtualization/api/coordination/v1alpha1"
	"github.com/deckhouse/virtualization/api/coordination/v1alpha1/vmaccondition"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/vdcondition"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/vmcondition"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/vmscondition"
)

const testNamespace = "ns"

type fakeUnfreezer struct {
	calls int
}

func (u *fakeUnfreezer) Unfreeze(_ context.Context, _, _ string, _ types.UID) error {
	u.calls++
	return nil
}

// failingClaimReader fails the reads of access claims and passes everything else through.
// failingClaimPatcher fails every patch of an access claim.
type failingClaimPatcher struct {
	client.Client
}

func (c failingClaimPatcher) Patch(ctx context.Context, obj client.Object, patch client.Patch, opts ...client.PatchOption) error {
	if _, ok := obj.(*coordinationv1alpha1.VirtualMachineAccessClaim); ok {
		return errors.New("the API server is unavailable")
	}

	return c.Client.Patch(ctx, obj, patch, opts...)
}

type failingClaimReader struct {
	client.Client
}

func (c failingClaimReader) Get(ctx context.Context, key client.ObjectKey, obj client.Object, opts ...client.GetOption) error {
	if _, ok := obj.(*coordinationv1alpha1.VirtualMachineAccessClaim); ok {
		return errors.New("the API server is throttling requests")
	}

	return c.Client.Get(ctx, key, obj, opts...)
}

var _ = Describe("LifeCycle handler", func() {
	var (
		recorder    eventrecord.EventRecorderLogger
		snapshotter *SnapshotterMock
		storer      *StorerMock
		vd          *v1alpha2.VirtualDisk
		vm          *v1alpha2.VirtualMachine
		kvvm        *virtv1.VirtualMachine
		kvvmi       *virtv1.VirtualMachineInstance
		secret      *corev1.Secret
		vdSnapshot  *v1alpha2.VirtualDiskSnapshot
		vmSnapshot  *v1alpha2.VirtualMachineSnapshot
		objs        []client.Object
		unfreezer   *fakeUnfreezer
		fakeClient  client.WithWatch
	)

	newHandler := func() *LifeCycleHandler {
		var err error

		fakeClient, err = testutil.NewFakeClientWithObjects(objs...)
		Expect(err).NotTo(HaveOccurred())

		return NewLifeCycleHandler(recorder, snapshotter, storer, fakeClient, fakeClient, unfreezer)
	}

	readyCondition := func() metav1.Condition {
		ready, _ := conditions.GetCondition(vmscondition.VirtualMachineSnapshotReadyType, vmSnapshot.Status.Conditions)
		return ready
	}

	claimKey := types.NamespacedName{Namespace: testNamespace, Name: "vms-vms-uid"}

	getClaim := func() *coordinationv1alpha1.VirtualMachineAccessClaim {
		claim := &coordinationv1alpha1.VirtualMachineAccessClaim{}
		Expect(fakeClient.Get(testContext(), claimKey, claim)).To(Succeed())

		return claim
	}

	withClaim := func(phase coordinationv1alpha1.AccessClaimPhase, reason vmaccondition.GrantedReason, message string) *coordinationv1alpha1.VirtualMachineAccessClaim {
		claim := &coordinationv1alpha1.VirtualMachineAccessClaim{
			ObjectMeta: metav1.ObjectMeta{Name: claimKey.Name, Namespace: claimKey.Namespace},
			Spec: coordinationv1alpha1.VirtualMachineAccessClaimSpec{
				VirtualMachine: coordinationv1alpha1.VirtualMachineAccessClaimTarget{Name: vm.Name},
				Access:         []coordinationv1alpha1.AccessMode{coordinationv1alpha1.AccessModeSnapshot, coordinationv1alpha1.AccessModeFreeze},
				FreezePolicy:   ptr.To(coordinationv1alpha1.FreezePolicyRequired),
			},
			Status: coordinationv1alpha1.VirtualMachineAccessClaimStatus{Phase: phase},
		}
		if phase == coordinationv1alpha1.AccessClaimPhaseGranted || phase == coordinationv1alpha1.AccessClaimPhaseCompleted {
			claim.Status.GrantedAt = ptr.To(metav1.Now())
		}

		if reason != "" {
			meta.SetStatusCondition(&claim.Status.Conditions, metav1.Condition{
				Type:    vmaccondition.GrantedType.String(),
				Status:  metav1.ConditionFalse,
				Reason:  reason.String(),
				Message: message,
			})
		}

		objs = append(objs, claim)

		return claim
	}

	BeforeEach(func() {
		unfreezer = &fakeUnfreezer{}

		vd = &v1alpha2.VirtualDisk{
			ObjectMeta: metav1.ObjectMeta{Name: "vd-bar", Namespace: testNamespace},
			Status: v1alpha2.VirtualDiskStatus{
				Phase: v1alpha2.DiskReady,
				Conditions: []metav1.Condition{
					{
						Type:   vdcondition.Ready.String(),
						Status: metav1.ConditionTrue,
					},
				},
			},
		}

		vm = &v1alpha2.VirtualMachine{
			ObjectMeta: metav1.ObjectMeta{Name: "vm", Namespace: testNamespace, UID: "vm-uid"},
			Spec: v1alpha2.VirtualMachineSpec{
				BlockDeviceRefs: []v1alpha2.BlockDeviceSpecRef{
					{
						Kind: v1alpha2.DiskDevice,
						Name: vd.Name,
					},
				},
			},
			Status: v1alpha2.VirtualMachineStatus{
				Phase: v1alpha2.MachineRunning,
				BlockDeviceRefs: []v1alpha2.BlockDeviceStatusRef{
					{
						Kind: v1alpha2.DiskDevice,
						Name: vd.Name,
					},
				},
				Conditions: []metav1.Condition{
					{
						Type:   vmcondition.TypeBlockDevicesReady.String(),
						Status: metav1.ConditionTrue,
					},
				},
			},
		}

		kvvm = &virtv1.VirtualMachine{ObjectMeta: metav1.ObjectMeta{Name: "vm", Namespace: testNamespace}}
		kvvmi = &virtv1.VirtualMachineInstance{
			ObjectMeta: metav1.ObjectMeta{Name: "vm", Namespace: testNamespace},
			Status: virtv1.VirtualMachineInstanceStatus{
				Phase: virtv1.Running,
			},
		}

		secret = &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{Name: vm.Name, Namespace: testNamespace},
		}

		vmSnapshot = &v1alpha2.VirtualMachineSnapshot{
			ObjectMeta: metav1.ObjectMeta{Name: "vm-snapshot", Namespace: testNamespace, UID: "vms-uid"},
			Spec: v1alpha2.VirtualMachineSnapshotSpec{
				VirtualMachineName:  vm.Name,
				RequiredConsistency: true,
			},
			Status: v1alpha2.VirtualMachineSnapshotStatus{
				VirtualMachineSnapshotSecretName: "vm-snapshot",
				Conditions: []metav1.Condition{
					{
						Type:   vmscondition.VirtualMachineReadyType.String(),
						Status: metav1.ConditionTrue,
					},
				},
			},
		}

		vdSnapshot = &v1alpha2.VirtualDiskSnapshot{
			ObjectMeta: metav1.ObjectMeta{Name: getVDSnapshotName(vd.Name, vmSnapshot), Namespace: testNamespace},
			Status: v1alpha2.VirtualDiskSnapshotStatus{
				Phase: v1alpha2.VirtualDiskSnapshotPhaseReady,
			},
		}

		snapshotter = &SnapshotterMock{
			GetVirtualDiskFunc: func(_ context.Context, name, namespace string) (*v1alpha2.VirtualDisk, error) {
				return vd, nil
			},
			GetVirtualMachineFunc: func(_ context.Context, _, _ string) (*v1alpha2.VirtualMachine, error) {
				return vm, nil
			},
			GetSecretFunc: func(_ context.Context, _, _ string) (*corev1.Secret, error) {
				return secret, nil
			},
			GetVirtualDiskSnapshotFunc: func(_ context.Context, _, _ string) (*v1alpha2.VirtualDiskSnapshot, error) {
				return vdSnapshot, nil
			},
			GetVirtualMachineInstanceFunc: func(_ context.Context, _ *v1alpha2.VirtualMachine) (*virtv1.VirtualMachineInstance, error) {
				return kvvmi, nil
			},
		}

		objs = []client.Object{vd, vm, kvvm, kvvmi, secret, vmSnapshot, vdSnapshot}

		recorder = &eventrecord.EventRecorderLoggerMock{
			EventFunc: func(_ client.Object, _, _, _ string) {},
		}
	})

	Context("The virtual machine is not ready", func() {
		It("fails when the virtual machine is missing", func() {
			snapshotter.GetVirtualMachineFunc = func(_ context.Context, _, _ string) (*v1alpha2.VirtualMachine, error) {
				return nil, nil
			}

			h := newHandler()

			_, err := h.Handle(testContext(), vmSnapshot)
			Expect(err).To(BeNil())
			Expect(vmSnapshot.Status.Phase).To(Equal(v1alpha2.VirtualMachineSnapshotPhaseFailed))
			ready, _ := conditions.GetCondition(vmscondition.VirtualMachineSnapshotReadyType, vmSnapshot.Status.Conditions)
			Expect(ready.Status).To(Equal(metav1.ConditionFalse))
			Expect(ready.Reason).To(Equal(vmscondition.WaitingForTheVirtualMachine.String()))
			Expect(ready.Message).To(ContainSubstring("does not exist"))
		})

		It("fails when the virtual machine is reported as not ready", func() {
			cb := conditions.NewConditionBuilder(vmscondition.VirtualMachineReadyType).
				Generation(vmSnapshot.Generation).
				Status(metav1.ConditionFalse)
			conditions.SetCondition(cb, &vmSnapshot.Status.Conditions)

			h := newHandler()

			_, err := h.Handle(testContext(), vmSnapshot)
			Expect(err).To(BeNil())
			Expect(vmSnapshot.Status.Phase).To(Equal(v1alpha2.VirtualMachineSnapshotPhaseFailed))
			ready, _ := conditions.GetCondition(vmscondition.VirtualMachineSnapshotReadyType, vmSnapshot.Status.Conditions)
			Expect(ready.Status).To(Equal(metav1.ConditionFalse))
			Expect(ready.Reason).To(Equal(vmscondition.WaitingForTheVirtualMachine.String()))
			Expect(ready.Message).To(ContainSubstring("not ready"))
		})
	})

	Context("The block devices of the virtual machine are not in the consistent state", func() {
		It("The BlockDevicesReady condition of the virtual machine isn't True", func() {
			snapshotter.GetVirtualMachineFunc = func(_ context.Context, _, _ string) (*v1alpha2.VirtualMachine, error) {
				cb := conditions.NewConditionBuilder(vmcondition.TypeBlockDevicesReady).
					Generation(vm.Generation).
					Status(metav1.ConditionFalse)
				conditions.SetCondition(cb, &vm.Status.Conditions)

				return vm, nil
			}
			h := newHandler()

			_, err := h.Handle(testContext(), vmSnapshot)
			Expect(err).To(BeNil())
			Expect(vmSnapshot.Status.Phase).To(Equal(v1alpha2.VirtualMachineSnapshotPhaseFailed))
			ready, _ := conditions.GetCondition(vmscondition.VirtualMachineSnapshotReadyType, vmSnapshot.Status.Conditions)
			Expect(ready.Status).To(Equal(metav1.ConditionFalse))
			Expect(ready.Reason).To(Equal(vmscondition.BlockDevicesNotReady.String()))
			Expect(ready.Message).ToNot(BeEmpty())
		})

		It("The virtual disk is Pending", func() {
			snapshotter.GetVirtualDiskFunc = func(_ context.Context, _, _ string) (*v1alpha2.VirtualDisk, error) {
				vd.Status.Phase = v1alpha2.DiskPending
				return vd, nil
			}
			h := newHandler()

			_, err := h.Handle(testContext(), vmSnapshot)
			Expect(err).To(BeNil())
			Expect(vmSnapshot.Status.Phase).To(Equal(v1alpha2.VirtualMachineSnapshotPhaseFailed))
			ready, _ := conditions.GetCondition(vmscondition.VirtualMachineSnapshotReadyType, vmSnapshot.Status.Conditions)
			Expect(ready.Status).To(Equal(metav1.ConditionFalse))
			Expect(ready.Reason).To(Equal(vmscondition.BlockDevicesNotReady.String()))
			Expect(ready.Message).ToNot(BeEmpty())
		})

		It("The virtual disk is missing", func() {
			snapshotter.GetVirtualDiskFunc = func(_ context.Context, _, _ string) (*v1alpha2.VirtualDisk, error) {
				return nil, nil
			}
			h := newHandler()

			_, err := h.Handle(testContext(), vmSnapshot)
			Expect(err).To(BeNil())
			Expect(vmSnapshot.Status.Phase).To(Equal(v1alpha2.VirtualMachineSnapshotPhaseFailed))
			Expect(readyCondition().Reason).To(Equal(vmscondition.BlockDevicesNotReady.String()))
		})

		It("The virtual disk is not Ready", func() {
			snapshotter.GetVirtualDiskFunc = func(_ context.Context, _, _ string) (*v1alpha2.VirtualDisk, error) {
				cb := conditions.NewConditionBuilder(vdcondition.Ready).
					Generation(vd.Generation).
					Status(metav1.ConditionFalse)
				conditions.SetCondition(cb, &vd.Status.Conditions)

				return vd, nil
			}
			h := newHandler()

			_, err := h.Handle(testContext(), vmSnapshot)
			Expect(err).To(BeNil())
			Expect(vmSnapshot.Status.Phase).To(Equal(v1alpha2.VirtualMachineSnapshotPhaseFailed))
			ready, _ := conditions.GetCondition(vmscondition.VirtualMachineSnapshotReadyType, vmSnapshot.Status.Conditions)
			Expect(ready.Status).To(Equal(metav1.ConditionFalse))
			Expect(ready.Reason).To(Equal(vmscondition.BlockDevicesNotReady.String()))
			Expect(ready.Message).ToNot(BeEmpty())
		})

		It("The virtual disk is the process of Resizing", func() {
			snapshotter.GetVirtualDiskFunc = func(_ context.Context, _, _ string) (*v1alpha2.VirtualDisk, error) {
				cb := conditions.NewConditionBuilder(vdcondition.ResizingType).
					Generation(vd.Generation).
					Status(metav1.ConditionTrue).
					Reason(vdcondition.InProgress)
				conditions.SetCondition(cb, &vd.Status.Conditions)

				return vd, nil
			}
			h := newHandler()

			_, err := h.Handle(testContext(), vmSnapshot)
			Expect(err).To(BeNil())
			Expect(vmSnapshot.Status.Phase).To(Equal(v1alpha2.VirtualMachineSnapshotPhaseFailed))
			ready, _ := conditions.GetCondition(vmscondition.VirtualMachineSnapshotReadyType, vmSnapshot.Status.Conditions)
			Expect(ready.Status).To(Equal(metav1.ConditionFalse))
			Expect(ready.Reason).To(Equal(vmscondition.BlockDevicesNotReady.String()))
			Expect(ready.Message).ToNot(BeEmpty())
		})
	})

	Context("Access to the virtual machine", func() {
		It("keeps a ready snapshot as it is when its claim cannot be read", func() {
			vmSnapshot.Status.Phase = v1alpha2.VirtualMachineSnapshotPhaseReady
			meta.SetStatusCondition(&vmSnapshot.Status.Conditions, metav1.Condition{
				Type:   vmscondition.VirtualMachineSnapshotReadyType.String(),
				Status: metav1.ConditionTrue,
				Reason: vmscondition.VirtualMachineReady.String(),
			})

			h := newHandler()
			h.client = failingClaimReader{Client: fakeClient}
			h.apiReader = h.client

			_, err := h.Handle(testContext(), vmSnapshot)
			Expect(err).To(HaveOccurred())
			Expect(vmSnapshot.Status.Phase).To(Equal(v1alpha2.VirtualMachineSnapshotPhaseReady))
			Expect(readyCondition().Status).To(Equal(metav1.ConditionTrue))
			Expect(readyCondition().Reason).To(Equal(vmscondition.VirtualMachineReady.String()))
		})

		It("reads the claim of a ready snapshot from the cache alone", func() {
			vmSnapshot.Status.Phase = v1alpha2.VirtualMachineSnapshotPhaseReady
			meta.SetStatusCondition(&vmSnapshot.Status.Conditions, metav1.Condition{
				Type:   vmscondition.VirtualMachineSnapshotReadyType.String(),
				Status: metav1.ConditionTrue,
				Reason: vmscondition.VirtualMachineReady.String(),
			})

			h := newHandler()
			h.apiReader = failingClaimReader{Client: fakeClient}

			_, err := h.Handle(testContext(), vmSnapshot)
			Expect(err).To(BeNil())
			Expect(vmSnapshot.Status.Phase).To(Equal(v1alpha2.VirtualMachineSnapshotPhaseReady))
		})

		It("confirms an absent claim past the cache for a snapshot in progress", func() {
			vmSnapshot.Status.Phase = v1alpha2.VirtualMachineSnapshotPhaseInProgress
			h := newHandler()
			h.apiReader = failingClaimReader{Client: fakeClient}

			_, err := h.Handle(testContext(), vmSnapshot)
			Expect(err).To(HaveOccurred())
			Expect(vmSnapshot.Status.Phase).To(Equal(v1alpha2.VirtualMachineSnapshotPhaseInProgress))
			Expect(unfreezer.calls).To(BeZero())
		})

		DescribeTable("requests access before taking the snapshot",
			func(requiredConsistency bool, access []coordinationv1alpha1.AccessMode, policy *coordinationv1alpha1.FreezePolicy, waitReason vmscondition.VirtualMachineSnapshotReadyReason) {
				vmSnapshot.Spec.RequiredConsistency = requiredConsistency
				h := newHandler()

				_, err := h.Handle(testContext(), vmSnapshot)
				Expect(err).To(BeNil())

				claim := getClaim()
				Expect(claim.Spec.VirtualMachine.Name).To(Equal(vm.Name))
				Expect(claim.Spec.Access).To(ConsistOf(access))
				Expect(claim.Spec.FreezePolicy).To(Equal(policy))
				Expect(claim.Spec.Timeout.Duration).To(Equal(accessclaim.Timeout))
				Expect(metav1.IsControlledBy(claim, vmSnapshot)).To(BeTrue())

				Expect(vmSnapshot.Status.Phase).To(Equal(v1alpha2.VirtualMachineSnapshotPhasePending))
				Expect(readyCondition().Reason).To(Equal(waitReason.String()))
				Expect(snapshotter.GetVirtualDiskSnapshotCalls()).To(BeEmpty())
			},
			Entry("a consistent snapshot asks for the freeze", true,
				[]coordinationv1alpha1.AccessMode{coordinationv1alpha1.AccessModeSnapshot, coordinationv1alpha1.AccessModeFreeze},
				ptr.To(coordinationv1alpha1.FreezePolicyRequired), vmscondition.FileSystemFreezing),
			Entry("a snapshot without consistency does not freeze", false,
				[]coordinationv1alpha1.AccessMode{coordinationv1alpha1.AccessModeSnapshot}, nil, vmscondition.Snapshotting),
		)

		It("shows the coordinator's freeze progress under the reason the snapshot declares", func() {
			vmSnapshot.Status.Phase = v1alpha2.VirtualMachineSnapshotPhasePending

			withClaim(coordinationv1alpha1.AccessClaimPhasePending, vmaccondition.Freezing, "Waiting for the guest to confirm the filesystem freeze.")

			h := newHandler()

			_, err := h.Handle(testContext(), vmSnapshot)
			Expect(err).To(BeNil())
			Expect(vmSnapshot.Status.Phase).To(Equal(v1alpha2.VirtualMachineSnapshotPhasePending))
			Expect(readyCondition().Reason).To(Equal(vmscondition.FileSystemFreezing.String()))
			Expect(readyCondition().Message).To(Equal("Waiting for the guest to confirm the filesystem freeze."))
		})

		It("starts the snapshot once the access is granted", func() {
			vmSnapshot.Status.Phase = v1alpha2.VirtualMachineSnapshotPhasePending

			withClaim(coordinationv1alpha1.AccessClaimPhaseGranted, vmaccondition.Frozen, "")

			h := newHandler()

			_, err := h.Handle(testContext(), vmSnapshot)
			Expect(err).To(BeNil())
			Expect(vmSnapshot.Status.Phase).To(Equal(v1alpha2.VirtualMachineSnapshotPhaseInProgress))
			Expect(readyCondition().Reason).To(Equal(vmscondition.Snapshotting.String()))
		})

		It("fails when the access is refused", func() {
			vmSnapshot.Status.Phase = v1alpha2.VirtualMachineSnapshotPhasePending

			withClaim(coordinationv1alpha1.AccessClaimPhaseDenied, vmaccondition.VirtualMachineBusy, "The virtual machine is busy.")

			h := newHandler()

			_, err := h.Handle(testContext(), vmSnapshot)
			Expect(err).To(BeNil())
			Expect(vmSnapshot.Status.Phase).To(Equal(v1alpha2.VirtualMachineSnapshotPhaseFailed))
			Expect(readyCondition().Reason).To(Equal(vmscondition.VirtualMachineBusy.String()))
			Expect(vmSnapshot.Status.Consistent).To(Equal(ptr.To(false)))
			Expect(readyCondition().Message).To(Equal("The virtual machine is busy."))
		})

		It("fails when the session is aborted during the capture", func() {
			vmSnapshot.Status.Phase = v1alpha2.VirtualMachineSnapshotPhaseInProgress

			withClaim(coordinationv1alpha1.AccessClaimPhaseAborted, vmaccondition.VirtualMachineStateChanged, "The virtual machine has changed.")

			h := newHandler()

			_, err := h.Handle(testContext(), vmSnapshot)
			Expect(err).To(BeNil())
			Expect(vmSnapshot.Status.Phase).To(Equal(v1alpha2.VirtualMachineSnapshotPhaseFailed))
			Expect(readyCondition().Reason).To(Equal(vmscondition.VirtualMachineStateChanged.String()))
		})

		It("keeps the access while a disk snapshot is in progress", func() {
			vmSnapshot.Status.Phase = v1alpha2.VirtualMachineSnapshotPhaseInProgress
			vdSnapshot.Status.Phase = v1alpha2.VirtualDiskSnapshotPhaseInProgress

			withClaim(coordinationv1alpha1.AccessClaimPhaseGranted, vmaccondition.Frozen, "")

			h := newHandler()

			_, err := h.Handle(testContext(), vmSnapshot)
			Expect(err).To(BeNil())
			Expect(getClaim().Spec.Release).To(BeFalse())
			Expect(vmSnapshot.Status.Phase).To(Equal(v1alpha2.VirtualMachineSnapshotPhaseInProgress))
			Expect(readyCondition().Reason).To(Equal(vmscondition.Snapshotting.String()))
		})

		It("releases the access once every disk snapshot is ready and the resources are written", func() {
			vmSnapshot.Status.Phase = v1alpha2.VirtualMachineSnapshotPhaseInProgress
			claim := withClaim(coordinationv1alpha1.AccessClaimPhaseGranted, vmaccondition.Frozen, "")
			claim.Status.Frozen = true
			h := newHandler()

			res, err := h.Handle(testContext(), vmSnapshot)
			Expect(err).To(BeNil())
			Expect(vmSnapshot.Status.Resources).NotTo(BeEmpty())
			Expect(getClaim().Spec.Release).To(BeFalse())
			Expect(res.RequeueAfter).To(BeNumerically(">", 0))

			_, err = h.Handle(testContext(), vmSnapshot)
			Expect(err).To(BeNil())
			Expect(getClaim().Spec.Release).To(BeTrue())
			Expect(vmSnapshot.Status.Phase).To(Equal(v1alpha2.VirtualMachineSnapshotPhaseInProgress))
			Expect(readyCondition().Reason).To(Equal(vmscondition.FileSystemUnfreezing.String()))
			Expect(vmSnapshot.Status.VirtualDiskSnapshotNames).To(ConsistOf(vdSnapshot.Name))
			Expect(vmSnapshot.Status.Resources).NotTo(BeEmpty())
			Expect(vmSnapshot.Status.Consistent).To(BeNil())
		})

		It("keeps the condition it had when the access cannot be released", func() {
			vmSnapshot.Status.Phase = v1alpha2.VirtualMachineSnapshotPhaseInProgress
			vmSnapshot.Status.Resources = []v1alpha2.ResourceRef{{Kind: v1alpha2.VirtualMachineKind, Name: vm.Name}}
			meta.SetStatusCondition(&vmSnapshot.Status.Conditions, metav1.Condition{
				Type:    vmscondition.VirtualMachineSnapshotReadyType.String(),
				Status:  metav1.ConditionFalse,
				Reason:  vmscondition.Snapshotting.String(),
				Message: "Waiting for the virtual disk snapshots (1/1).",
			})
			withClaim(coordinationv1alpha1.AccessClaimPhaseGranted, vmaccondition.Frozen, "")

			h := newHandler()
			h.client = failingClaimPatcher{Client: fakeClient}

			_, err := h.Handle(testContext(), vmSnapshot)
			Expect(err).To(HaveOccurred())
			Expect(readyCondition().Reason).To(Equal(vmscondition.Snapshotting.String()))
			Expect(readyCondition().Message).To(Equal("Waiting for the virtual disk snapshots (1/1)."))
		})

		It("fails when a disk snapshot fails, and releases the access once the failure is written", func() {
			vmSnapshot.Status.Phase = v1alpha2.VirtualMachineSnapshotPhaseInProgress
			vdSnapshot.Status.Phase = v1alpha2.VirtualDiskSnapshotPhaseFailed

			withClaim(coordinationv1alpha1.AccessClaimPhaseGranted, vmaccondition.Frozen, "")

			h := newHandler()

			res, err := h.Handle(testContext(), vmSnapshot)
			Expect(err).To(BeNil())
			Expect(vmSnapshot.Status.Phase).To(Equal(v1alpha2.VirtualMachineSnapshotPhaseFailed))
			Expect(getClaim().Spec.Release).To(BeFalse())
			Expect(res.RequeueAfter).To(BeNumerically(">", 0))

			_, err = h.Handle(testContext(), vmSnapshot)
			Expect(err).To(BeNil())
			Expect(getClaim().Spec.Release).To(BeTrue())
		})

		It("fails instead of turning ready when the access completed before the resources were written", func() {
			vmSnapshot.Status.Phase = v1alpha2.VirtualMachineSnapshotPhaseInProgress
			claim := withClaim(coordinationv1alpha1.AccessClaimPhaseCompleted, vmaccondition.Released, "")
			claim.Spec.Release = true
			claim.Status.Frozen = true
			claim.Status.VirtualMachineInstanceUID = "vmi-uid"
			h := newHandler()

			_, err := h.Handle(testContext(), vmSnapshot)
			Expect(err).To(BeNil())
			Expect(vmSnapshot.Status.Phase).To(Equal(v1alpha2.VirtualMachineSnapshotPhaseFailed))
			Expect(vmSnapshot.Status.Consistent).To(Equal(ptr.To(false)))
		})

		DescribeTable("turns ready with the consistency of the verdict",
			func(frozen bool, instanceUID types.UID, consistent bool) {
				vmSnapshot.Status.Phase = v1alpha2.VirtualMachineSnapshotPhaseInProgress
				claim := withClaim(coordinationv1alpha1.AccessClaimPhaseCompleted, vmaccondition.Released, "")
				claim.Spec.Release = true
				claim.Status.Frozen = frozen
				claim.Status.VirtualMachineInstanceUID = instanceUID
				vmSnapshot.Status.Resources = []v1alpha2.ResourceRef{{Kind: v1alpha2.VirtualMachineKind, Name: vm.Name}}
				h := newHandler()

				_, err := h.Handle(testContext(), vmSnapshot)
				Expect(err).To(BeNil())
				Expect(vmSnapshot.Status.Phase).To(Equal(v1alpha2.VirtualMachineSnapshotPhaseReady))
				Expect(readyCondition().Status).To(Equal(metav1.ConditionTrue))
				Expect(vmSnapshot.Status.Consistent).To(Equal(ptr.To(consistent)))
			},
			Entry("a frozen guest", true, types.UID("vmi-uid"), true),
			Entry("a stopped virtual machine", false, types.UID(""), true),
			Entry("a guest that was not frozen", false, types.UID("vmi-uid"), false),
		)

		Context("a session whose guest is being unfrozen", func() {
			unfrozenClaim := func(reason vmaccondition.UnfrozenReason, message string) {
				claim := withClaim(coordinationv1alpha1.AccessClaimPhaseCompleted, vmaccondition.Released, "")
				claim.Spec.Release = true
				claim.Status.Frozen = true
				claim.Status.VirtualMachineInstanceUID = "vmi-uid"
				meta.SetStatusCondition(&claim.Status.Conditions, metav1.Condition{
					Type:    vmaccondition.UnfrozenType.String(),
					Status:  metav1.ConditionFalse,
					Reason:  reason.String(),
					Message: message,
				})
			}

			BeforeEach(func() {
				vmSnapshot.Status.Phase = v1alpha2.VirtualMachineSnapshotPhaseInProgress
				vmSnapshot.Status.Resources = []v1alpha2.ResourceRef{{Kind: v1alpha2.VirtualMachineKind, Name: vm.Name}}
			})

			It("waits for the unfreeze before turning ready", func() {
				unfrozenClaim(vmaccondition.Unfreezing, "")

				h := newHandler()

				_, err := h.Handle(testContext(), vmSnapshot)
				Expect(err).To(BeNil())
				Expect(vmSnapshot.Status.Phase).To(Equal(v1alpha2.VirtualMachineSnapshotPhaseInProgress))
				Expect(readyCondition().Reason).To(Equal(vmscondition.FileSystemUnfreezing.String()))
			})

			It("turns ready and warns when the guest does not confirm the unfreeze", func() {
				var warnings []string

				recorder = &eventrecord.EventRecorderLoggerMock{
					EventFunc: func(_ client.Object, eventtype, reason, _ string) {
						if eventtype == corev1.EventTypeWarning {
							warnings = append(warnings, reason)
						}
					},
				}

				unfrozenClaim(vmaccondition.UnfreezeNotConfirmed, "The guest did not confirm the unfreeze.")

				h := newHandler()

				_, err := h.Handle(testContext(), vmSnapshot)
				Expect(err).To(BeNil())
				Expect(vmSnapshot.Status.Phase).To(Equal(v1alpha2.VirtualMachineSnapshotPhaseReady))
				Expect(vmSnapshot.Status.Consistent).To(Equal(ptr.To(true)))
				Expect(readyCondition().Message).To(Equal("The guest did not confirm the unfreeze."))
				Expect(warnings).To(Equal([]string{v1alpha2.ReasonFilesystemUnfreezeNotConfirmed}))

				_, err = h.Handle(testContext(), vmSnapshot)
				Expect(err).To(BeNil())
				Expect(readyCondition().Message).To(Equal("The guest did not confirm the unfreeze."))
				Expect(warnings).To(HaveLen(1))
			})
		})

		It("keeps the recorded failure reason and releases the access of a failed snapshot", func() {
			vmSnapshot.Status.Phase = v1alpha2.VirtualMachineSnapshotPhaseFailed
			conditions.SetCondition(
				conditions.NewConditionBuilder(vmscondition.VirtualMachineSnapshotReadyType).
					Status(metav1.ConditionFalse).
					Reason(vmscondition.VirtualMachineSnapshotFailed).
					Message("The disk snapshot has failed."),
				&vmSnapshot.Status.Conditions,
			)
			withClaim(coordinationv1alpha1.AccessClaimPhaseGranted, vmaccondition.Frozen, "")

			h := newHandler()

			_, err := h.Handle(testContext(), vmSnapshot)
			Expect(err).To(BeNil())
			Expect(readyCondition().Reason).To(Equal(vmscondition.VirtualMachineSnapshotFailed.String()))
			Expect(readyCondition().Message).To(Equal("The disk snapshot has failed."))
			Expect(getClaim().Spec.Release).To(BeTrue())
		})

		It("releases the access of a deleted snapshot", func() {
			vmSnapshot.Status.Phase = v1alpha2.VirtualMachineSnapshotPhaseInProgress
			vmSnapshot.DeletionTimestamp = ptr.To(metav1.Now())
			vmSnapshot.Finalizers = []string{v1alpha2.FinalizerVMSnapshotProtection}

			withClaim(coordinationv1alpha1.AccessClaimPhaseGranted, vmaccondition.Frozen, "")

			h := newHandler()

			_, err := h.Handle(testContext(), vmSnapshot)
			Expect(err).To(BeNil())
			Expect(getClaim().Spec.Release).To(BeTrue())
			Expect(vmSnapshot.Status.Phase).To(Equal(v1alpha2.VirtualMachineSnapshotPhaseTerminating))
			Expect(unfreezer.calls).To(BeZero())
		})
	})

	Context("A snapshot started before the module update", func() {
		BeforeEach(func() {
			vmSnapshot.Status.Phase = v1alpha2.VirtualMachineSnapshotPhaseInProgress
		})

		It("fails and unfreezes the guest it may have left frozen", func() {
			h := newHandler()

			_, err := h.Handle(testContext(), vmSnapshot)
			Expect(err).To(BeNil())
			Expect(vmSnapshot.Status.Phase).To(Equal(v1alpha2.VirtualMachineSnapshotPhaseFailed))
			Expect(vmSnapshot.Status.Consistent).To(Equal(ptr.To(false)))
			Expect(readyCondition().Reason).To(Equal(vmscondition.AbortedByModuleUpdate.String()))
			Expect(unfreezer.calls).To(Equal(1))
		})

		It("leaves a guest held by a coordinator session alone", func() {
			Expect(vmaccess.WriteLock(kvvm, &vmaccess.Lock{ClaimUID: "other"})).To(Succeed())

			h := newHandler()

			_, err := h.Handle(testContext(), vmSnapshot)
			Expect(err).To(BeNil())
			Expect(vmSnapshot.Status.Phase).To(Equal(v1alpha2.VirtualMachineSnapshotPhaseFailed))
			Expect(unfreezer.calls).To(BeZero())
		})

		It("unfreezes the guest when deleted", func() {
			vmSnapshot.DeletionTimestamp = ptr.To(metav1.Now())
			vmSnapshot.Finalizers = []string{v1alpha2.FinalizerVMSnapshotProtection}
			h := newHandler()

			_, err := h.Handle(testContext(), vmSnapshot)
			Expect(err).To(BeNil())
			Expect(unfreezer.calls).To(Equal(1))
		})
	})

	Context("fill status resources", func() {
		It("includes a mac address resource for a single non-main network", func() {
			vmmac := &v1alpha2.VirtualMachineMACAddress{
				TypeMeta: metav1.TypeMeta{
					Kind:       v1alpha2.VirtualMachineMACAddressKind,
					APIVersion: v1alpha2.SchemeGroupVersion.String(),
				},
				ObjectMeta: metav1.ObjectMeta{
					Name:      "vm-mac-secondary",
					Namespace: vm.Namespace,
				},
			}

			vm.Spec.Networks = []v1alpha2.NetworksSpec{
				{
					Type: v1alpha2.NetworksTypeNetwork,
				},
			}
			vm.Status.Networks = []v1alpha2.NetworksStatus{
				{
					Type:                         v1alpha2.NetworksTypeNetwork,
					VirtualMachineMACAddressName: vmmac.Name,
				},
			}

			var err error

			fakeClient, err = testutil.NewFakeClientWithObjects(vd, vm, vmSnapshot, vdSnapshot, vmmac)
			Expect(err).NotTo(HaveOccurred())

			h := NewLifeCycleHandler(recorder, snapshotter, storer, fakeClient, fakeClient, unfreezer)
			err = h.fillStatusResources(testContext(), vmSnapshot, vm)
			Expect(err).NotTo(HaveOccurred())

			Expect(vmSnapshot.Status.Resources).To(ContainElement(v1alpha2.ResourceRef{
				Kind:       vmmac.Kind,
				ApiVersion: vmmac.APIVersion,
				Name:       vmmac.Name,
			}))
		})

		It("skips virtual machine ip address lookup when the status reference is empty", func() {
			vm.Spec.Networks = nil
			vm.Status.Networks = nil
			vm.Spec.BlockDeviceRefs = nil
			vm.Status.BlockDeviceRefs = nil
			vmSnapshot.Spec.KeepIPAddress = v1alpha2.KeepIPAddressAlways
			vm.Status.VirtualMachineIPAddress = ""

			var err error

			fakeClient, err = testutil.NewFakeClientWithObjects(vm, vmSnapshot)
			Expect(err).NotTo(HaveOccurred())

			h := NewLifeCycleHandler(recorder, snapshotter, storer, fakeClient, fakeClient, unfreezer)
			err = h.fillStatusResources(testContext(), vmSnapshot, vm)
			Expect(err).NotTo(HaveOccurred())
			Expect(vmSnapshot.Status.Resources).To(Equal([]v1alpha2.ResourceRef{
				{
					Kind:       vm.Kind,
					ApiVersion: vm.APIVersion,
					Name:       vm.Name,
				},
			}))
		})
	})
})
