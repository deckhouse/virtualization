//go:build EE
// +build EE

/*
Copyright 2026 Flant JSC
Licensed under the Deckhouse Platform Enterprise Edition (EE) license. See https://github.com/deckhouse/deckhouse/blob/main/ee/LICENSE
*/

package handler

import (
	"context"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/deckhouse/virtualization-controller/pkg/common/testutil"
	"github.com/deckhouse/virtualization-controller/pkg/controller/vmpool/internal/expectations"
	"github.com/deckhouse/virtualization-controller/pkg/controller/vmpool/internal/poollabels"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/vmpoolcondition"
)

const (
	poolNamespace = "ci"
	poolName      = "web"
	poolUID       = types.UID("pool-uid-0001")
)

func newPool(replicas int32) *v1alpha2.VirtualMachinePool {
	return &v1alpha2.VirtualMachinePool{
		ObjectMeta: metav1.ObjectMeta{
			Name:       poolName,
			Namespace:  poolNamespace,
			UID:        poolUID,
			Generation: 1,
		},
		Spec: v1alpha2.VirtualMachinePoolSpec{
			Replicas:        ptr.To(replicas),
			ScaleDownPolicy: v1alpha2.ScaleDownPolicyNewestFirst,
		},
	}
}

// newMemberVM builds a VM that belongs to pool: the managed labels and the
// controller ownerReference are what listMembers keys on.
func newMemberVM(pool *v1alpha2.VirtualMachinePool, name string, phase v1alpha2.MachinePhase, createdAt time.Time, terminating bool) *v1alpha2.VirtualMachine {
	vm := &v1alpha2.VirtualMachine{
		ObjectMeta: metav1.ObjectMeta{
			Name:              name,
			Namespace:         pool.Namespace,
			UID:               types.UID(name + "-uid"),
			Labels:            poollabels.Member(pool),
			CreationTimestamp: metav1.NewTime(createdAt),
			OwnerReferences:   []metav1.OwnerReference{*metav1.NewControllerRef(pool, v1alpha2.VirtualMachinePoolGVK)},
		},
		Status: v1alpha2.VirtualMachineStatus{Phase: phase},
	}
	if terminating {
		ts := metav1.NewTime(createdAt.Add(time.Hour))
		vm.DeletionTimestamp = &ts
		vm.Finalizers = []string{"test.local/keep"}
	}
	return vm
}

func listMemberNames(ctx context.Context, c client.Client, pool *v1alpha2.VirtualMachinePool) []string {
	var list v1alpha2.VirtualMachineList
	Expect(c.List(ctx, &list, client.InNamespace(pool.Namespace), poollabels.MemberSelector(pool))).To(Succeed())
	names := make([]string, 0, len(list.Items))
	for i := range list.Items {
		names = append(names, list.Items[i].Name)
	}
	return names
}

var _ = Describe("SyncHandler", func() {
	var (
		ctx   context.Context
		exp   *expectations.Expectations
		clock time.Time
	)

	BeforeEach(func() {
		ctx = context.Background()
		exp = expectations.New()
		clock = time.Unix(1_700_000_000, 0)
	})

	Context("scale up", func() {
		It("creates the missing replicas from the template", func() {
			pool := newPool(3)
			c, err := testutil.NewFakeClientWithObjects(pool)
			Expect(err).NotTo(HaveOccurred())

			h := NewSyncHandler(c, exp)
			_, err = h.Handle(ctx, pool)
			Expect(err).NotTo(HaveOccurred())

			members := listMemberNames(ctx, c, pool)
			Expect(members).To(HaveLen(3))
		})

		It("stamps managed labels and a controller ownerReference on each replica", func() {
			pool := newPool(1)
			c, err := testutil.NewFakeClientWithObjects(pool)
			Expect(err).NotTo(HaveOccurred())

			_, err = NewSyncHandler(c, exp).Handle(ctx, pool)
			Expect(err).NotTo(HaveOccurred())

			var list v1alpha2.VirtualMachineList
			Expect(c.List(ctx, &list, client.InNamespace(pool.Namespace))).To(Succeed())
			Expect(list.Items).To(HaveLen(1))
			vm := list.Items[0]
			Expect(vm.Name).To(HavePrefix(poolName + "-"))
			Expect(vm.Labels).To(HaveKeyWithValue(poollabels.PoolUID, string(poolUID)))
			Expect(vm.Labels).To(HaveKeyWithValue(poollabels.Pool, poolName))
			Expect(vm.Labels).To(HaveKeyWithValue(poollabels.TemplateHash, poollabels.ComputeTemplateHash(pool)))
			ref := metav1.GetControllerOf(&vm)
			Expect(ref).NotTo(BeNil())
			Expect(ref.UID).To(Equal(poolUID))
			Expect(ref.Kind).To(Equal(v1alpha2.VirtualMachinePoolKind))
		})

		It("does not create again while creations are unobserved (cache-lag guard)", func() {
			pool := newPool(3)
			c, err := testutil.NewFakeClientWithObjects(pool)
			Expect(err).NotTo(HaveOccurred())
			h := NewSyncHandler(c, exp)

			// First pass creates 3 and records expectations.
			_, err = h.Handle(ctx, pool)
			Expect(err).NotTo(HaveOccurred())
			Expect(listMemberNames(ctx, c, pool)).To(HaveLen(3))

			// Second pass: cache now shows 3, but expectations are unmet — the
			// handler must NOT create 3 more. It requeues instead.
			res, err := h.Handle(ctx, pool)
			Expect(err).NotTo(HaveOccurred())
			Expect(res.RequeueAfter).To(BeNumerically(">", 0))
			Expect(listMemberNames(ctx, c, pool)).To(HaveLen(3))
		})
	})

	Context("steady state", func() {
		It("neither creates nor deletes when live == desired", func() {
			pool := newPool(2)
			m1 := newMemberVM(pool, "web-aaaaa", v1alpha2.MachineRunning, clock, false)
			m2 := newMemberVM(pool, "web-bbbbb", v1alpha2.MachineRunning, clock, false)
			c, err := testutil.NewFakeClientWithObjects(pool, m1, m2)
			Expect(err).NotTo(HaveOccurred())

			_, err = NewSyncHandler(c, exp).Handle(ctx, pool)
			Expect(err).NotTo(HaveOccurred())

			Expect(listMemberNames(ctx, c, pool)).To(HaveLen(2))
			Expect(pool.Status.Replicas).To(Equal(int32(2)))
			Expect(pool.Status.ReadyReplicas).To(Equal(int32(2)))
			Expect(pool.Status.Selector).To(ContainSubstring(string(poolUID)))
			Expect(meta.IsStatusConditionTrue(pool.Status.Conditions, vmpoolcondition.TypeAvailable.String())).To(BeTrue())
			Expect(meta.IsStatusConditionFalse(pool.Status.Conditions, vmpoolcondition.TypeProgressing.String())).To(BeTrue())
		})
	})

	Context("template revision", func() {
		It("reports Synced when every replica is on the current template hash", func() {
			pool := newPool(2)
			hash := poollabels.ComputeTemplateHash(pool)
			m1 := newMemberVM(pool, "web-a", v1alpha2.MachineRunning, clock, false)
			m2 := newMemberVM(pool, "web-b", v1alpha2.MachineRunning, clock, false)
			m1.Labels[poollabels.TemplateHash] = hash
			m2.Labels[poollabels.TemplateHash] = hash
			c, err := testutil.NewFakeClientWithObjects(pool, m1, m2)
			Expect(err).NotTo(HaveOccurred())

			_, err = NewSyncHandler(c, exp).Handle(ctx, pool)
			Expect(err).NotTo(HaveOccurred())

			Expect(pool.Status.DesiredTemplateHash).To(Equal(hash))
			Expect(pool.Status.UpdatedReplicas).To(Equal(int32(2)))
			Expect(meta.IsStatusConditionTrue(pool.Status.Conditions, vmpoolcondition.TypeSynced.String())).To(BeTrue())
		})

		It("reports Synced=False when a replica lags on an old hash", func() {
			pool := newPool(2)
			hash := poollabels.ComputeTemplateHash(pool)
			current := newMemberVM(pool, "web-a", v1alpha2.MachineRunning, clock, false)
			lagging := newMemberVM(pool, "web-b", v1alpha2.MachineRunning, clock, false)
			current.Labels[poollabels.TemplateHash] = hash
			lagging.Labels[poollabels.TemplateHash] = "stale"
			c, err := testutil.NewFakeClientWithObjects(pool, current, lagging)
			Expect(err).NotTo(HaveOccurred())

			_, err = NewSyncHandler(c, exp).Handle(ctx, pool)
			Expect(err).NotTo(HaveOccurred())

			Expect(pool.Status.UpdatedReplicas).To(Equal(int32(1)))
			Expect(meta.IsStatusConditionFalse(pool.Status.Conditions, vmpoolcondition.TypeSynced.String())).To(BeTrue())
		})
	})

	Context("scale down", func() {
		It("deletes the youngest surplus replicas", func() {
			pool := newPool(1)
			older := newMemberVM(pool, "web-old", v1alpha2.MachineRunning, clock, false)
			newer := newMemberVM(pool, "web-new", v1alpha2.MachineRunning, clock.Add(time.Minute), false)
			c, err := testutil.NewFakeClientWithObjects(pool, older, newer)
			Expect(err).NotTo(HaveOccurred())

			_, err = NewSyncHandler(c, exp).Handle(ctx, pool)
			Expect(err).NotTo(HaveOccurred())

			remaining := listMemberNames(ctx, c, pool)
			Expect(remaining).To(ConsistOf("web-old")) // newest removed first
		})

		It("deletes the oldest surplus replicas under OldestFirst", func() {
			pool := newPool(1)
			pool.Spec.ScaleDownPolicy = v1alpha2.ScaleDownPolicyOldestFirst
			older := newMemberVM(pool, "web-old", v1alpha2.MachineRunning, clock, false)
			newer := newMemberVM(pool, "web-new", v1alpha2.MachineRunning, clock.Add(time.Minute), false)
			c, err := testutil.NewFakeClientWithObjects(pool, older, newer)
			Expect(err).NotTo(HaveOccurred())

			_, err = NewSyncHandler(c, exp).Handle(ctx, pool)
			Expect(err).NotTo(HaveOccurred())

			Expect(listMemberNames(ctx, c, pool)).To(ConsistOf("web-new")) // oldest removed first
		})

		It("removes nothing anonymously under Explicit", func() {
			pool := newPool(1)
			pool.Spec.ScaleDownPolicy = v1alpha2.ScaleDownPolicyExplicit
			m1 := newMemberVM(pool, "web-a", v1alpha2.MachineRunning, clock, false)
			m2 := newMemberVM(pool, "web-b", v1alpha2.MachineRunning, clock.Add(time.Minute), false)
			c, err := testutil.NewFakeClientWithObjects(pool, m1, m2)
			Expect(err).NotTo(HaveOccurred())

			_, err = NewSyncHandler(c, exp).Handle(ctx, pool)
			Expect(err).NotTo(HaveOccurred())

			// Explicit forbids anonymous scale-down: both replicas stay.
			Expect(listMemberNames(ctx, c, pool)).To(ConsistOf("web-a", "web-b"))
		})
	})

	Context("Terminating accounting (invariant 2)", func() {
		It("counts a Terminating member toward the reduction and deletes fewer healthy ones", func() {
			pool := newPool(1)
			// live=3, desired=1 => surplus 2; one member already Terminating counts
			// as one of those two, so only ONE healthy replica should be deleted.
			terminating := newMemberVM(pool, "web-term", v1alpha2.MachineRunning, clock, true)
			healthyOld := newMemberVM(pool, "web-old", v1alpha2.MachineRunning, clock.Add(time.Minute), false)
			healthyNew := newMemberVM(pool, "web-new", v1alpha2.MachineRunning, clock.Add(2*time.Minute), false)
			c, err := testutil.NewFakeClientWithObjects(pool, terminating, healthyOld, healthyNew)
			Expect(err).NotTo(HaveOccurred())

			_, err = NewSyncHandler(c, exp).Handle(ctx, pool)
			Expect(err).NotTo(HaveOccurred())

			remaining := listMemberNames(ctx, c, pool)
			// web-new (youngest healthy) deleted; web-term still present (Terminating,
			// held by finalizer); web-old kept.
			Expect(remaining).To(ConsistOf("web-term", "web-old"))
		})
	})
})
