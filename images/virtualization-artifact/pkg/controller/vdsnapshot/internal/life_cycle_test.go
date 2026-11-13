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

	vsv1 "github.com/kubernetes-csi/external-snapshotter/client/v6/apis/volumesnapshot/v1"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/ptr"
	virtv1 "kubevirt.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/deckhouse/virtualization-controller/pkg/common/annotations"
	"github.com/deckhouse/virtualization-controller/pkg/common/testutil"
	"github.com/deckhouse/virtualization-controller/pkg/common/vmaccess"
	"github.com/deckhouse/virtualization-controller/pkg/controller/accessclaim"
	"github.com/deckhouse/virtualization-controller/pkg/controller/conditions"
	"github.com/deckhouse/virtualization-controller/pkg/controller/reconciler"
	"github.com/deckhouse/virtualization-controller/pkg/controller/service"
	"github.com/deckhouse/virtualization-controller/pkg/eventrecord"
	coordinationv1alpha1 "github.com/deckhouse/virtualization/api/coordination/v1alpha1"
	"github.com/deckhouse/virtualization/api/coordination/v1alpha1/vmaccondition"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/vdscondition"
)

const testNamespace = "ns"

type fakeUnfreezer struct {
	calls int
}

func (u *fakeUnfreezer) Unfreeze(_ context.Context, _, _ string, _ types.UID) error {
	u.calls++
	return nil
}

// failingClaimClient fails the reads or the writes of access claims and passes everything else through.
type failingClaimClient struct {
	client.Client
	failGet   bool
	failPatch bool
}

func (c failingClaimClient) Get(ctx context.Context, key client.ObjectKey, obj client.Object, opts ...client.GetOption) error {
	if _, ok := obj.(*coordinationv1alpha1.VirtualMachineAccessClaim); ok && c.failGet {
		return errors.New("the API server is throttling requests")
	}

	return c.Client.Get(ctx, key, obj, opts...)
}

func (c failingClaimClient) Patch(ctx context.Context, obj client.Object, patch client.Patch, opts ...client.PatchOption) error {
	if _, ok := obj.(*coordinationv1alpha1.VirtualMachineAccessClaim); ok && c.failPatch {
		return errors.New("the API server is unavailable")
	}

	return c.Client.Patch(ctx, obj, patch, opts...)
}

var _ = Describe("LifeCycle handler", func() {
	var (
		snapshotter *LifeCycleSnapshotterMock
		pvc         *corev1.PersistentVolumeClaim
		vd          *v1alpha2.VirtualDisk
		vs          *vsv1.VolumeSnapshot
		vdSnapshot  *v1alpha2.VirtualDiskSnapshot
		objs        []client.Object
		unfreezer   *fakeUnfreezer
		fakeClient  client.Client
		warnings    []string
	)

	newHandler := func(volumeSnapshotAPIAvailable bool) *LifeCycleHandler {
		var err error

		fakeClient, err = testutil.NewFakeClientWithObjects(objs...)
		Expect(err).NotTo(HaveOccurred())

		recorder := &eventrecord.EventRecorderLoggerMock{
			EventFunc: func(_ client.Object, eventtype, reason, _ string) {
				if eventtype == corev1.EventTypeWarning {
					warnings = append(warnings, reason)
				}
			},
		}

		return NewLifeCycleHandler(recorder, snapshotter, fakeClient, fakeClient, unfreezer, volumeSnapshotAPIAvailable)
	}

	readyCondition := func() metav1.Condition {
		ready, _ := conditions.GetCondition(vdscondition.VirtualDiskSnapshotReadyType, vdSnapshot.Status.Conditions)
		return ready
	}

	BeforeEach(func() {
		objs = nil
		warnings = nil
		unfreezer = &fakeUnfreezer{}

		pvc = &corev1.PersistentVolumeClaim{
			ObjectMeta: metav1.ObjectMeta{Name: "pvc-01", Namespace: testNamespace},
			Status: corev1.PersistentVolumeClaimStatus{
				Phase: corev1.ClaimBound,
			},
		}

		vd = &v1alpha2.VirtualDisk{
			ObjectMeta: metav1.ObjectMeta{Name: "vd-01", Namespace: testNamespace},
			Status: v1alpha2.VirtualDiskStatus{
				Target: v1alpha2.DiskTarget{
					PersistentVolumeClaim: pvc.Name,
				},
			},
		}

		vs = &vsv1.VolumeSnapshot{
			ObjectMeta: metav1.ObjectMeta{Name: "vs-01", Namespace: testNamespace},
		}

		vdSnapshot = &v1alpha2.VirtualDiskSnapshot{
			ObjectMeta: metav1.ObjectMeta{Name: "vdsnapshot", Namespace: testNamespace, UID: "vds-uid"},
			Spec:       v1alpha2.VirtualDiskSnapshotSpec{VirtualDiskName: vd.Name},
			Status: v1alpha2.VirtualDiskSnapshotStatus{
				Conditions: []metav1.Condition{
					{
						Type:   vdscondition.VirtualDiskReadyType.String(),
						Status: metav1.ConditionTrue,
					},
				},
			},
		}

		snapshotter = &LifeCycleSnapshotterMock{
			CreateVolumeSnapshotFunc: func(_ context.Context, _ *vsv1.VolumeSnapshot) (*vsv1.VolumeSnapshot, error) {
				return vs, nil
			},
			GetPersistentVolumeClaimFunc: func(_ context.Context, _, _ string) (*corev1.PersistentVolumeClaim, error) {
				return pvc, nil
			},
			GetVirtualDiskFunc: func(_ context.Context, _, _ string) (*v1alpha2.VirtualDisk, error) {
				return vd, nil
			},
			GetVirtualMachineFunc: func(_ context.Context, _, _ string) (*v1alpha2.VirtualMachine, error) {
				return nil, nil
			},
			GetVolumeSnapshotFunc: func(_ context.Context, _, _ string) (*vsv1.VolumeSnapshot, error) {
				return nil, nil
			},
			GetVirtualMachineInstanceFunc: func(_ context.Context, _ *v1alpha2.VirtualMachine) (*virtv1.VirtualMachineInstance, error) {
				return nil, nil
			},
		}
	})

	Context("The virtual disk snapshot without virtual machine", func() {
		BeforeEach(func() {
			vdSnapshot.Annotations = map[string]string{annotations.AnnStartedOnUnattachedDisk: "true"}
		})

		It("marks the snapshot as started on an unattached disk before it leaves Pending", func() {
			vdSnapshot.Annotations = nil
			h := newHandler(true)

			res, err := h.Handle(testContext(), vdSnapshot)
			Expect(err).To(BeNil())
			Expect(res.IsZero()).To(BeFalse())
			Expect(vdSnapshot.Annotations).To(HaveKey(annotations.AnnStartedOnUnattachedDisk))
			Expect(vdSnapshot.Status.Phase).To(Equal(v1alpha2.VirtualDiskSnapshotPhasePending))

			_, err = h.Handle(testContext(), vdSnapshot)
			Expect(err).To(BeNil())
			Expect(vdSnapshot.Status.Phase).To(Equal(v1alpha2.VirtualDiskSnapshotPhaseInProgress))
		})

		It("fails a snapshot in progress without the mark, started before the module update", func() {
			vdSnapshot.Annotations = nil
			vdSnapshot.Status.Phase = v1alpha2.VirtualDiskSnapshotPhaseInProgress
			h := newHandler(true)

			_, err := h.Handle(testContext(), vdSnapshot)
			Expect(err).To(BeNil())
			Expect(vdSnapshot.Status.Phase).To(Equal(v1alpha2.VirtualDiskSnapshotPhaseFailed))
			ready, _ := conditions.GetCondition(vdscondition.VirtualDiskSnapshotReadyType, vdSnapshot.Status.Conditions)
			Expect(ready.Reason).To(Equal(vdscondition.AbortedByModuleUpdate.String()))
			Expect(vdSnapshot.Status.Consistent).To(HaveValue(BeFalse()))
		})

		It("The volume snapshot has created", func() {
			h := newHandler(true)

			_, err := h.Handle(testContext(), vdSnapshot)
			Expect(err).To(BeNil())
			Expect(vdSnapshot.Status.Phase).To(Equal(v1alpha2.VirtualDiskSnapshotPhaseInProgress))
			ready, _ := conditions.GetCondition(vdscondition.VirtualDiskSnapshotReadyType, vdSnapshot.Status.Conditions)
			Expect(ready.Status).To(Equal(metav1.ConditionFalse))
			Expect(ready.Reason).To(Equal(vdscondition.Snapshotting.String()))
			Expect(ready.Message).ToNot(BeEmpty())
		})

		DescribeTable("stores original size in volume snapshot annotations",
			func(vdSize, pvcSize, expectedSize string) {
				if vdSize != "" {
					size := resource.MustParse(vdSize)
					vd.Spec.PersistentVolumeClaim.Size = &size
				}

				if pvcSize != "" {
					pvc.Spec.Resources.Requests = corev1.ResourceList{
						corev1.ResourceStorage: resource.MustParse(pvcSize),
					}
				}

				snapshotter.CreateVolumeSnapshotFunc = func(_ context.Context, vs *vsv1.VolumeSnapshot) (*vsv1.VolumeSnapshot, error) {
					Expect(vs.Annotations[annotations.AnnVirtualDiskOriginalSize]).To(Equal(expectedSize))
					return vs, nil
				}

				h := newHandler(true)

				_, err := h.Handle(testContext(), vdSnapshot)
				Expect(err).To(BeNil())
			},
			Entry("from VirtualDisk spec size", "20Gi", "10Gi", "20Gi"),
			Entry("from PVC requested size when VirtualDisk spec size is omitted", "", "10Gi", "10Gi"),
		)

		It("stores the selected node of the pvc in volume snapshot annotations", func() {
			pvc.Annotations = map[string]string{service.SelectedNodeAnnotation: "node-1"}

			snapshotter.CreateVolumeSnapshotFunc = func(_ context.Context, vs *vsv1.VolumeSnapshot) (*vsv1.VolumeSnapshot, error) {
				Expect(vs.Annotations[annotations.AnnVirtualDiskOriginalSelectedNode]).To(Equal("node-1"))
				return vs, nil
			}

			h := newHandler(true)

			_, err := h.Handle(testContext(), vdSnapshot)
			Expect(err).To(BeNil())
		})

		It("fails without the VolumeSnapshot API before requesting access", func() {
			h := newHandler(false)

			res, err := h.Handle(testContext(), vdSnapshot)
			Expect(err).To(BeNil())
			Expect(res).To(Equal(reconcile.Result{}))
			Expect(vdSnapshot.Status.Phase).To(Equal(v1alpha2.VirtualDiskSnapshotPhaseFailed))
			ready, _ := conditions.GetCondition(vdscondition.VirtualDiskSnapshotReadyType, vdSnapshot.Status.Conditions)
			Expect(ready.Status).To(Equal(metav1.ConditionFalse))
			Expect(ready.Reason).To(Equal(vdscondition.VirtualDiskSnapshotFailed.String()))
			Expect(snapshotter.GetVirtualMachineCalls()).To(BeEmpty())
			Expect(snapshotter.CreateVolumeSnapshotCalls()).To(BeEmpty())
		})

		It("The volume snapshot has failed", func() {
			snapshotter.GetVolumeSnapshotFunc = func(_ context.Context, _, _ string) (*vsv1.VolumeSnapshot, error) {
				vs.Status = &vsv1.VolumeSnapshotStatus{
					Error: &vsv1.VolumeSnapshotError{
						Message: ptr.To("error"),
					},
				}

				return vs, nil
			}

			h := newHandler(true)

			_, err := h.Handle(testContext(), vdSnapshot)
			Expect(err).To(BeNil())
			Expect(vdSnapshot.Status.Phase).To(Equal(v1alpha2.VirtualDiskSnapshotPhaseFailed))
			ready, _ := conditions.GetCondition(vdscondition.VirtualDiskSnapshotReadyType, vdSnapshot.Status.Conditions)
			Expect(ready.Status).To(Equal(metav1.ConditionFalse))
			Expect(ready.Reason).To(Equal(vdscondition.VirtualDiskSnapshotFailed.String()))
			Expect(ready.Message).ToNot(BeEmpty())
		})

		It("The volume snapshot is not ready yet", func() {
			snapshotter.GetVolumeSnapshotFunc = func(_ context.Context, _, _ string) (*vsv1.VolumeSnapshot, error) {
				return vs, nil
			}

			h := newHandler(true)

			_, err := h.Handle(testContext(), vdSnapshot)
			Expect(err).To(BeNil())
			Expect(vdSnapshot.Status.Phase).To(Equal(v1alpha2.VirtualDiskSnapshotPhaseInProgress))
			ready, _ := conditions.GetCondition(vdscondition.VirtualDiskSnapshotReadyType, vdSnapshot.Status.Conditions)
			Expect(ready.Status).To(Equal(metav1.ConditionFalse))
			Expect(ready.Reason).To(Equal(vdscondition.Snapshotting.String()))
			Expect(ready.Message).ToNot(BeEmpty())
		})

		It("The volume snapshot is ready", func() {
			snapshotter.GetVolumeSnapshotFunc = func(_ context.Context, _, _ string) (*vsv1.VolumeSnapshot, error) {
				vs.Status = &vsv1.VolumeSnapshotStatus{
					ReadyToUse: ptr.To(true),
				}

				return vs, nil
			}

			h := newHandler(true)

			_, err := h.Handle(testContext(), vdSnapshot)
			Expect(err).To(BeNil())
			_, err = h.Handle(testContext(), vdSnapshot)
			Expect(err).To(BeNil())
			Expect(vdSnapshot.Status.Phase).To(Equal(v1alpha2.VirtualDiskSnapshotPhaseReady))
			Expect(vdSnapshot.Status.Consistent).To(Equal(ptr.To(true)))
			ready, _ := conditions.GetCondition(vdscondition.VirtualDiskSnapshotReadyType, vdSnapshot.Status.Conditions)
			Expect(ready.Status).To(Equal(metav1.ConditionTrue))
			Expect(ready.Reason).To(Equal(vdscondition.VirtualDiskSnapshotReady.String()))
			Expect(ready.Message).To(BeEmpty())
		})

		It("fails when the virtual disk is missing", func() {
			snapshotter.GetVirtualDiskFunc = func(_ context.Context, _, _ string) (*v1alpha2.VirtualDisk, error) {
				return nil, nil
			}

			h := newHandler(true)

			_, err := h.Handle(testContext(), vdSnapshot)
			Expect(err).To(BeNil())
			Expect(vdSnapshot.Status.Phase).To(Equal(v1alpha2.VirtualDiskSnapshotPhaseFailed))
			ready, _ := conditions.GetCondition(vdscondition.VirtualDiskSnapshotReadyType, vdSnapshot.Status.Conditions)
			Expect(ready.Status).To(Equal(metav1.ConditionFalse))
			Expect(ready.Reason).To(Equal(vdscondition.WaitingForTheVirtualDisk.String()))
			Expect(ready.Message).To(ContainSubstring("does not exist"))
		})

		It("fails when the virtual disk pvc is not Bound", func() {
			pvc.Status.Phase = corev1.ClaimPending

			h := newHandler(true)

			_, err := h.Handle(testContext(), vdSnapshot)
			Expect(err).To(BeNil())
			Expect(vdSnapshot.Status.Phase).To(Equal(v1alpha2.VirtualDiskSnapshotPhaseFailed))
			ready, _ := conditions.GetCondition(vdscondition.VirtualDiskSnapshotReadyType, vdSnapshot.Status.Conditions)
			Expect(ready.Status).To(Equal(metav1.ConditionFalse))
			Expect(ready.Reason).To(Equal(vdscondition.WaitingForTheVirtualDisk.String()))
			Expect(ready.Message).To(ContainSubstring("not in phase"))
		})
	})

	Context("The virtual disk snapshot of a disk attached to a virtual machine", func() {
		var (
			vm    *v1alpha2.VirtualMachine
			kvvmi *virtv1.VirtualMachineInstance
			kvvm  *virtv1.VirtualMachine
		)

		claimKey := types.NamespacedName{Namespace: testNamespace, Name: "vds-vds-uid"}

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

		readyVolumeSnapshot := func() {
			snapshotter.GetVolumeSnapshotFunc = func(_ context.Context, _, _ string) (*vsv1.VolumeSnapshot, error) {
				vs.Status = &vsv1.VolumeSnapshotStatus{ReadyToUse: ptr.To(true)}
				return vs, nil
			}
		}

		BeforeEach(func() {
			vm = &v1alpha2.VirtualMachine{
				ObjectMeta: metav1.ObjectMeta{Name: "vm", Namespace: testNamespace, UID: "vm-uid"},
				Status:     v1alpha2.VirtualMachineStatus{Phase: v1alpha2.MachineRunning},
			}
			kvvmi = &virtv1.VirtualMachineInstance{
				ObjectMeta: metav1.ObjectMeta{Name: "vm", Namespace: testNamespace},
				Status:     virtv1.VirtualMachineInstanceStatus{Phase: virtv1.Running},
			}
			kvvm = &virtv1.VirtualMachine{ObjectMeta: metav1.ObjectMeta{Name: "vm", Namespace: testNamespace}}
			objs = append(objs, kvvm, kvvmi)

			vd.Status.AttachedToVirtualMachines = []v1alpha2.AttachedVirtualMachine{{Name: vm.Name}}
			vdSnapshot.Spec.RequiredConsistency = true

			snapshotter.GetVirtualMachineFunc = func(_ context.Context, _, _ string) (*v1alpha2.VirtualMachine, error) {
				return vm, nil
			}
			snapshotter.GetVirtualMachineInstanceFunc = func(_ context.Context, _ *v1alpha2.VirtualMachine) (*virtv1.VirtualMachineInstance, error) {
				return kvvmi, nil
			}
		})

		DescribeTable("requests access before taking the snapshot",
			func(requiredConsistency bool, access []coordinationv1alpha1.AccessMode, policy *coordinationv1alpha1.FreezePolicy, reason vdscondition.VirtualDiskSnapshotReadyReason) {
				vdSnapshot.Spec.RequiredConsistency = requiredConsistency
				h := newHandler(true)

				_, err := h.Handle(testContext(), vdSnapshot)
				Expect(err).To(BeNil())

				claim := getClaim()
				Expect(claim.Spec.VirtualMachine.Name).To(Equal(vm.Name))
				Expect(claim.Spec.Access).To(Equal(access))
				Expect(claim.Spec.FreezePolicy).To(Equal(policy))
				Expect(claim.Spec.Timeout.Duration).To(Equal(accessclaim.Timeout))
				Expect(metav1.IsControlledBy(claim, vdSnapshot)).To(BeTrue())

				Expect(vdSnapshot.Status.Phase).To(Equal(v1alpha2.VirtualDiskSnapshotPhasePending))
				Expect(readyCondition().Reason).To(Equal(reason.String()))
				Expect(snapshotter.CreateVolumeSnapshotCalls()).To(BeEmpty())
			},
			Entry("takes the snapshot without a freeze when consistency is not required", false,
				[]coordinationv1alpha1.AccessMode{coordinationv1alpha1.AccessModeSnapshot}, nil, vdscondition.Snapshotting),
			Entry("requires the freeze for a consistent snapshot", true,
				[]coordinationv1alpha1.AccessMode{coordinationv1alpha1.AccessModeSnapshot, coordinationv1alpha1.AccessModeFreeze},
				ptr.To(coordinationv1alpha1.FreezePolicyRequired), vdscondition.FileSystemFreezing),
		)

		It("shows the coordinator's freeze progress under the reason the snapshot declares", func() {
			withClaim(coordinationv1alpha1.AccessClaimPhasePending, vmaccondition.Freezing, "Waiting for the guest to confirm the filesystem freeze.")

			h := newHandler(true)

			_, err := h.Handle(testContext(), vdSnapshot)
			Expect(err).To(BeNil())
			Expect(vdSnapshot.Status.Phase).To(Equal(v1alpha2.VirtualDiskSnapshotPhasePending))
			Expect(readyCondition().Reason).To(Equal(vdscondition.FileSystemFreezing.String()))
			Expect(readyCondition().Message).To(Equal("Waiting for the guest to confirm the filesystem freeze."))
		})

		It("takes the snapshot once the access is granted", func() {
			withClaim(coordinationv1alpha1.AccessClaimPhaseGranted, vmaccondition.Frozen, "")

			h := newHandler(true)

			_, err := h.Handle(testContext(), vdSnapshot)
			Expect(err).To(BeNil())
			Expect(vdSnapshot.Status.Phase).To(Equal(v1alpha2.VirtualDiskSnapshotPhaseInProgress))

			_, err = h.Handle(testContext(), vdSnapshot)
			Expect(err).To(BeNil())
			Expect(snapshotter.CreateVolumeSnapshotCalls()).To(HaveLen(1))
		})

		It("fails when the access is refused", func() {
			withClaim(coordinationv1alpha1.AccessClaimPhaseDenied, vmaccondition.VirtualMachineBusy, "The virtual machine is busy.")

			h := newHandler(true)

			_, err := h.Handle(testContext(), vdSnapshot)
			Expect(err).To(BeNil())
			Expect(vdSnapshot.Status.Phase).To(Equal(v1alpha2.VirtualDiskSnapshotPhaseFailed))
			Expect(vdSnapshot.Status.Consistent).To(Equal(ptr.To(false)))
			Expect(readyCondition().Reason).To(Equal(vdscondition.VirtualMachineBusy.String()))
			Expect(readyCondition().Message).To(Equal("The virtual machine is busy."))
			Expect(snapshotter.CreateVolumeSnapshotCalls()).To(BeEmpty())
		})

		It("fails when the session is aborted during the capture", func() {
			vdSnapshot.Status.Phase = v1alpha2.VirtualDiskSnapshotPhaseInProgress

			withClaim(coordinationv1alpha1.AccessClaimPhaseAborted, vmaccondition.VirtualMachineStateChanged, "The virtual machine has changed.")

			snapshotter.GetVolumeSnapshotFunc = func(_ context.Context, _, _ string) (*vsv1.VolumeSnapshot, error) {
				return vs, nil
			}
			h := newHandler(true)

			_, err := h.Handle(testContext(), vdSnapshot)
			Expect(err).To(BeNil())
			Expect(vdSnapshot.Status.Phase).To(Equal(v1alpha2.VirtualDiskSnapshotPhaseFailed))
			Expect(readyCondition().Reason).To(Equal(vdscondition.VirtualMachineStateChanged.String()))
		})

		It("keeps the condition it had when the access cannot be released", func() {
			vdSnapshot.Status.Phase = v1alpha2.VirtualDiskSnapshotPhaseInProgress
			meta.SetStatusCondition(&vdSnapshot.Status.Conditions, metav1.Condition{
				Type:    vdscondition.VirtualDiskSnapshotReadyType.String(),
				Status:  metav1.ConditionFalse,
				Reason:  vdscondition.Snapshotting.String(),
				Message: "Waiting for the snapshot to be ready to use.",
			})
			withClaim(coordinationv1alpha1.AccessClaimPhaseGranted, vmaccondition.Frozen, "")
			readyVolumeSnapshot()

			h := newHandler(true)
			h.client = failingClaimClient{Client: fakeClient, failPatch: true}

			_, err := h.Handle(testContext(), vdSnapshot)
			Expect(err).To(HaveOccurred())
			Expect(readyCondition().Reason).To(Equal(vdscondition.Snapshotting.String()))
			Expect(readyCondition().Message).To(Equal("Waiting for the snapshot to be ready to use."))
		})

		It("releases the access once the volume snapshot is ready and waits for the verdict", func() {
			vdSnapshot.Status.Phase = v1alpha2.VirtualDiskSnapshotPhaseInProgress

			withClaim(coordinationv1alpha1.AccessClaimPhaseGranted, vmaccondition.Frozen, "")
			readyVolumeSnapshot()

			h := newHandler(true)

			_, err := h.Handle(testContext(), vdSnapshot)
			Expect(err).To(BeNil())
			Expect(getClaim().Spec.Release).To(BeTrue())
			Expect(vdSnapshot.Status.Phase).To(Equal(v1alpha2.VirtualDiskSnapshotPhaseInProgress))
			Expect(vdSnapshot.Status.Consistent).To(BeNil())
		})

		DescribeTable("turns ready with the consistency of the verdict",
			func(frozen bool, instanceUID types.UID, consistent bool) {
				vdSnapshot.Status.Phase = v1alpha2.VirtualDiskSnapshotPhaseInProgress
				claim := withClaim(coordinationv1alpha1.AccessClaimPhaseCompleted, vmaccondition.Released, "")
				claim.Spec.Release = true
				claim.Status.Frozen = frozen
				claim.Status.VirtualMachineInstanceUID = instanceUID

				readyVolumeSnapshot()

				h := newHandler(true)

				_, err := h.Handle(testContext(), vdSnapshot)
				Expect(err).To(BeNil())
				Expect(vdSnapshot.Status.Phase).To(Equal(v1alpha2.VirtualDiskSnapshotPhaseReady))
				Expect(vdSnapshot.Status.Consistent).To(Equal(ptr.To(consistent)))
			},
			Entry("a frozen guest", true, types.UID("vmi-uid"), true),
			Entry("a stopped virtual machine", false, types.UID(""), true),
			Entry("a guest that was not frozen", false, types.UID("vmi-uid"), false),
		)

		DescribeTable("waits for the verdict only, whatever has happened to the disk since the capture",
			func(changeDisk func()) {
				vdSnapshot.Status.Phase = v1alpha2.VirtualDiskSnapshotPhaseInProgress
				claim := withClaim(coordinationv1alpha1.AccessClaimPhaseCompleted, vmaccondition.Released, "")
				claim.Spec.Release = true
				claim.Status.Frozen = true
				claim.Status.VirtualMachineInstanceUID = "vmi-uid"

				readyVolumeSnapshot()
				changeDisk()

				h := newHandler(true)

				_, err := h.Handle(testContext(), vdSnapshot)
				Expect(err).To(BeNil())
				Expect(vdSnapshot.Status.Phase).To(Equal(v1alpha2.VirtualDiskSnapshotPhaseReady))
				Expect(vdSnapshot.Status.Consistent).To(Equal(ptr.To(true)))
			},
			Entry("a deleted disk", func() {
				snapshotter.GetVirtualDiskFunc = func(_ context.Context, _, _ string) (*v1alpha2.VirtualDisk, error) {
					return nil, nil
				}
			}),
			Entry("a disk that is not ready", func() {
				vdSnapshot.Status.Conditions[0].Status = metav1.ConditionFalse
			}),
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
				vdSnapshot.Status.Phase = v1alpha2.VirtualDiskSnapshotPhaseInProgress

				readyVolumeSnapshot()

				snapshotter.UpdateVolumeSnapshotFunc = func(_ context.Context, _ *vsv1.VolumeSnapshot) error { return nil }
			})

			It("waits for the unfreeze before turning ready", func() {
				unfrozenClaim(vmaccondition.Unfreezing, "")

				h := newHandler(true)

				_, err := h.Handle(testContext(), vdSnapshot)
				Expect(err).To(BeNil())
				Expect(vdSnapshot.Status.Phase).To(Equal(v1alpha2.VirtualDiskSnapshotPhaseInProgress))
				Expect(readyCondition().Reason).To(Equal(vdscondition.FileSystemUnfreezing.String()))
			})

			It("turns ready and warns when the guest does not confirm the unfreeze", func() {
				unfrozenClaim(vmaccondition.UnfreezeNotConfirmed, "The guest did not confirm the unfreeze.")

				h := newHandler(true)

				_, err := h.Handle(testContext(), vdSnapshot)
				Expect(err).To(BeNil())
				Expect(vdSnapshot.Status.Phase).To(Equal(v1alpha2.VirtualDiskSnapshotPhaseReady))
				Expect(vdSnapshot.Status.Consistent).To(Equal(ptr.To(true)))
				Expect(readyCondition().Message).To(Equal("The guest did not confirm the unfreeze."))
				Expect(warnings).To(Equal([]string{v1alpha2.ReasonFilesystemUnfreezeNotConfirmed}))

				_, err = h.Handle(testContext(), vdSnapshot)
				Expect(err).To(BeNil())
				Expect(readyCondition().Message).To(Equal("The guest did not confirm the unfreeze."))
				Expect(warnings).To(HaveLen(1))
			})
		})

		It("releases the access of a failed snapshot", func() {
			vdSnapshot.Status.Phase = v1alpha2.VirtualDiskSnapshotPhaseFailed

			withClaim(coordinationv1alpha1.AccessClaimPhaseGranted, vmaccondition.Frozen, "")

			h := newHandler(true)

			_, err := h.Handle(testContext(), vdSnapshot)
			Expect(err).To(BeNil())
			Expect(getClaim().Spec.Release).To(BeTrue())
		})

		It("releases the access in the pass after the one that fails a snapshot under a granted access", func() {
			vdSnapshot.Status.Phase = v1alpha2.VirtualDiskSnapshotPhaseInProgress

			withClaim(coordinationv1alpha1.AccessClaimPhaseGranted, vmaccondition.Frozen, "")

			snapshotter.GetVirtualDiskFunc = func(_ context.Context, _, _ string) (*v1alpha2.VirtualDisk, error) {
				return nil, nil
			}
			h := newHandler(true)

			res, err := h.Handle(testContext(), vdSnapshot)
			Expect(err).To(BeNil())
			Expect(vdSnapshot.Status.Phase).To(Equal(v1alpha2.VirtualDiskSnapshotPhaseFailed))
			Expect(getClaim().Spec.Release).To(BeFalse())
			Expect(res.RequeueAfter).To(BeNumerically(">", 0))

			_, err = h.Handle(testContext(), vdSnapshot)
			Expect(err).To(BeNil())
			Expect(getClaim().Spec.Release).To(BeTrue())
		})

		It("fails instead of taking the volume snapshot once the access is released", func() {
			vdSnapshot.Status.Phase = v1alpha2.VirtualDiskSnapshotPhaseInProgress
			claim := withClaim(coordinationv1alpha1.AccessClaimPhaseCompleted, vmaccondition.Released, "")
			claim.Spec.Release = true
			claim.Status.Frozen = true
			claim.Status.VirtualMachineInstanceUID = "vmi-uid"
			h := newHandler(true)

			_, err := h.Handle(testContext(), vdSnapshot)
			Expect(err).To(BeNil())
			Expect(vdSnapshot.Status.Phase).To(Equal(v1alpha2.VirtualDiskSnapshotPhaseFailed))
			Expect(vdSnapshot.Status.Consistent).To(Equal(ptr.To(false)))
			Expect(snapshotter.CreateVolumeSnapshotCalls()).To(BeEmpty())
		})

		It("keeps a ready snapshot as it is when its claim cannot be read", func() {
			vdSnapshot.Status.Phase = v1alpha2.VirtualDiskSnapshotPhaseReady
			meta.SetStatusCondition(&vdSnapshot.Status.Conditions, metav1.Condition{
				Type:   vdscondition.VirtualDiskSnapshotReadyType.String(),
				Status: metav1.ConditionTrue,
				Reason: vdscondition.VirtualDiskSnapshotReady.String(),
			})
			readyVolumeSnapshot()

			h := newHandler(true)
			h.client = failingClaimClient{Client: fakeClient, failGet: true}
			h.apiReader = h.client

			_, err := h.Handle(testContext(), vdSnapshot)
			Expect(err).To(HaveOccurred())
			Expect(vdSnapshot.Status.Phase).To(Equal(v1alpha2.VirtualDiskSnapshotPhaseReady))
			Expect(readyCondition().Status).To(Equal(metav1.ConditionTrue))
			Expect(readyCondition().Reason).To(Equal(vdscondition.VirtualDiskSnapshotReady.String()))
		})

		It("reads the claim of a ready snapshot from the cache alone", func() {
			vdSnapshot.Status.Phase = v1alpha2.VirtualDiskSnapshotPhaseReady
			meta.SetStatusCondition(&vdSnapshot.Status.Conditions, metav1.Condition{
				Type:   vdscondition.VirtualDiskSnapshotReadyType.String(),
				Status: metav1.ConditionTrue,
				Reason: vdscondition.VirtualDiskSnapshotReady.String(),
			})
			readyVolumeSnapshot()

			h := newHandler(true)
			h.apiReader = failingClaimClient{Client: fakeClient, failGet: true}

			_, err := h.Handle(testContext(), vdSnapshot)
			Expect(err).To(BeNil())
			Expect(vdSnapshot.Status.Phase).To(Equal(v1alpha2.VirtualDiskSnapshotPhaseReady))
		})

		It("confirms an absent claim past the cache for a snapshot in progress", func() {
			vdSnapshot.Status.Phase = v1alpha2.VirtualDiskSnapshotPhaseInProgress
			h := newHandler(true)
			h.apiReader = failingClaimClient{Client: fakeClient, failGet: true}

			_, err := h.Handle(testContext(), vdSnapshot)
			Expect(err).To(HaveOccurred())
			Expect(vdSnapshot.Status.Phase).To(Equal(v1alpha2.VirtualDiskSnapshotPhaseInProgress))
			Expect(unfreezer.calls).To(BeZero())
		})

		It("stops the handler chain while the access of a deleted snapshot cannot be ended", func() {
			vdSnapshot.Status.Phase = v1alpha2.VirtualDiskSnapshotPhaseInProgress
			vdSnapshot.DeletionTimestamp = ptr.To(metav1.Now())

			withClaim(coordinationv1alpha1.AccessClaimPhaseGranted, vmaccondition.Frozen, "")

			h := newHandler(true)
			h.client = failingClaimClient{Client: fakeClient, failPatch: true}

			res, err := h.Handle(testContext(), vdSnapshot)
			Expect(errors.Is(err, reconciler.ErrStopHandlerChain)).To(BeTrue())
			Expect(res.RequeueAfter).To(BeNumerically(">", 0))
			Expect(getClaim().Spec.Release).To(BeFalse())
		})

		It("releases the access of a deleted snapshot", func() {
			vdSnapshot.Status.Phase = v1alpha2.VirtualDiskSnapshotPhaseInProgress
			vdSnapshot.DeletionTimestamp = ptr.To(metav1.Now())

			withClaim(coordinationv1alpha1.AccessClaimPhaseGranted, vmaccondition.Frozen, "")

			h := newHandler(true)

			_, err := h.Handle(testContext(), vdSnapshot)
			Expect(err).To(BeNil())
			Expect(getClaim().Spec.Release).To(BeTrue())
			Expect(vdSnapshot.Status.Phase).To(Equal(v1alpha2.VirtualDiskSnapshotPhaseTerminating))
			Expect(unfreezer.calls).To(BeZero())
		})

		Context("a snapshot started before the module update", func() {
			BeforeEach(func() {
				vdSnapshot.Status.Phase = v1alpha2.VirtualDiskSnapshotPhaseInProgress
				kvvmi.Annotations = map[string]string{annotations.AnnVMFilesystemRequest: service.RequestFSFreeze}
			})

			It("fails and unfreezes the guest it may have left frozen", func() {
				h := newHandler(true)

				_, err := h.Handle(testContext(), vdSnapshot)
				Expect(err).To(BeNil())
				Expect(vdSnapshot.Status.Phase).To(Equal(v1alpha2.VirtualDiskSnapshotPhaseFailed))
				Expect(readyCondition().Reason).To(Equal(vdscondition.AbortedByModuleUpdate.String()))
				Expect(unfreezer.calls).To(Equal(1))

				instance := &virtv1.VirtualMachineInstance{}
				Expect(fakeClient.Get(testContext(), client.ObjectKeyFromObject(kvvmi), instance)).To(Succeed())
				// The coordinator removes the filesystem request the snapshot left.
				Expect(instance.Annotations).To(HaveKey(annotations.AnnVMFilesystemRequest))
			})

			It("leaves a guest held by a coordinator session alone", func() {
				Expect(vmaccess.WriteLock(kvvm, &vmaccess.Lock{ClaimUID: "other"})).To(Succeed())

				h := newHandler(true)

				_, err := h.Handle(testContext(), vdSnapshot)
				Expect(err).To(BeNil())
				Expect(vdSnapshot.Status.Phase).To(Equal(v1alpha2.VirtualDiskSnapshotPhaseFailed))
				Expect(unfreezer.calls).To(BeZero())
			})

			DescribeTable("unfreezes the guest when deleted",
				func(phase v1alpha2.VirtualDiskSnapshotPhase) {
					vdSnapshot.Status.Phase = phase
					vdSnapshot.DeletionTimestamp = ptr.To(metav1.Now())
					h := newHandler(true)

					_, err := h.Handle(testContext(), vdSnapshot)
					Expect(err).To(BeNil())
					Expect(unfreezer.calls).To(Equal(1))
				},
				Entry("in progress", v1alpha2.VirtualDiskSnapshotPhaseInProgress),
				Entry("failed", v1alpha2.VirtualDiskSnapshotPhaseFailed),
			)
		})

		Context("a snapshot started on the disk before it was attached", func() {
			BeforeEach(func() {
				vdSnapshot.Status.Phase = v1alpha2.VirtualDiskSnapshotPhaseInProgress
				vdSnapshot.Annotations = map[string]string{annotations.AnnStartedOnUnattachedDisk: "true"}
			})

			It("fails as a change of the virtual machine and leaves the guest alone", func() {
				h := newHandler(true)

				_, err := h.Handle(testContext(), vdSnapshot)
				Expect(err).To(BeNil())
				Expect(vdSnapshot.Status.Phase).To(Equal(v1alpha2.VirtualDiskSnapshotPhaseFailed))
				Expect(readyCondition().Reason).To(Equal(vdscondition.VirtualMachineStateChanged.String()))
				Expect(readyCondition().Message).To(ContainSubstring("was attached"))
				Expect(unfreezer.calls).To(BeZero())
			})

			It("leaves the guest alone when deleted", func() {
				vdSnapshot.DeletionTimestamp = ptr.To(metav1.Now())
				h := newHandler(true)

				_, err := h.Handle(testContext(), vdSnapshot)
				Expect(err).To(BeNil())
				Expect(unfreezer.calls).To(BeZero())
			})
		})
	})

	Context("The virtual disk snapshot of a virtual machine snapshot", func() {
		var vmSnapshot *v1alpha2.VirtualMachineSnapshot

		parentClaim := func(phase coordinationv1alpha1.AccessClaimPhase, frozen bool) {
			objs = append(objs, &coordinationv1alpha1.VirtualMachineAccessClaim{
				ObjectMeta: metav1.ObjectMeta{Name: accessclaim.Name(vmSnapshot), Namespace: testNamespace},
				Spec: coordinationv1alpha1.VirtualMachineAccessClaimSpec{
					VirtualMachine: coordinationv1alpha1.VirtualMachineAccessClaimTarget{Name: "vm"},
					Access:         []coordinationv1alpha1.AccessMode{coordinationv1alpha1.AccessModeSnapshot, coordinationv1alpha1.AccessModeFreeze},
					FreezePolicy:   ptr.To(coordinationv1alpha1.FreezePolicyRequired),
				},
				Status: coordinationv1alpha1.VirtualMachineAccessClaimStatus{Phase: phase, Frozen: frozen, VirtualMachineInstanceUID: "vmi-uid"},
			})
		}

		BeforeEach(func() {
			vmSnapshot = &v1alpha2.VirtualMachineSnapshot{
				ObjectMeta: metav1.ObjectMeta{Name: "vmsnapshot", Namespace: testNamespace, UID: "vms-uid"},
				Status:     v1alpha2.VirtualMachineSnapshotStatus{Phase: v1alpha2.VirtualMachineSnapshotPhaseInProgress},
			}
			objs = append(objs, vmSnapshot)
			vdSnapshot.OwnerReferences = []metav1.OwnerReference{service.MakeOwnerReference(vmSnapshot)}
			vdSnapshot.OwnerReferences[0].Kind = v1alpha2.VirtualMachineSnapshotKind
			vdSnapshot.OwnerReferences[0].APIVersion = v1alpha2.SchemeGroupVersion.String()
			vdSnapshot.OwnerReferences[0].UID = vmSnapshot.UID
			vd.Status.AttachedToVirtualMachines = []v1alpha2.AttachedVirtualMachine{{Name: "vm"}}
		})

		It("takes the disk under the access of its parent without a claim of its own", func() {
			parentClaim(coordinationv1alpha1.AccessClaimPhaseGranted, true)

			h := newHandler(true)

			_, err := h.Handle(testContext(), vdSnapshot)
			Expect(err).To(BeNil())
			Expect(vdSnapshot.Status.Phase).To(Equal(v1alpha2.VirtualDiskSnapshotPhaseInProgress))

			claims := &coordinationv1alpha1.VirtualMachineAccessClaimList{}
			Expect(fakeClient.List(testContext(), claims)).To(Succeed())
			Expect(claims.Items).To(HaveLen(1))
			Expect(snapshotter.GetVirtualMachineCalls()).To(BeEmpty())
		})

		DescribeTable("takes its consistency from its parent once the parent is terminal",
			func(phase v1alpha2.VirtualMachineSnapshotPhase, parentConsistent, consistent *bool) {
				vdSnapshot.Status.Phase = v1alpha2.VirtualDiskSnapshotPhaseReady
				vmSnapshot.Status.Phase = phase
				vmSnapshot.Status.Consistent = parentConsistent
				vs.Status = &vsv1.VolumeSnapshotStatus{ReadyToUse: ptr.To(true)}
				snapshotter.GetVolumeSnapshotFunc = func(_ context.Context, _, _ string) (*vsv1.VolumeSnapshot, error) {
					return vs, nil
				}
				snapshotter.UpdateVolumeSnapshotFunc = func(_ context.Context, _ *vsv1.VolumeSnapshot) error {
					return nil
				}
				h := newHandler(true)

				_, err := h.Handle(testContext(), vdSnapshot)
				Expect(err).To(BeNil())
				Expect(vdSnapshot.Status.Phase).To(Equal(v1alpha2.VirtualDiskSnapshotPhaseReady))
				Expect(vdSnapshot.Status.Consistent).To(Equal(consistent))
			},
			Entry("a parent in progress", v1alpha2.VirtualMachineSnapshotPhaseInProgress, nil, nil),
			Entry("a consistent ready parent", v1alpha2.VirtualMachineSnapshotPhaseReady, ptr.To(true), ptr.To(true)),
			Entry("an inconsistent ready parent", v1alpha2.VirtualMachineSnapshotPhaseReady, ptr.To(false), ptr.To(false)),
			Entry("a failed parent", v1alpha2.VirtualMachineSnapshotPhaseFailed, ptr.To(false), ptr.To(false)),
		)

		Context("deleted while in progress", func() {
			BeforeEach(func() {
				vdSnapshot.Status.Phase = v1alpha2.VirtualDiskSnapshotPhaseInProgress
				vdSnapshot.DeletionTimestamp = ptr.To(metav1.Now())
				kvvmi := &virtv1.VirtualMachineInstance{
					ObjectMeta: metav1.ObjectMeta{Name: "vm", Namespace: testNamespace},
					Status:     virtv1.VirtualMachineInstanceStatus{Phase: virtv1.Running},
				}
				objs = append(objs, &virtv1.VirtualMachine{ObjectMeta: metav1.ObjectMeta{Name: "vm", Namespace: testNamespace}}, kvvmi)
				snapshotter.GetVirtualMachineFunc = func(_ context.Context, name, namespace string) (*v1alpha2.VirtualMachine, error) {
					return &v1alpha2.VirtualMachine{ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: namespace}}, nil
				}
				snapshotter.GetVirtualMachineInstanceFunc = func(_ context.Context, _ *v1alpha2.VirtualMachine) (*virtv1.VirtualMachineInstance, error) {
					return kvvmi, nil
				}
			})

			It("leaves the guest to the claim of its parent", func() {
				parentClaim(coordinationv1alpha1.AccessClaimPhaseGranted, true)

				h := newHandler(true)

				_, err := h.Handle(testContext(), vdSnapshot)
				Expect(err).To(BeNil())
				Expect(unfreezer.calls).To(BeZero())
			})

			DescribeTable("unfreezes the guest when its parent was started before the module update",
				func(phase v1alpha2.VirtualDiskSnapshotPhase) {
					vdSnapshot.Status.Phase = phase
					h := newHandler(true)

					_, err := h.Handle(testContext(), vdSnapshot)
					Expect(err).To(BeNil())
					Expect(unfreezer.calls).To(Equal(1))
				},
				Entry("in progress", v1alpha2.VirtualDiskSnapshotPhaseInProgress),
				Entry("failed", v1alpha2.VirtualDiskSnapshotPhaseFailed),
			)
		})

		It("fails when its parent was started before the module update", func() {
			vdSnapshot.Status.Phase = v1alpha2.VirtualDiskSnapshotPhaseInProgress
			h := newHandler(true)

			_, err := h.Handle(testContext(), vdSnapshot)
			Expect(err).To(BeNil())
			Expect(vdSnapshot.Status.Phase).To(Equal(v1alpha2.VirtualDiskSnapshotPhaseFailed))
			ready, _ := conditions.GetCondition(vdscondition.VirtualDiskSnapshotReadyType, vdSnapshot.Status.Conditions)
			Expect(ready.Reason).To(Equal(vdscondition.AbortedByModuleUpdate.String()))
			Expect(vdSnapshot.Status.Consistent).To(HaveValue(BeFalse()))
		})

		It("is inconsistent when its parent was replaced under the same name", func() {
			vdSnapshot.Status.Phase = v1alpha2.VirtualDiskSnapshotPhaseReady
			vdSnapshot.OwnerReferences[0].UID = "replaced-vms-uid"
			vs.Status = &vsv1.VolumeSnapshotStatus{ReadyToUse: ptr.To(true)}
			snapshotter.GetVolumeSnapshotFunc = func(_ context.Context, _, _ string) (*vsv1.VolumeSnapshot, error) {
				return vs, nil
			}
			snapshotter.UpdateVolumeSnapshotFunc = func(_ context.Context, _ *vsv1.VolumeSnapshot) error {
				return nil
			}
			h := newHandler(true)

			_, err := h.Handle(testContext(), vdSnapshot)
			Expect(err).To(BeNil())
			Expect(vdSnapshot.Status.Consistent).To(Equal(ptr.To(false)))
		})
	})

	// A snapshot taken before the controller recorded the source size keeps failing to restore after an
	// upgrade, because the annotation is only ever written when the VolumeSnapshot is created. Reaching
	// those snapshots is the whole point of doing this on reconciliation of a Ready one.
	Context("Backfilling the original size of an older snapshot", func() {
		var updated *vsv1.VolumeSnapshot

		reconcileReady := func() error {
			snapshotter.GetVolumeSnapshotFunc = func(_ context.Context, _, _ string) (*vsv1.VolumeSnapshot, error) {
				vs.Status = &vsv1.VolumeSnapshotStatus{ReadyToUse: ptr.To(true)}
				return vs, nil
			}
			snapshotter.UpdateVolumeSnapshotFunc = func(_ context.Context, vs *vsv1.VolumeSnapshot) error {
				updated = vs
				return nil
			}

			vdSnapshot.Status.Phase = v1alpha2.VirtualDiskSnapshotPhaseReady

			_, err := newHandler(true).Handle(testContext(), vdSnapshot)

			return err
		}

		BeforeEach(func() {
			updated = nil
			vs.Spec.Source.PersistentVolumeClaimName = ptr.To(pvc.Name)
			pvc.Spec.Resources.Requests = corev1.ResourceList{
				corev1.ResourceStorage: resource.MustParse("100Mi"),
			}
		})

		It("stamps the size of the claim the snapshot was taken from", func() {
			Expect(reconcileReady()).To(Succeed())

			Expect(updated).NotTo(BeNil())
			Expect(updated.Annotations[annotations.AnnVirtualDiskOriginalSize]).To(Equal("100Mi"))
			Expect(vdSnapshot.Status.Phase).To(Equal(v1alpha2.VirtualDiskSnapshotPhaseReady))
		})

		It("leaves a size the snapshot already records alone", func() {
			vs.Annotations = map[string]string{annotations.AnnVirtualDiskOriginalSize: "42Mi"}

			Expect(reconcileReady()).To(Succeed())

			Expect(updated).To(BeNil())
		})

		// Nothing left to read the size off: the restore falls back to the mountable minimum instead.
		It("does nothing when the source claim is gone", func() {
			snapshotter.GetPersistentVolumeClaimFunc = func(_ context.Context, _, _ string) (*corev1.PersistentVolumeClaim, error) {
				return nil, nil
			}

			Expect(reconcileReady()).To(Succeed())

			Expect(updated).To(BeNil())
		})

		It("does nothing for a snapshot bound to a pre-provisioned content", func() {
			vs.Spec.Source.PersistentVolumeClaimName = nil
			vs.Spec.Source.VolumeSnapshotContentName = ptr.To("content")

			Expect(reconcileReady()).To(Succeed())

			Expect(updated).To(BeNil())
		})
	})
})
