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

package vmaccess

import (
	"context"
	"errors"
	"testing"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/ptr"
	virtv1 "kubevirt.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/deckhouse/virtualization-controller/pkg/common/annotations"
	"github.com/deckhouse/virtualization-controller/pkg/common/testutil"
	"github.com/deckhouse/virtualization-controller/pkg/common/vmaccess"
	"github.com/deckhouse/virtualization-controller/pkg/controller/vmaccess/internal"
	"github.com/deckhouse/virtualization-controller/pkg/controller/vmaccess/internal/state"
	"github.com/deckhouse/virtualization-controller/pkg/eventrecord"
	coordinationv1alpha1 "github.com/deckhouse/virtualization/api/coordination/v1alpha1"
	"github.com/deckhouse/virtualization/api/coordination/v1alpha1/vmaccondition"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
)

func TestVirtualMachineAccessCoordinator(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "VirtualMachineAccessCoordinator")
}

const (
	namespace = "ns"
	vmName    = "vm"
	vmUID     = types.UID("vm-uid")
	vmiUID    = types.UID("vmi-uid")
	timeout   = 10 * time.Minute
)

var key = types.NamespacedName{Namespace: namespace, Name: vmName}

type event struct {
	object client.Object
	reason string
}

type freezeCall struct {
	claimUID        types.UID
	unfreezeTimeout time.Duration
}

var _ = Describe("Reconciler", func() {
	var (
		ctx         context.Context
		now         time.Time
		freezes     []freezeCall
		unfreezes   []types.UID
		freezeErr   error
		unfreezeErr error
		events      []event
		// statusLags keeps fsFreezeStatus as it is after an unfreeze that went through, as a virt-handler that
		// has not reported the unfreeze yet does.
		statusLags bool
		// unfreezeReported is set when the fake reports the unfreeze, whose event triggers the next pass.
		unfreezeReported bool
	)

	clock := func() time.Time { return now }
	advance := func(d time.Duration) { now = now.Add(d) }

	newVM := func() *v1alpha2.VirtualMachine {
		return &v1alpha2.VirtualMachine{ObjectMeta: metav1.ObjectMeta{Name: vmName, Namespace: namespace, UID: vmUID}}
	}
	newKVVM := func() *virtv1.VirtualMachine {
		return &virtv1.VirtualMachine{ObjectMeta: metav1.ObjectMeta{Name: vmName, Namespace: namespace}}
	}
	newKVVMI := func(agent bool) *virtv1.VirtualMachineInstance {
		agentStatus := corev1.ConditionFalse
		if agent {
			agentStatus = corev1.ConditionTrue
		}

		return &virtv1.VirtualMachineInstance{
			ObjectMeta: metav1.ObjectMeta{Name: vmName, Namespace: namespace, UID: vmiUID},
			Status: virtv1.VirtualMachineInstanceStatus{
				Phase: virtv1.Running,
				Conditions: []virtv1.VirtualMachineInstanceCondition{{
					Type:   virtv1.VirtualMachineInstanceAgentConnected,
					Status: agentStatus,
				}},
			},
		}
	}
	newClaim := func(name string, uid types.UID, created time.Time, access ...coordinationv1alpha1.AccessMode) *coordinationv1alpha1.VirtualMachineAccessClaim {
		claim := &coordinationv1alpha1.VirtualMachineAccessClaim{
			ObjectMeta: metav1.ObjectMeta{
				Name:              name,
				Namespace:         namespace,
				UID:               uid,
				CreationTimestamp: metav1.NewTime(created),
				OwnerReferences: []metav1.OwnerReference{{
					APIVersion: v1alpha2.SchemeGroupVersion.String(),
					Kind:       v1alpha2.VirtualMachineSnapshotKind,
					Name:       "snapshot-" + name,
					UID:        uid + "-owner",
					Controller: ptr.To(true),
				}},
			},
			Spec: coordinationv1alpha1.VirtualMachineAccessClaimSpec{
				VirtualMachine: coordinationv1alpha1.VirtualMachineAccessClaimTarget{Name: vmName, UID: vmUID},
				Access:         access,
				Timeout:        metav1.Duration{Duration: timeout},
			},
		}
		for _, a := range access {
			if a == coordinationv1alpha1.AccessModeFreeze {
				claim.Spec.FreezePolicy = ptr.To(coordinationv1alpha1.FreezePolicyRequired)
			}
		}

		return claim
	}
	freezeClaim := func(name string, uid types.UID) *coordinationv1alpha1.VirtualMachineAccessClaim {
		return newClaim(name, uid, now, coordinationv1alpha1.AccessModeSnapshot, coordinationv1alpha1.AccessModeFreeze)
	}

	newReconciler := func(c client.Client) *Reconciler {
		freezer := &internal.FreezerMock{
			FreezeFunc: func(_ context.Context, _, _ string, claimUID types.UID, unfreezeTimeout time.Duration) error {
				freezes = append(freezes, freezeCall{claimUID: claimUID, unfreezeTimeout: unfreezeTimeout})
				return freezeErr
			},
			UnfreezeFunc: func(ctx context.Context, namespace, name string, claimUID types.UID) error {
				unfreezes = append(unfreezes, claimUID)

				if unfreezeErr != nil || statusLags {
					return unfreezeErr
				}

				kvvmi := &virtv1.VirtualMachineInstance{}
				if err := c.Get(ctx, types.NamespacedName{Namespace: namespace, Name: name}, kvvmi); err != nil {
					return client.IgnoreNotFound(err)
				}

				kvvmi.Status.FSFreezeStatus = ""
				unfreezeReported = true

				return c.Status().Update(ctx, kvvmi)
			},
		}

		return NewReconciler(c,
			internal.NewProtectionHandler(),
			internal.NewLifecycleHandler(freezer, &eventrecord.EventRecorderLoggerMock{
				EventFunc: func(object client.Object, _, reason, _ string) {
					events = append(events, event{object: object, reason: reason})
				},
			}, clock),
			internal.NewCleanupHandler(),
		)
	}

	// reconcileVM runs the pass that writes the final status of a claim and the pass that then releases its
	// lock, as the event of that write would, and the pass the event of a reported unfreeze triggers.
	reconcileVM := func(r *Reconciler) reconcile.Result {
		GinkgoHelper()

		for range 5 {
			res, err := r.Reconcile(ctx, reconcile.Request{NamespacedName: key})
			Expect(err).NotTo(HaveOccurred())

			if res.RequeueAfter != internal.StatusWriteRecheck && !unfreezeReported {
				return res
			}

			unfreezeReported = false
		}

		Fail("the lock keeps waiting for the status of its claim to be written")

		return reconcile.Result{}
	}
	getClaim := func(c client.Client, name string) *coordinationv1alpha1.VirtualMachineAccessClaim {
		GinkgoHelper()

		claim := &coordinationv1alpha1.VirtualMachineAccessClaim{}
		Expect(c.Get(ctx, types.NamespacedName{Namespace: namespace, Name: name}, claim)).To(Succeed())

		return claim
	}
	getLock := func(c client.Client) *vmaccess.Lock {
		GinkgoHelper()

		kvvm := &virtv1.VirtualMachine{}
		Expect(c.Get(ctx, key, kvvm)).To(Succeed())
		lock, err := vmaccess.ReadLock(kvvm)
		Expect(err).NotTo(HaveOccurred())

		return lock
	}
	expectGranted := func(claim *coordinationv1alpha1.VirtualMachineAccessClaim, status metav1.ConditionStatus, reason vmaccondition.GrantedReason) {
		GinkgoHelper()

		cond := meta.FindStatusCondition(claim.Status.Conditions, vmaccondition.GrantedType.String())
		Expect(cond).NotTo(BeNil())
		Expect(cond.Status).To(Equal(status))
		Expect(cond.Reason).To(Equal(reason.String()))
	}
	updateKVVMI := func(c client.Client, mutate func(*virtv1.VirtualMachineInstance)) {
		GinkgoHelper()

		kvvmi := &virtv1.VirtualMachineInstance{}
		Expect(c.Get(ctx, key, kvvmi)).To(Succeed())
		mutate(kvvmi)
		Expect(c.Status().Update(ctx, kvvmi)).To(Succeed())
	}
	release := func(c client.Client, name string) {
		GinkgoHelper()

		claim := getClaim(c, name)
		claim.Spec.Release = true
		Expect(c.Update(ctx, claim)).To(Succeed())
	}

	BeforeEach(func() {
		ctx = testutil.ContextBackgroundWithNoOpLogger()
		now = time.Date(2026, 10, 22, 10, 0, 0, 0, time.UTC)
		freezes = nil
		unfreezes = nil
		freezeErr = nil
		unfreezeErr = nil
		events = nil
		statusLags = false
		unfreezeReported = false
	})

	// grantFrozen walks a freeze claim on a running machine up to Granted/Frozen, with the guest reporting
	// the freeze.
	grantFrozen := func(c client.Client, r *Reconciler, name string) {
		GinkgoHelper()
		reconcileVM(r)
		expectGranted(getClaim(c, name), metav1.ConditionTrue, vmaccondition.Frozen)
		updateKVVMI(c, func(vmi *virtv1.VirtualMachineInstance) { vmi.Status.FSFreezeStatus = "frozen" })
	}

	It("freezes, grants, releases and unfreezes a running virtual machine", func() {
		claim := freezeClaim("a", "a-uid")
		c, err := testutil.NewFakeClientWithObjects(newVM(), newKVVM(), newKVVMI(true), claim)
		Expect(err).NotTo(HaveOccurred())

		r := newReconciler(c)

		reconcileVM(r)

		got := getClaim(c, "a")
		Expect(got.Status.Phase).To(Equal(coordinationv1alpha1.AccessClaimPhaseGranted), "a freeze request that succeeds is the confirmation")
		Expect(got.Status.Frozen).To(BeTrue())
		Expect(got.Status.Deadline.Time).To(BeTemporally("==", now.Add(timeout)))
		Expect(got.Status.VirtualMachineInstanceUID).To(Equal(vmiUID))
		Expect(got.Finalizers).To(ContainElement(v1alpha2.FinalizerVMACCleanup))
		expectGranted(got, metav1.ConditionTrue, vmaccondition.Frozen)
		Expect(getLock(c)).To(Equal(&vmaccess.Lock{ClaimUID: "a-uid", VMIUID: vmiUID}))
		Expect(freezes).To(Equal([]freezeCall{{claimUID: "a-uid", unfreezeTimeout: internal.FreezeConfirmTimeout + timeout + internal.SafetyUnfreezeMargin}}))

		release(c, "a")
		reconcileVM(r)

		got = getClaim(c, "a")
		Expect(got.Status.Phase).To(Equal(coordinationv1alpha1.AccessClaimPhaseCompleted))
		expectGranted(got, metav1.ConditionFalse, vmaccondition.Released)
		unfrozen := meta.FindStatusCondition(got.Status.Conditions, vmaccondition.UnfrozenType.String())
		Expect(unfrozen.Status).To(Equal(metav1.ConditionTrue))
		Expect(unfrozen.Reason).To(Equal(vmaccondition.UnfreezeConfirmed.String()))
		Expect(unfreezes).To(Equal([]types.UID{"a-uid"}))
		Expect(getLock(c)).To(BeNil())
		Expect(got.Finalizers).NotTo(ContainElement(v1alpha2.FinalizerVMACCleanup))
	})

	// fsFreezeStatus can read empty while the guest is frozen.
	It("unfreezes the guest at the end of its session even when fsFreezeStatus reads empty", func() {
		c, err := testutil.NewFakeClientWithObjects(newVM(), newKVVM(), newKVVMI(true), freezeClaim("a", "a-uid"))
		Expect(err).NotTo(HaveOccurred())

		r := newReconciler(c)

		reconcileVM(r)
		expectGranted(getClaim(c, "a"), metav1.ConditionTrue, vmaccondition.Frozen)

		release(c, "a")
		reconcileVM(r)

		Expect(unfreezes).To(Equal([]types.UID{"a-uid"}))
		Expect(getLock(c)).To(BeNil())
	})

	It("holds the lock until the unfreeze goes through", func() {
		c, err := testutil.NewFakeClientWithObjects(newVM(), newKVVM(), newKVVMI(true), freezeClaim("a", "a-uid"))
		Expect(err).NotTo(HaveOccurred())

		r := newReconciler(c)
		grantFrozen(c, r, "a")

		unfreezeErr = errors.New("the guest agent did not answer")

		release(c, "a")
		reconcileVM(r)
		Expect(meta.FindStatusCondition(getClaim(c, "a").Status.Conditions, vmaccondition.UnfrozenType.String()).Reason).To(Equal(vmaccondition.Unfreezing.String()))
		Expect(getLock(c)).NotTo(BeNil())

		unfreezeErr = nil

		advance(internal.RequestResendInterval)
		reconcileVM(r)
		Expect(meta.FindStatusCondition(getClaim(c, "a").Status.Conditions, vmaccondition.UnfrozenType.String()).Reason).To(Equal(vmaccondition.UnfreezeConfirmed.String()))
		Expect(getLock(c)).To(BeNil())
		Expect(unfreezes).To(HaveLen(2))
	})

	It("holds the lock until fsFreezeStatus stops reading frozen after the unfreeze", func() {
		c, err := testutil.NewFakeClientWithObjects(newVM(), newKVVM(), newKVVMI(true), freezeClaim("a", "a-uid"))
		Expect(err).NotTo(HaveOccurred())

		r := newReconciler(c)
		grantFrozen(c, r, "a")

		statusLags = true

		release(c, "a")
		reconcileVM(r)
		advance(internal.PollInterval)
		reconcileVM(r)
		Expect(meta.FindStatusCondition(getClaim(c, "a").Status.Conditions, vmaccondition.UnfrozenType.String()).Reason).To(Equal(vmaccondition.Unfreezing.String()))
		Expect(getLock(c)).NotTo(BeNil())
		Expect(unfreezes).To(HaveLen(1), "an unfreeze that went through is not sent again")

		updateKVVMI(c, func(vmi *virtv1.VirtualMachineInstance) { vmi.Status.FSFreezeStatus = "" })
		reconcileVM(r)
		Expect(meta.FindStatusCondition(getClaim(c, "a").Status.Conditions, vmaccondition.UnfrozenType.String()).Reason).To(Equal(vmaccondition.UnfreezeConfirmed.String()))
		Expect(getLock(c)).To(BeNil())
		Expect(unfreezes).To(HaveLen(1))
	})

	It("does not confirm the unfreeze while fsFreezeStatus keeps reading frozen", func() {
		c, err := testutil.NewFakeClientWithObjects(newVM(), newKVVM(), newKVVMI(true), freezeClaim("a", "a-uid"))
		Expect(err).NotTo(HaveOccurred())

		r := newReconciler(c)
		grantFrozen(c, r, "a")

		statusLags = true

		release(c, "a")
		reconcileVM(r)
		advance(internal.UnfreezeConfirmTimeout + time.Second)
		reconcileVM(r)

		got := getClaim(c, "a")
		Expect(got.Status.Phase).To(Equal(coordinationv1alpha1.AccessClaimPhaseCompleted))
		unfrozen := meta.FindStatusCondition(got.Status.Conditions, vmaccondition.UnfrozenType.String())
		Expect(unfrozen.Status).To(Equal(metav1.ConditionFalse))
		Expect(unfrozen.Reason).To(Equal(vmaccondition.UnfreezeNotConfirmed.String()))
		Expect(getLock(c)).To(BeNil())
	})

	It("denies a second claim while the first one holds the virtual machine", func() {
		first := freezeClaim("a", "a-uid")
		second := newClaim("b", "b-uid", now.Add(time.Second), coordinationv1alpha1.AccessModeSnapshot)
		c, err := testutil.NewFakeClientWithObjects(newVM(), newKVVM(), newKVVMI(true), first, second)
		Expect(err).NotTo(HaveOccurred())

		reconcileVM(newReconciler(c))

		Expect(getLock(c).ClaimUID).To(Equal(types.UID("a-uid")))
		got := getClaim(c, "b")
		Expect(got.Status.Phase).To(Equal(coordinationv1alpha1.AccessClaimPhaseDenied))
		expectGranted(got, metav1.ConditionFalse, vmaccondition.VirtualMachineBusy)
		Expect(meta.FindStatusCondition(got.Status.Conditions, vmaccondition.GrantedType.String()).Message).To(ContainSubstring(`VirtualMachineSnapshot "snapshot-a"`))
		Expect(got.Finalizers).NotTo(ContainElement(v1alpha2.FinalizerVMACCleanup))
	})

	It("lets only one of two decisions made from the same stale read take the lock", func() {
		c, err := testutil.NewFakeClientWithObjects(newVM(), newKVVM())
		Expect(err).NotTo(HaveOccurred())

		first, second := state.New(c, key), state.New(c, key)
		Expect(first.Load(ctx)).To(Succeed())
		Expect(second.Load(ctx)).To(Succeed())

		Expect(first.AcquireLock(ctx, "a-uid", "")).To(Succeed())
		err = second.AcquireLock(ctx, "b-uid", "")
		Expect(apierrors.IsConflict(err)).To(BeTrue(), "got %v", err)
		Expect(getLock(c).ClaimUID).To(Equal(types.UID("a-uid")))
	})

	It("grants a stopped virtual machine without a freeze and aborts when it starts", func() {
		claim := freezeClaim("a", "a-uid")
		c, err := testutil.NewFakeClientWithObjects(newVM(), newKVVM(), claim)
		Expect(err).NotTo(HaveOccurred())

		r := newReconciler(c)

		reconcileVM(r)

		got := getClaim(c, "a")
		Expect(got.Status.Phase).To(Equal(coordinationv1alpha1.AccessClaimPhaseGranted))
		expectGranted(got, metav1.ConditionTrue, vmaccondition.NotRequired)
		Expect(freezes).To(BeEmpty())

		Expect(c.Create(ctx, newKVVMI(true))).To(Succeed())
		reconcileVM(r)

		got = getClaim(c, "a")
		Expect(got.Status.Phase).To(Equal(coordinationv1alpha1.AccessClaimPhaseAborted))
		expectGranted(got, metav1.ConditionFalse, vmaccondition.VirtualMachineStateChanged)
		Expect(getLock(c)).To(BeNil())
	})

	It("keeps the session of a stopped virtual machine past the timeout", func() {
		c, err := testutil.NewFakeClientWithObjects(newVM(), newKVVM(), freezeClaim("a", "a-uid"))
		Expect(err).NotTo(HaveOccurred())

		r := newReconciler(c)

		res := reconcileVM(r)
		got := getClaim(c, "a")
		Expect(got.Status.Phase).To(Equal(coordinationv1alpha1.AccessClaimPhaseGranted))
		Expect(got.Status.Deadline).To(BeNil())
		Expect(res.RequeueAfter).To(BeZero())

		advance(timeout + time.Hour)
		reconcileVM(r)
		Expect(getClaim(c, "a").Status.Phase).To(Equal(coordinationv1alpha1.AccessClaimPhaseGranted))
		Expect(getLock(c).ClaimUID).To(Equal(types.UID("a-uid")))
	})

	It("denies a claim while the filesystem is frozen without a lock and leaves the freeze alone", func() {
		kvvmi := newKVVMI(true)
		kvvmi.Status.FSFreezeStatus = "frozen"
		c, err := testutil.NewFakeClientWithObjects(newVM(), newKVVM(), kvvmi, freezeClaim("a", "a-uid"))
		Expect(err).NotTo(HaveOccurred())

		reconcileVM(newReconciler(c))

		got := getClaim(c, "a")
		Expect(got.Status.Phase).To(Equal(coordinationv1alpha1.AccessClaimPhaseDenied))
		expectGranted(got, metav1.ConditionFalse, vmaccondition.FilesystemAlreadyFrozen)
		Expect(getLock(c)).To(BeNil())
		Expect(freezes).To(BeEmpty())
		Expect(unfreezes).To(BeEmpty())
	})

	Context("a freeze left by a snapshot taken before the module update", func() {
		var kvvmi *virtv1.VirtualMachineInstance

		hasFilesystemRequest := func(c client.Client) bool {
			GinkgoHelper()

			instance := &virtv1.VirtualMachineInstance{}
			Expect(c.Get(ctx, key, instance)).To(Succeed())
			_, ok := instance.Annotations[annotations.AnnVMFilesystemRequest]

			return ok
		}

		BeforeEach(func() {
			kvvmi = newKVVMI(true)
			kvvmi.Annotations = map[string]string{annotations.AnnVMFilesystemRequest: "freeze"}
			kvvmi.Status.FSFreezeStatus = "frozen"
		})

		DescribeTable("is cleared without a claim",
			func(request string) {
				kvvmi.Annotations[annotations.AnnVMFilesystemRequest] = request
				c, err := testutil.NewFakeClientWithObjects(newVM(), newKVVM(), kvvmi)
				Expect(err).NotTo(HaveOccurred())

				reconcileVM(newReconciler(c))

				Expect(unfreezes).To(Equal([]types.UID{""}))
				Expect(hasFilesystemRequest(c)).To(BeFalse())
				Expect(events).To(BeEmpty())
			},
			Entry("a freeze request", "freeze"),
			Entry("an unfreeze request", "unfreeze"),
		)

		It("is cleared before a claim is decided", func() {
			c, err := testutil.NewFakeClientWithObjects(newVM(), newKVVM(), kvvmi, freezeClaim("a", "a-uid"))
			Expect(err).NotTo(HaveOccurred())

			reconcileVM(newReconciler(c))

			Expect(unfreezes).To(Equal([]types.UID{""}), "the unfreeze goes out before any lock is taken")
			Expect(hasFilesystemRequest(c)).To(BeFalse())
			expectGranted(getClaim(c, "a"), metav1.ConditionTrue, vmaccondition.Frozen)
			Expect(freezes).To(HaveLen(1))
		})

		It("is left alone while a claim holds the lock", func() {
			kvvm := newKVVM()
			Expect(vmaccess.WriteLock(kvvm, &vmaccess.Lock{ClaimUID: "a-uid", VMIUID: vmiUID})).To(Succeed())
			c, err := testutil.NewFakeClientWithObjects(newVM(), kvvm, kvvmi,
				newClaim("a", "a-uid", now, coordinationv1alpha1.AccessModeSnapshot))
			Expect(err).NotTo(HaveOccurred())

			reconcileVM(newReconciler(c))

			Expect(unfreezes).To(BeEmpty())
			Expect(hasFilesystemRequest(c)).To(BeTrue())
		})

		It("is cleared while a snapshot of the virtual machine is in progress", func() {
			c, err := testutil.NewFakeClientWithObjects(newVM(), newKVVM(), kvvmi,
				&v1alpha2.VirtualMachineSnapshot{
					ObjectMeta: metav1.ObjectMeta{Name: "old", Namespace: namespace},
					Spec:       v1alpha2.VirtualMachineSnapshotSpec{VirtualMachineName: vmName},
					Status:     v1alpha2.VirtualMachineSnapshotStatus{Phase: v1alpha2.VirtualMachineSnapshotPhaseInProgress},
				})
			Expect(err).NotTo(HaveOccurred())

			reconcileVM(newReconciler(c))

			Expect(unfreezes).To(Equal([]types.UID{""}))
			Expect(hasFilesystemRequest(c)).To(BeFalse())
		})

		It("only removes the request of a stopped virtual machine", func() {
			kvvmi.Status.Phase = virtv1.Succeeded
			c, err := testutil.NewFakeClientWithObjects(newVM(), newKVVM(), kvvmi)
			Expect(err).NotTo(HaveOccurred())

			reconcileVM(newReconciler(c))

			Expect(unfreezes).To(BeEmpty())
			Expect(hasFilesystemRequest(c)).To(BeFalse())
		})

		It("sends a failed unfreeze again and keeps the request until it goes through", func() {
			unfreezeErr = errors.New("the guest agent did not answer")
			c, err := testutil.NewFakeClientWithObjects(newVM(), newKVVM(), kvvmi)
			Expect(err).NotTo(HaveOccurred())

			r := newReconciler(c)

			res := reconcileVM(r)
			Expect(res.RequeueAfter).NotTo(BeZero())
			Expect(unfreezes).To(HaveLen(1))
			Expect(hasFilesystemRequest(c)).To(BeTrue())

			unfreezeErr = nil

			advance(internal.RequestResendInterval)
			reconcileVM(r)
			Expect(unfreezes).To(HaveLen(2))
			Expect(hasFilesystemRequest(c)).To(BeFalse())
			Expect(events).To(BeEmpty())
		})

		It("reports an unfreeze still failing past the timeout and removes the request", func() {
			unfreezeErr = errors.New("the guest agent did not answer")
			c, err := testutil.NewFakeClientWithObjects(newVM(), newKVVM(), kvvmi, freezeClaim("a", "a-uid"))
			Expect(err).NotTo(HaveOccurred())

			r := newReconciler(c)

			reconcileVM(r)
			Expect(hasFilesystemRequest(c)).To(BeTrue())
			Expect(events).To(BeEmpty())
			expectGranted(getClaim(c, "a"), metav1.ConditionFalse, vmaccondition.FilesystemAlreadyFrozen)

			advance(internal.UnfreezeConfirmTimeout)
			reconcileVM(r)
			Expect(events).NotTo(BeEmpty())
			Expect(events[0].object.GetName()).To(Equal(vmName))
			Expect(events[0].reason).To(Equal(v1alpha2.ReasonFilesystemFreezeCleanupFailed))
			Expect(hasFilesystemRequest(c)).To(BeFalse())
		})
	})

	It("denies a claim when the guest agent is not connected and the freeze is required", func() {
		c, err := testutil.NewFakeClientWithObjects(newVM(), newKVVM(), newKVVMI(false), freezeClaim("a", "a-uid"))
		Expect(err).NotTo(HaveOccurred())

		reconcileVM(newReconciler(c))

		expectGranted(getClaim(c, "a"), metav1.ConditionFalse, vmaccondition.FilesystemFreezeFailed)
		Expect(getLock(c)).To(BeNil())
	})

	It("grants without a freeze when the guest agent is not connected and the policy is BestEffort", func() {
		claim := freezeClaim("a", "a-uid")
		claim.Spec.FreezePolicy = ptr.To(coordinationv1alpha1.FreezePolicyBestEffort)
		c, err := testutil.NewFakeClientWithObjects(newVM(), newKVVM(), newKVVMI(false), claim)
		Expect(err).NotTo(HaveOccurred())

		reconcileVM(newReconciler(c))

		got := getClaim(c, "a")
		expectGranted(got, metav1.ConditionTrue, vmaccondition.FreezeSkipped)
		Expect(got.Status.Frozen).To(BeFalse())
		Expect(getLock(c)).NotTo(BeNil())
	})

	It("skips a best-effort freeze once the guest agent disconnects while the freeze is retried", func() {
		claim := freezeClaim("a", "a-uid")
		claim.Spec.FreezePolicy = ptr.To(coordinationv1alpha1.FreezePolicyBestEffort)
		c, err := testutil.NewFakeClientWithObjects(newVM(), newKVVM(), newKVVMI(true), claim)
		Expect(err).NotTo(HaveOccurred())

		r := newReconciler(c)
		freezeErr = errors.New("the guest agent did not answer")

		reconcileVM(r)
		expectGranted(getClaim(c, "a"), metav1.ConditionFalse, vmaccondition.Freezing)

		kvvmi := &virtv1.VirtualMachineInstance{}
		Expect(c.Get(ctx, key, kvvmi)).To(Succeed())
		kvvmi.Status.Conditions[0].Status = corev1.ConditionFalse
		Expect(c.Status().Update(ctx, kvvmi)).To(Succeed())

		reconcileVM(r)

		got := getClaim(c, "a")
		expectGranted(got, metav1.ConditionTrue, vmaccondition.FreezeSkipped)
		Expect(got.Status.Frozen).To(BeFalse())
		Expect(freezes).To(HaveLen(1))
	})

	It("denies a claim whose freeze never goes through, and unfreezes a freeze that may have landed late", func() {
		c, err := testutil.NewFakeClientWithObjects(newVM(), newKVVM(), newKVVMI(true), freezeClaim("a", "a-uid"))
		Expect(err).NotTo(HaveOccurred())

		r := newReconciler(c)
		freezeErr = errors.New("the guest agent did not answer")

		reconcileVM(r)
		expectGranted(getClaim(c, "a"), metav1.ConditionFalse, vmaccondition.Freezing)
		advance(internal.FreezeConfirmTimeout + time.Second)
		reconcileVM(r)

		got := getClaim(c, "a")
		Expect(got.Status.Phase).To(Equal(coordinationv1alpha1.AccessClaimPhaseDenied))
		expectGranted(got, metav1.ConditionFalse, vmaccondition.FilesystemFreezeFailed)
		Expect(unfreezes).To(Equal([]types.UID{"a-uid"}))
		Expect(getLock(c)).NotTo(BeNil(), "kept while the unfreeze is resent")

		advance(internal.RequestResendInterval)
		reconcileVM(r)
		advance(internal.RequestResendInterval)
		reconcileVM(r)
		Expect(getLock(c)).NotTo(BeNil())
		advance(internal.PollInterval)
		reconcileVM(r)

		Expect(unfreezes).To(Equal([]types.UID{"a-uid", "a-uid", "a-uid"}))
		Expect(getLock(c)).To(BeNil())
	})

	It("releases the lock at once when the freeze was skipped for a guest without an agent", func() {
		claim := freezeClaim("a", "a-uid")
		claim.Spec.FreezePolicy = ptr.To(coordinationv1alpha1.FreezePolicyBestEffort)
		c, err := testutil.NewFakeClientWithObjects(newVM(), newKVVM(), newKVVMI(false), claim)
		Expect(err).NotTo(HaveOccurred())

		r := newReconciler(c)
		reconcileVM(r)
		expectGranted(getClaim(c, "a"), metav1.ConditionTrue, vmaccondition.FreezeSkipped)

		release(c, "a")
		reconcileVM(r)

		Expect(getClaim(c, "a").Status.Phase).To(Equal(coordinationv1alpha1.AccessClaimPhaseCompleted))
		Expect(unfreezes).To(BeEmpty())
		Expect(getLock(c)).To(BeNil())
	})

	It("denies claims for a virtual machine that is not ready or was recreated", func() {
		recreated := freezeClaim("recreated", "r-uid")
		recreated.Spec.VirtualMachine.UID = "old-vm-uid"
		c, err := testutil.NewFakeClientWithObjects(newVM(), recreated)
		Expect(err).NotTo(HaveOccurred())
		reconcileVM(newReconciler(c))
		expectGranted(getClaim(c, "recreated"), metav1.ConditionFalse, vmaccondition.VirtualMachineStateChanged)

		c, err = testutil.NewFakeClientWithObjects(newVM(), freezeClaim("no-kvvm", "n-uid"))
		Expect(err).NotTo(HaveOccurred())
		reconcileVM(newReconciler(c))
		expectGranted(getClaim(c, "no-kvvm"), metav1.ConditionFalse, vmaccondition.VirtualMachineNotReady)
	})

	It("aborts a session past its deadline and unfreezes the guest", func() {
		c, err := testutil.NewFakeClientWithObjects(newVM(), newKVVM(), newKVVMI(true), freezeClaim("a", "a-uid"))
		Expect(err).NotTo(HaveOccurred())

		r := newReconciler(c)
		grantFrozen(c, r, "a")

		advance(timeout)
		reconcileVM(r)

		got := getClaim(c, "a")
		Expect(got.Status.Phase).To(Equal(coordinationv1alpha1.AccessClaimPhaseAborted))
		expectGranted(got, metav1.ConditionFalse, vmaccondition.DeadlineExceeded)
		Expect(unfreezes).To(Equal([]types.UID{"a-uid"}))
	})

	Context("a released session the coordinator sees late", func() {
		migration := func() *virtv1.VirtualMachineInstanceMigration {
			return &virtv1.VirtualMachineInstanceMigration{
				ObjectMeta: metav1.ObjectMeta{Name: "migration", Namespace: namespace},
				Spec:       virtv1.VirtualMachineInstanceMigrationSpec{VMIName: vmName},
				Status:     virtv1.VirtualMachineInstanceMigrationStatus{Phase: virtv1.MigrationRunning},
			}
		}

		DescribeTable("completes despite a violation that leaves the captured data intact",
			func(violate func(c client.Client, r *Reconciler)) {
				kvvmi := newKVVMI(true)
				kvvmi.Finalizers = []string{"test/keep"}
				c, err := testutil.NewFakeClientWithObjects(newVM(), newKVVM(), kvvmi, freezeClaim("a", "a-uid"))
				Expect(err).NotTo(HaveOccurred())

				r := newReconciler(c)
				grantFrozen(c, r, "a")

				violate(c, r)
				release(c, "a")
				reconcileVM(r)

				got := getClaim(c, "a")
				Expect(got.Status.Phase).To(Equal(coordinationv1alpha1.AccessClaimPhaseCompleted))
				Expect(got.Status.Frozen).To(BeTrue())
				Expect(unfreezes).To(Equal([]types.UID{"a-uid"}))
			},
			Entry("past the deadline with the guest still frozen", func(client.Client, *Reconciler) { advance(timeout) }),
			Entry("with the instance stopping", func(c client.Client, _ *Reconciler) {
				stopping := &virtv1.VirtualMachineInstance{}
				Expect(c.Get(ctx, key, stopping)).To(Succeed())
				Expect(c.Delete(ctx, stopping)).To(Succeed())
			}),
			Entry("with a migration started", func(c client.Client, _ *Reconciler) { Expect(c.Create(ctx, migration())).To(Succeed()) }),
			Entry("with fsFreezeStatus reading empty for long", func(c client.Client, r *Reconciler) {
				updateKVVMI(c, func(vmi *virtv1.VirtualMachineInstance) { vmi.Status.FSFreezeStatus = "" })
				reconcileVM(r)
				advance(internal.UnfrozenTolerance + time.Second)
			}),
		)

		It("aborts past the deadline when the guest no longer reads frozen", func() {
			c, err := testutil.NewFakeClientWithObjects(newVM(), newKVVM(), newKVVMI(true), freezeClaim("a", "a-uid"))
			Expect(err).NotTo(HaveOccurred())

			r := newReconciler(c)
			grantFrozen(c, r, "a")

			advance(timeout)
			updateKVVMI(c, func(vmi *virtv1.VirtualMachineInstance) { vmi.Status.FSFreezeStatus = "" })
			release(c, "a")
			reconcileVM(r)

			got := getClaim(c, "a")
			Expect(got.Status.Phase).To(Equal(coordinationv1alpha1.AccessClaimPhaseAborted))
			expectGranted(got, metav1.ConditionFalse, vmaccondition.DeadlineExceeded)
			Expect(unfreezes).To(Equal([]types.UID{"a-uid"}))
			Expect(getLock(c)).To(BeNil())
		})

		It("aborts when the instance was replaced", func() {
			c, err := testutil.NewFakeClientWithObjects(newVM(), newKVVM(), newKVVMI(true), freezeClaim("a", "a-uid"))
			Expect(err).NotTo(HaveOccurred())

			r := newReconciler(c)
			grantFrozen(c, r, "a")

			release(c, "a")
			Expect(c.Delete(ctx, newKVVMI(true))).To(Succeed())
			restarted := newKVVMI(true)
			restarted.UID = "restarted-vmi-uid"
			Expect(c.Create(ctx, restarted)).To(Succeed())
			reconcileVM(r)

			got := getClaim(c, "a")
			Expect(got.Status.Phase).To(Equal(coordinationv1alpha1.AccessClaimPhaseAborted))
			expectGranted(got, metav1.ConditionFalse, vmaccondition.VirtualMachineStateChanged)
		})
	})

	Context("a final status that fails to be written", func() {
		var failStatus bool

		newFailingClient := func(objs ...client.Object) client.Client {
			GinkgoHelper()

			failStatus = false
			c, err := testutil.NewFakeClientWithInterceptorWithObjects(interceptor.Funcs{
				SubResourcePatch: func(ctx context.Context, c client.Client, sub string, obj client.Object, patch client.Patch, opts ...client.SubResourcePatchOption) error {
					if _, ok := obj.(*coordinationv1alpha1.VirtualMachineAccessClaim); ok && failStatus {
						return errors.New("status write failed")
					}

					return c.SubResource(sub).Patch(ctx, obj, patch, opts...)
				},
			}, objs...)
			Expect(err).NotTo(HaveOccurred())

			return c
		}
		reconcileFailing := func(r *Reconciler) {
			GinkgoHelper()

			_, err := r.Reconcile(ctx, reconcile.Request{NamespacedName: key})
			Expect(err).To(HaveOccurred())
		}

		It("keeps the lock of an aborted session until its verdict is written", func() {
			c := newFailingClient(newVM(), newKVVM(), newKVVMI(true), freezeClaim("a", "a-uid"))
			r := newReconciler(c)
			grantFrozen(c, r, "a")

			advance(timeout)

			failStatus = true

			reconcileFailing(r)
			Expect(getClaim(c, "a").Status.Phase).To(Equal(coordinationv1alpha1.AccessClaimPhaseGranted))
			Expect(getLock(c)).NotTo(BeNil(), "a released lock would let the claim be granted again")

			failStatus = false

			reconcileVM(r)
			advance(internal.RequestResendInterval)
			reconcileVM(r)

			got := getClaim(c, "a")
			Expect(got.Status.Phase).To(Equal(coordinationv1alpha1.AccessClaimPhaseAborted))
			expectGranted(got, metav1.ConditionFalse, vmaccondition.DeadlineExceeded)
			Expect(getLock(c)).To(BeNil())
			Expect(freezes).To(HaveLen(1), "the guest is not frozen again")
		})

		It("keeps the lock of a completed session until its unfreeze is written", func() {
			c := newFailingClient(newVM(), newKVVM(), newKVVMI(true), freezeClaim("a", "a-uid"))
			r := newReconciler(c)
			grantFrozen(c, r, "a")

			release(c, "a")

			failStatus = true

			reconcileFailing(r)
			Expect(getLock(c)).NotTo(BeNil())

			failStatus = false

			reconcileVM(r)
			advance(internal.RequestResendInterval)
			reconcileVM(r)

			got := getClaim(c, "a")
			Expect(got.Status.Phase).To(Equal(coordinationv1alpha1.AccessClaimPhaseCompleted))
			Expect(meta.FindStatusCondition(got.Status.Conditions, vmaccondition.UnfrozenType.String()).Reason).To(Equal(vmaccondition.UnfreezeConfirmed.String()))
			Expect(getLock(c)).To(BeNil())
			Expect(freezes).To(HaveLen(1), "the guest is not frozen again")
		})
	})

	It("tolerates a short empty fsFreezeStatus and aborts on a lasting one", func() {
		c, err := testutil.NewFakeClientWithObjects(newVM(), newKVVM(), newKVVMI(true), freezeClaim("a", "a-uid"))
		Expect(err).NotTo(HaveOccurred())

		r := newReconciler(c)
		grantFrozen(c, r, "a")

		updateKVVMI(c, func(vmi *virtv1.VirtualMachineInstance) { vmi.Status.FSFreezeStatus = "" })
		reconcileVM(r)
		advance(internal.UnfrozenTolerance / 2)
		reconcileVM(r)
		Expect(getClaim(c, "a").Status.Phase).To(Equal(coordinationv1alpha1.AccessClaimPhaseGranted))

		updateKVVMI(c, func(vmi *virtv1.VirtualMachineInstance) { vmi.Status.FSFreezeStatus = "frozen" })
		reconcileVM(r)
		updateKVVMI(c, func(vmi *virtv1.VirtualMachineInstance) { vmi.Status.FSFreezeStatus = "" })
		reconcileVM(r)
		advance(internal.UnfrozenTolerance / 2)
		reconcileVM(r)
		Expect(getClaim(c, "a").Status.Phase).To(Equal(coordinationv1alpha1.AccessClaimPhaseGranted), "a freeze seen again restarts the tolerance")

		advance(internal.UnfrozenTolerance)
		reconcileVM(r)

		got := getClaim(c, "a")
		Expect(got.Status.Phase).To(Equal(coordinationv1alpha1.AccessClaimPhaseAborted))
		expectGranted(got, metav1.ConditionFalse, vmaccondition.FilesystemUnfrozenUnexpectedly)
	})

	It("denies a claim while the virtual machine migrates and aborts a session a migration starts in", func() {
		migration := &virtv1.VirtualMachineInstanceMigration{
			ObjectMeta: metav1.ObjectMeta{Name: "migration", Namespace: namespace},
			Spec:       virtv1.VirtualMachineInstanceMigrationSpec{VMIName: vmName},
			Status:     virtv1.VirtualMachineInstanceMigrationStatus{Phase: virtv1.MigrationRunning},
		}
		c, err := testutil.NewFakeClientWithObjects(newVM(), newKVVM(), newKVVMI(true), migration, freezeClaim("a", "a-uid"))
		Expect(err).NotTo(HaveOccurred())
		reconcileVM(newReconciler(c))
		got := getClaim(c, "a")
		expectGranted(got, metav1.ConditionFalse, vmaccondition.VirtualMachineBusy)
		Expect(meta.FindStatusCondition(got.Status.Conditions, vmaccondition.GrantedType.String()).Message).To(ContainSubstring("migrating"))

		c, err = testutil.NewFakeClientWithObjects(newVM(), newKVVM(), newKVVMI(true), freezeClaim("b", "b-uid"))
		Expect(err).NotTo(HaveOccurred())

		r := newReconciler(c)
		grantFrozen(c, r, "b")

		migration.ResourceVersion = ""
		Expect(c.Create(ctx, migration)).To(Succeed())
		reconcileVM(r)

		got = getClaim(c, "b")
		Expect(got.Status.Phase).To(Equal(coordinationv1alpha1.AccessClaimPhaseAborted))
		expectGranted(got, metav1.ConditionFalse, vmaccondition.VirtualMachineStateChanged)
	})

	It("names the operation that migrates the virtual machine when it denies a claim", func() {
		migration := &virtv1.VirtualMachineInstanceMigration{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "vmop-migrate",
				Namespace: namespace,
				OwnerReferences: []metav1.OwnerReference{{
					APIVersion: v1alpha2.SchemeGroupVersion.String(),
					Kind:       v1alpha2.VirtualMachineOperationKind,
					Name:       "migrate",
					UID:        "migrate-uid",
					Controller: ptr.To(true),
				}},
			},
			Spec:   virtv1.VirtualMachineInstanceMigrationSpec{VMIName: vmName},
			Status: virtv1.VirtualMachineInstanceMigrationStatus{Phase: virtv1.MigrationRunning},
		}
		c, err := testutil.NewFakeClientWithObjects(newVM(), newKVVM(), newKVVMI(true), migration, freezeClaim("a", "a-uid"))
		Expect(err).NotTo(HaveOccurred())

		reconcileVM(newReconciler(c))

		got := getClaim(c, "a")
		expectGranted(got, metav1.ConditionFalse, vmaccondition.VirtualMachineBusy)
		Expect(meta.FindStatusCondition(got.Status.Conditions, vmaccondition.GrantedType.String()).Message).To(ContainSubstring(`VirtualMachineOperation "migrate"`))
	})

	It("ignores a finished migration", func() {
		migration := &virtv1.VirtualMachineInstanceMigration{
			ObjectMeta: metav1.ObjectMeta{Name: "migration", Namespace: namespace},
			Spec:       virtv1.VirtualMachineInstanceMigrationSpec{VMIName: vmName},
			Status:     virtv1.VirtualMachineInstanceMigrationStatus{Phase: virtv1.MigrationSucceeded},
		}
		c, err := testutil.NewFakeClientWithObjects(newVM(), newKVVM(), newKVVMI(true), migration, freezeClaim("a", "a-uid"))
		Expect(err).NotTo(HaveOccurred())

		reconcileVM(newReconciler(c))

		expectGranted(getClaim(c, "a"), metav1.ConditionTrue, vmaccondition.Frozen)
	})

	It("completes and releases the lock when the unfreeze never goes through", func() {
		c, err := testutil.NewFakeClientWithObjects(newVM(), newKVVM(), newKVVMI(true), freezeClaim("a", "a-uid"))
		Expect(err).NotTo(HaveOccurred())

		r := newReconciler(c)
		grantFrozen(c, r, "a")

		unfreezeErr = errors.New("the guest agent did not answer")

		release(c, "a")
		reconcileVM(r)
		advance(internal.RequestResendInterval)
		reconcileVM(r)
		Expect(getLock(c)).NotTo(BeNil())
		advance(internal.UnfreezeConfirmTimeout)
		reconcileVM(r)

		got := getClaim(c, "a")
		Expect(got.Status.Phase).To(Equal(coordinationv1alpha1.AccessClaimPhaseCompleted))
		unfrozen := meta.FindStatusCondition(got.Status.Conditions, vmaccondition.UnfrozenType.String())
		Expect(unfrozen.Status).To(Equal(metav1.ConditionFalse))
		Expect(unfrozen.Reason).To(Equal(vmaccondition.UnfreezeNotConfirmed.String()))
		Expect(getLock(c)).To(BeNil())
		Expect(len(unfreezes)).To(BeNumerically(">=", 2), "the unfreeze is resent while waiting")
	})

	It("unfreezes the guest before letting a deleted holder go", func() {
		c, err := testutil.NewFakeClientWithObjects(newVM(), newKVVM(), newKVVMI(true), freezeClaim("a", "a-uid"))
		Expect(err).NotTo(HaveOccurred())

		r := newReconciler(c)
		grantFrozen(c, r, "a")

		unfreezeErr = errors.New("the guest agent did not answer")

		Expect(c.Delete(ctx, getClaim(c, "a"))).To(Succeed())
		reconcileVM(r)
		Expect(getClaim(c, "a").Finalizers).To(ContainElement(v1alpha2.FinalizerVMACCleanup), "kept until the unfreeze goes through")

		unfreezeErr = nil

		advance(internal.RequestResendInterval)
		reconcileVM(r)
		Expect(getLock(c)).To(BeNil())
		err = c.Get(ctx, types.NamespacedName{Namespace: namespace, Name: "a"}, &coordinationv1alpha1.VirtualMachineAccessClaim{})
		Expect(apierrors.IsNotFound(err)).To(BeTrue(), "the claim is gone once its finalizer is removed")
	})

	It("unfreezes and releases a lock whose claim is gone", func() {
		kvvm := newKVVM()
		Expect(vmaccess.WriteLock(kvvm, &vmaccess.Lock{ClaimUID: "gone-uid", VMIUID: vmiUID})).To(Succeed())

		kvvmi := newKVVMI(true)
		kvvmi.Status.FSFreezeStatus = "frozen"
		c, err := testutil.NewFakeClientWithObjects(newVM(), kvvm, kvvmi)
		Expect(err).NotTo(HaveOccurred())

		reconcileVM(newReconciler(c))

		Expect(unfreezes).To(Equal([]types.UID{"gone-uid"}))
		Expect(getLock(c)).To(BeNil())
	})

	It("keeps a lock whose claim is gone until the unfreeze goes through", func() {
		kvvm := newKVVM()
		Expect(vmaccess.WriteLock(kvvm, &vmaccess.Lock{ClaimUID: "gone-uid", VMIUID: vmiUID})).To(Succeed())
		c, err := testutil.NewFakeClientWithObjects(newVM(), kvvm, newKVVMI(true))
		Expect(err).NotTo(HaveOccurred())

		r := newReconciler(c)
		unfreezeErr = errors.New("the guest agent did not answer")

		reconcileVM(r)
		Expect(getLock(c)).NotTo(BeNil())

		unfreezeErr = nil

		advance(internal.RequestResendInterval)
		reconcileVM(r)

		Expect(unfreezes).To(Equal([]types.UID{"gone-uid", "gone-uid"}))
		Expect(getLock(c)).To(BeNil())
	})

	It("releases a lock whose claim is gone when the unfreeze never goes through", func() {
		kvvm := newKVVM()
		Expect(vmaccess.WriteLock(kvvm, &vmaccess.Lock{ClaimUID: "gone-uid", VMIUID: vmiUID})).To(Succeed())
		c, err := testutil.NewFakeClientWithObjects(newVM(), kvvm, newKVVMI(true))
		Expect(err).NotTo(HaveOccurred())

		r := newReconciler(c)
		unfreezeErr = errors.New("the guest agent did not answer")

		reconcileVM(r)
		advance(internal.UnfreezeConfirmTimeout)
		reconcileVM(r)

		Expect(getLock(c)).To(BeNil())
	})

	It("takes the lock only once the finalizer of the claim is written", func() {
		c, err := testutil.NewFakeClientWithObjects(newVM(), newKVVM(), newKVVMI(true), freezeClaim("a", "a-uid"))
		Expect(err).NotTo(HaveOccurred())

		r := newReconciler(c)

		_, err = r.Reconcile(ctx, reconcile.Request{NamespacedName: key})
		Expect(err).NotTo(HaveOccurred())
		Expect(getClaim(c, "a").Finalizers).To(ContainElement(v1alpha2.FinalizerVMACCleanup))
		Expect(getLock(c)).To(BeNil())
		Expect(freezes).To(BeEmpty())

		_, err = r.Reconcile(ctx, reconcile.Request{NamespacedName: key})
		Expect(err).NotTo(HaveOccurred())
		Expect(getLock(c)).NotTo(BeNil())
		Expect(freezes).To(HaveLen(1))
	})

	It("aborts the session and unfreezes the guest as soon as the instance starts stopping", func() {
		kvvmi := newKVVMI(true)
		kvvmi.Finalizers = []string{"test/keep"}
		c, err := testutil.NewFakeClientWithObjects(newVM(), newKVVM(), kvvmi, freezeClaim("a", "a-uid"))
		Expect(err).NotTo(HaveOccurred())

		r := newReconciler(c)
		grantFrozen(c, r, "a")

		stopping := &virtv1.VirtualMachineInstance{}
		Expect(c.Get(ctx, key, stopping)).To(Succeed())
		Expect(c.Delete(ctx, stopping)).To(Succeed())
		reconcileVM(r)

		got := getClaim(c, "a")
		Expect(got.Status.Phase).To(Equal(coordinationv1alpha1.AccessClaimPhaseAborted))
		expectGranted(got, metav1.ConditionFalse, vmaccondition.VirtualMachineStateChanged)
		Expect(unfreezes).To(Equal([]types.UID{"a-uid"}), "a frozen guest cannot shut down gracefully")
	})

	It("releases the lock once the instance finished, despite the freeze status it was left with", func() {
		kvvmi := newKVVMI(true)
		kvvmi.Finalizers = []string{"test/keep"}
		c, err := testutil.NewFakeClientWithObjects(newVM(), newKVVM(), kvvmi, freezeClaim("a", "a-uid"))
		Expect(err).NotTo(HaveOccurred())

		r := newReconciler(c)
		grantFrozen(c, r, "a")

		unfreezeErr = errors.New("the instance is shutting down")

		stopping := &virtv1.VirtualMachineInstance{}
		Expect(c.Get(ctx, key, stopping)).To(Succeed())
		Expect(c.Delete(ctx, stopping)).To(Succeed())
		reconcileVM(r)
		Expect(getLock(c)).NotTo(BeNil(), "held while the running guest may still be frozen")

		updateKVVMI(c, func(vmi *virtv1.VirtualMachineInstance) { vmi.Status.Phase = virtv1.Succeeded })
		reconcileVM(r)

		Expect(getLock(c)).To(BeNil())
		unfrozen := meta.FindStatusCondition(getClaim(c, "a").Status.Conditions, vmaccondition.UnfrozenType.String())
		Expect(unfrozen.Reason).To(Equal(vmaccondition.UnfreezeConfirmed.String()))
	})

	It("denies a claim while the instance is stopping", func() {
		kvvmi := newKVVMI(true)
		kvvmi.Finalizers = []string{"test/keep"}
		c, err := testutil.NewFakeClientWithObjects(newVM(), newKVVM(), kvvmi)
		Expect(err).NotTo(HaveOccurred())
		Expect(c.Delete(ctx, kvvmi)).To(Succeed())
		Expect(c.Create(ctx, freezeClaim("a", "a-uid"))).To(Succeed())

		reconcileVM(newReconciler(c))

		expectGranted(getClaim(c, "a"), metav1.ConditionFalse, vmaccondition.VirtualMachineNotReady)
		Expect(getLock(c)).To(BeNil())
	})

	It("grants a stopped machine whose finished instance is still being deleted", func() {
		kvvmi := newKVVMI(true)
		kvvmi.Finalizers = []string{"test/keep"}
		kvvmi.Status.Phase = virtv1.Succeeded
		c, err := testutil.NewFakeClientWithObjects(newVM(), newKVVM(), kvvmi)
		Expect(err).NotTo(HaveOccurred())
		Expect(c.Delete(ctx, kvvmi)).To(Succeed())
		Expect(c.Create(ctx, freezeClaim("a", "a-uid"))).To(Succeed())
		r := newReconciler(c)

		reconcileVM(r)
		reconcileVM(r)

		got := getClaim(c, "a")
		Expect(got.Status.Phase).To(Equal(coordinationv1alpha1.AccessClaimPhaseGranted))
		expectGranted(got, metav1.ConditionTrue, vmaccondition.NotRequired)
	})

	It("grants a holder whose grant was not written instead of waiting for a freeze", func() {
		kvvm := newKVVM()
		Expect(vmaccess.WriteLock(kvvm, &vmaccess.Lock{ClaimUID: "a-uid", VMIUID: vmiUID})).To(Succeed())
		c, err := testutil.NewFakeClientWithObjects(newVM(), kvvm, newKVVMI(true),
			newClaim("a", "a-uid", now, coordinationv1alpha1.AccessModeSnapshot))
		Expect(err).NotTo(HaveOccurred())

		reconcileVM(newReconciler(c))

		got := getClaim(c, "a")
		expectGranted(got, metav1.ConditionTrue, vmaccondition.NotRequired)
		Expect(got.Status.VirtualMachineInstanceUID).To(Equal(vmiUID))
		Expect(freezes).To(BeEmpty())
	})

	It("grants a claim without a freeze on a running machine and aborts it on a restart", func() {
		c, err := testutil.NewFakeClientWithObjects(newVM(), newKVVM(), newKVVMI(true),
			newClaim("a", "a-uid", now, coordinationv1alpha1.AccessModeSnapshot))
		Expect(err).NotTo(HaveOccurred())

		r := newReconciler(c)

		reconcileVM(r)
		expectGranted(getClaim(c, "a"), metav1.ConditionTrue, vmaccondition.NotRequired)
		Expect(freezes).To(BeEmpty())

		Expect(c.Delete(ctx, newKVVMI(true))).To(Succeed())
		restarted := newKVVMI(true)
		restarted.UID = "restarted-vmi-uid"
		Expect(c.Create(ctx, restarted)).To(Succeed())
		reconcileVM(r)
		expectGranted(getClaim(c, "a"), metav1.ConditionFalse, vmaccondition.VirtualMachineStateChanged)
	})

	Context("a granted claim the lock does not name", func() {
		setLock := func(c client.Client, lock *vmaccess.Lock) {
			GinkgoHelper()

			kvvm := &virtv1.VirtualMachine{}
			Expect(c.Get(ctx, key, kvvm)).To(Succeed())
			Expect(vmaccess.WriteLock(kvvm, lock)).To(Succeed())
			Expect(c.Update(ctx, kvvm)).To(Succeed())
		}

		It("keeps its session and holds off other claims while the lock is missing from the cache", func() {
			c, err := testutil.NewFakeClientWithObjects(newVM(), newKVVM(), newKVVMI(true), freezeClaim("a", "a-uid"))
			Expect(err).NotTo(HaveOccurred())

			r := newReconciler(c)
			grantFrozen(c, r, "a")
			lock := getLock(c)

			setLock(c, nil)
			Expect(c.Create(ctx, newClaim("b", "b-uid", now.Add(time.Second), coordinationv1alpha1.AccessModeSnapshot))).To(Succeed())

			res := reconcileVM(r)
			Expect(res.RequeueAfter).To(Equal(internal.LockLossTolerance))

			got := getClaim(c, "a")
			Expect(got.Status.Phase).To(Equal(coordinationv1alpha1.AccessClaimPhaseGranted), "a frozen guest is its own freeze, not one made outside the coordinator")
			expectGranted(got, metav1.ConditionTrue, vmaccondition.Frozen)
			Expect(getLock(c)).To(BeNil(), "the lock is not taken again")
			Expect(freezes).To(HaveLen(1), "the guest is not frozen again")
			expectGranted(getClaim(c, "b"), metav1.ConditionFalse, vmaccondition.VirtualMachineBusy)

			setLock(c, lock)
			advance(internal.LockLossTolerance)
			reconcileVM(r)
			Expect(getClaim(c, "a").Status.Phase).To(Equal(coordinationv1alpha1.AccessClaimPhaseGranted))

			release(c, "a")
			reconcileVM(r)
			Expect(getClaim(c, "a").Status.Phase).To(Equal(coordinationv1alpha1.AccessClaimPhaseCompleted))
			Expect(getLock(c)).To(BeNil())
		})

		It("aborts the session, unfreezes the guest and releases the lock once the lock stays lost", func() {
			c, err := testutil.NewFakeClientWithObjects(newVM(), newKVVM(), newKVVMI(true), freezeClaim("a", "a-uid"))
			Expect(err).NotTo(HaveOccurred())

			r := newReconciler(c)
			grantFrozen(c, r, "a")

			setLock(c, nil)
			reconcileVM(r)
			advance(internal.LockLossTolerance)
			reconcileVM(r)
			reconcileVM(r)

			got := getClaim(c, "a")
			Expect(got.Status.Phase).To(Equal(coordinationv1alpha1.AccessClaimPhaseAborted))
			expectGranted(got, metav1.ConditionFalse, vmaccondition.VirtualMachineStateChanged)
			Expect(meta.FindStatusCondition(got.Status.Conditions, vmaccondition.UnfrozenType.String()).Reason).To(Equal(vmaccondition.UnfreezeConfirmed.String()))
			Expect(unfreezes).To(Equal([]types.UID{"a-uid"}))
			Expect(getLock(c)).To(BeNil())
		})

		It("drops a lock that names no claim and handles the claim that took the lock as one without it", func() {
			c, err := testutil.NewFakeClientWithObjects(newVM(), newKVVM(), newKVVMI(true), freezeClaim("a", "a-uid"))
			Expect(err).NotTo(HaveOccurred())

			r := newReconciler(c)
			grantFrozen(c, r, "a")

			kvvm := &virtv1.VirtualMachine{}
			Expect(c.Get(ctx, key, kvvm)).To(Succeed())
			kvvm.Annotations[annotations.AnnVMAccessLock] = "garbage"
			Expect(c.Update(ctx, kvvm)).To(Succeed())

			reconcileVM(r)
			Expect(getLock(c)).To(BeNil())
			Expect(getClaim(c, "a").Status.Phase).To(Equal(coordinationv1alpha1.AccessClaimPhaseGranted))

			advance(internal.LockLossTolerance)
			reconcileVM(r)
			reconcileVM(r)

			got := getClaim(c, "a")
			Expect(got.Status.Phase).To(Equal(coordinationv1alpha1.AccessClaimPhaseAborted))
			expectGranted(got, metav1.ConditionFalse, vmaccondition.VirtualMachineStateChanged)
			Expect(unfreezes).To(Equal([]types.UID{"a-uid"}))
			Expect(getLock(c)).To(BeNil())
		})

		It("drops a lock that names no claim when the virtual machine has no claims", func() {
			kvvm := newKVVM()
			kvvm.Annotations = map[string]string{annotations.AnnVMAccessLock: "garbage"}
			c, err := testutil.NewFakeClientWithObjects(newVM(), kvvm, newKVVMI(true))
			Expect(err).NotTo(HaveOccurred())

			reconcileVM(newReconciler(c))

			Expect(c.Get(ctx, key, kvvm)).To(Succeed())
			Expect(kvvm.Annotations).NotTo(HaveKey(annotations.AnnVMAccessLock))
		})

		It("aborts at once when the internal virtual machine is gone", func() {
			c, err := testutil.NewFakeClientWithObjects(newVM(), newKVVM(), newKVVMI(true), freezeClaim("a", "a-uid"))
			Expect(err).NotTo(HaveOccurred())

			r := newReconciler(c)
			grantFrozen(c, r, "a")

			Expect(c.Delete(ctx, newKVVMI(true))).To(Succeed())
			Expect(c.Delete(ctx, newKVVM())).To(Succeed())
			reconcileVM(r)

			got := getClaim(c, "a")
			Expect(got.Status.Phase).To(Equal(coordinationv1alpha1.AccessClaimPhaseAborted))
			expectGranted(got, metav1.ConditionFalse, vmaccondition.VirtualMachineStateChanged)
			Expect(got.Finalizers).NotTo(ContainElement(v1alpha2.FinalizerVMACCleanup))
			Expect(unfreezes).To(BeEmpty())
		})

		It("aborts at once when the virtual machine was recreated, without locking the new one", func() {
			c, err := testutil.NewFakeClientWithObjects(newVM(), newKVVM(), newKVVMI(true), freezeClaim("a", "a-uid"))
			Expect(err).NotTo(HaveOccurred())

			r := newReconciler(c)
			grantFrozen(c, r, "a")

			Expect(c.Delete(ctx, newKVVMI(true))).To(Succeed())
			Expect(c.Delete(ctx, newKVVM())).To(Succeed())
			Expect(c.Delete(ctx, newVM())).To(Succeed())
			recreated := newVM()
			recreated.UID = "recreated-vm-uid"
			Expect(c.Create(ctx, recreated)).To(Succeed())
			Expect(c.Create(ctx, newKVVM())).To(Succeed())
			reconcileVM(r)

			got := getClaim(c, "a")
			Expect(got.Status.Phase).To(Equal(coordinationv1alpha1.AccessClaimPhaseAborted))
			expectGranted(got, metav1.ConditionFalse, vmaccondition.VirtualMachineStateChanged)
			Expect(getLock(c)).To(BeNil())
		})

		It("aborts without touching the guest when another claim holds the lock", func() {
			c, err := testutil.NewFakeClientWithObjects(newVM(), newKVVM(), newKVVMI(true), freezeClaim("a", "a-uid"))
			Expect(err).NotTo(HaveOccurred())

			r := newReconciler(c)
			grantFrozen(c, r, "a")

			other := &vmaccess.Lock{ClaimUID: "other-uid", VMIUID: vmiUID}
			setLock(c, other)
			Expect(c.Create(ctx, newClaim("other", "other-uid", now.Add(time.Second), coordinationv1alpha1.AccessModeSnapshot))).To(Succeed())
			reconcileVM(r)
			advance(internal.LockLossTolerance)
			reconcileVM(r)

			got := getClaim(c, "a")
			Expect(got.Status.Phase).To(Equal(coordinationv1alpha1.AccessClaimPhaseAborted))
			expectGranted(got, metav1.ConditionFalse, vmaccondition.VirtualMachineStateChanged)
			Expect(unfreezes).To(BeEmpty())
			Expect(getLock(c)).To(Equal(other))
		})
	})
})
