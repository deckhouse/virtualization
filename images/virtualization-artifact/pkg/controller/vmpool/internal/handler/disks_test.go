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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/deckhouse/virtualization-controller/pkg/common/testutil"
	"github.com/deckhouse/virtualization-controller/pkg/controller/vmpool/internal/poollabels"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/vmcondition"
)

func retainTemplate(name string, keep int32, ttl *metav1.Duration) v1alpha2.VirtualDiskTemplateSpec {
	return v1alpha2.VirtualDiskTemplateSpec{
		Name:    name,
		Reclaim: v1alpha2.VirtualDiskReclaim{OnScaleDown: v1alpha2.VirtualDiskReclaimRetain, Keep: keep, TTL: ttl},
	}
}

func diskTemplate(name string, policy v1alpha2.VirtualDiskReclaimPolicy) v1alpha2.VirtualDiskTemplateSpec {
	return v1alpha2.VirtualDiskTemplateSpec{
		Name:    name,
		Reclaim: v1alpha2.VirtualDiskReclaim{OnScaleDown: policy},
	}
}

func diskExists(ctx context.Context, c client.Client, name string) (*v1alpha2.VirtualDisk, bool) {
	vd := &v1alpha2.VirtualDisk{}
	err := c.Get(ctx, types.NamespacedName{Namespace: poolNamespace, Name: name}, vd)
	if err != nil {
		return nil, false
	}
	return vd, true
}

// reuseDisk builds a free pool-owned Retain disk of the "cache" template.
func reuseDisk(pool *v1alpha2.VirtualMachinePool, name string, phase v1alpha2.DiskPhase) *v1alpha2.VirtualDisk {
	return &v1alpha2.VirtualDisk{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: poolNamespace,
			Labels: map[string]string{
				poollabels.PoolUID:      string(pool.GetUID()),
				poollabels.Pool:         pool.GetName(),
				poollabels.DiskTemplate: "cache",
			},
			OwnerReferences: []metav1.OwnerReference{*metav1.NewControllerRef(pool, v1alpha2.VirtualMachinePoolGVK)},
		},
		Status: v1alpha2.VirtualDiskStatus{Phase: phase},
	}
}

func listReuseDisks(ctx context.Context, c client.Client, dtName string) []v1alpha2.VirtualDisk {
	var list v1alpha2.VirtualDiskList
	Expect(c.List(ctx, &list, client.InNamespace(poolNamespace), client.MatchingLabels{poollabels.DiskTemplate: dtName})).To(Succeed())
	return list.Items
}

var _ = Describe("DisksHandler", func() {
	var ctx context.Context
	BeforeEach(func() { ctx = context.Background() })

	It("creates a Delete disk owned by the member and attaches it", func() {
		pool := newPool(1)
		pool.Spec.VirtualDiskTemplates = []v1alpha2.VirtualDiskTemplateSpec{diskTemplate("system", v1alpha2.VirtualDiskReclaimDelete)}
		m := newMemberVM(pool, "web-a", v1alpha2.MachineRunning, referenceTime, false)
		c, err := testutil.NewFakeClientWithObjects(pool, m)
		Expect(err).NotTo(HaveOccurred())

		_, err = NewDisksHandler(c).Handle(ctx, pool)
		Expect(err).NotTo(HaveOccurred())

		vd, ok := diskExists(ctx, c, "web-a-system")
		Expect(ok).To(BeTrue())
		Expect(vd.Labels).To(HaveKeyWithValue(poollabels.DiskTemplate, "system"))
		ref := metav1.GetControllerOf(vd)
		Expect(ref).NotTo(BeNil())
		Expect(ref.Kind).To(Equal(v1alpha2.VirtualMachineKind))
		Expect(ref.Name).To(Equal("web-a"))

		got := getVM(ctx, c, "web-a")
		Expect(got.Spec.BlockDeviceRefs).To(ContainElement(v1alpha2.BlockDeviceSpecRef{Kind: v1alpha2.DiskDevice, Name: "web-a-system"}))
	})

	It("is idempotent — a second pass creates no duplicate and adds no second ref", func() {
		pool := newPool(1)
		pool.Spec.VirtualDiskTemplates = []v1alpha2.VirtualDiskTemplateSpec{diskTemplate("system", v1alpha2.VirtualDiskReclaimDelete)}
		m := newMemberVM(pool, "web-a", v1alpha2.MachineRunning, referenceTime, false)
		c, err := testutil.NewFakeClientWithObjects(pool, m)
		Expect(err).NotTo(HaveOccurred())

		h := NewDisksHandler(c)
		_, err = h.Handle(ctx, pool)
		Expect(err).NotTo(HaveOccurred())
		_, err = h.Handle(ctx, pool)
		Expect(err).NotTo(HaveOccurred())

		got := getVM(ctx, c, "web-a")
		count := 0
		for _, ref := range got.Spec.BlockDeviceRefs {
			if ref.Name == "web-a-system" {
				count++
			}
		}
		Expect(count).To(Equal(1))
	})

	It("creates a pool-owned Retain disk and attaches it", func() {
		pool := newPool(1)
		pool.Spec.VirtualDiskTemplates = []v1alpha2.VirtualDiskTemplateSpec{diskTemplate("cache", v1alpha2.VirtualDiskReclaimRetain)}
		m := newMemberVM(pool, "web-a", v1alpha2.MachineRunning, referenceTime, false)
		c, err := testutil.NewFakeClientWithObjects(pool, m)
		Expect(err).NotTo(HaveOccurred())

		_, err = NewDisksHandler(c).Handle(ctx, pool)
		Expect(err).NotTo(HaveOccurred())

		disks := listReuseDisks(ctx, c, "cache")
		Expect(disks).To(HaveLen(1))
		Expect(disks[0].Name).To(HavePrefix(poolName + "-cache-"))
		ref := metav1.GetControllerOf(&disks[0])
		Expect(ref.Kind).To(Equal(v1alpha2.VirtualMachinePoolKind)) // owned by the pool, not the VM
		Expect(getVM(ctx, c, "web-a").Spec.BlockDeviceRefs).To(ContainElement(v1alpha2.BlockDeviceSpecRef{Kind: v1alpha2.DiskDevice, Name: disks[0].Name}))
	})

	It("reuses a free Ready disk instead of creating a new one", func() {
		pool := newPool(1)
		pool.Spec.VirtualDiskTemplates = []v1alpha2.VirtualDiskTemplateSpec{diskTemplate("cache", v1alpha2.VirtualDiskReclaimRetain)}
		free := reuseDisk(pool, "web-cache-free", v1alpha2.DiskReady)
		m := newMemberVM(pool, "web-a", v1alpha2.MachineRunning, referenceTime, false)
		c, err := testutil.NewFakeClientWithObjects(pool, free, m)
		Expect(err).NotTo(HaveOccurred())

		_, err = NewDisksHandler(c).Handle(ctx, pool)
		Expect(err).NotTo(HaveOccurred())

		Expect(listReuseDisks(ctx, c, "cache")).To(HaveLen(1)) // no new disk created
		Expect(getVM(ctx, c, "web-a").Spec.BlockDeviceRefs).To(ContainElement(v1alpha2.BlockDeviceSpecRef{Kind: v1alpha2.DiskDevice, Name: "web-cache-free"}))
	})

	It("does not reuse a disk already held by another live member", func() {
		pool := newPool(2)
		pool.Spec.VirtualDiskTemplates = []v1alpha2.VirtualDiskTemplateSpec{diskTemplate("cache", v1alpha2.VirtualDiskReclaimRetain)}
		busy := reuseDisk(pool, "web-cache-busy", v1alpha2.DiskReady)
		holder := newMemberVM(pool, "web-holder", v1alpha2.MachineRunning, referenceTime, false)
		holder.Spec.BlockDeviceRefs = []v1alpha2.BlockDeviceSpecRef{{Kind: v1alpha2.DiskDevice, Name: "web-cache-busy"}}
		newcomer := newMemberVM(pool, "web-new", v1alpha2.MachineRunning, referenceTime, false)
		c, err := testutil.NewFakeClientWithObjects(pool, busy, holder, newcomer)
		Expect(err).NotTo(HaveOccurred())

		_, err = NewDisksHandler(c).Handle(ctx, pool)
		Expect(err).NotTo(HaveOccurred())

		// The busy disk stays with its holder; the newcomer gets a fresh one.
		Expect(listReuseDisks(ctx, c, "cache")).To(HaveLen(2))
		Expect(getVM(ctx, c, "web-new").Spec.BlockDeviceRefs).NotTo(ContainElement(v1alpha2.BlockDeviceSpecRef{Kind: v1alpha2.DiskDevice, Name: "web-cache-busy"}))
	})

	It("does not reuse a disk that is not Ready", func() {
		pool := newPool(1)
		pool.Spec.VirtualDiskTemplates = []v1alpha2.VirtualDiskTemplateSpec{diskTemplate("cache", v1alpha2.VirtualDiskReclaimRetain)}
		pending := reuseDisk(pool, "web-cache-pending", v1alpha2.DiskPending)
		m := newMemberVM(pool, "web-a", v1alpha2.MachineRunning, referenceTime, false)
		c, err := testutil.NewFakeClientWithObjects(pool, pending, m)
		Expect(err).NotTo(HaveOccurred())

		_, err = NewDisksHandler(c).Handle(ctx, pool)
		Expect(err).NotTo(HaveOccurred())

		// The pending disk is not reused; a fresh one is created instead.
		Expect(listReuseDisks(ctx, c, "cache")).To(HaveLen(2))
		Expect(getVM(ctx, c, "web-a").Spec.BlockDeviceRefs).NotTo(ContainElement(v1alpha2.BlockDeviceSpecRef{Kind: v1alpha2.DiskDevice, Name: "web-cache-pending"}))
	})

	It("clears free-since when a free disk is reused", func() {
		pool := newPool(1)
		pool.Spec.VirtualDiskTemplates = []v1alpha2.VirtualDiskTemplateSpec{retainTemplate("cache", 0, &metav1.Duration{Duration: 30 * time.Minute})}
		free := reuseDisk(pool, "web-cache-1", v1alpha2.DiskReady)
		free.Annotations = map[string]string{poollabels.FreeSince: referenceTime.UTC().Format(time.RFC3339)}
		m := newMemberVM(pool, "web-a", v1alpha2.MachineRunning, referenceTime, false)
		c, err := testutil.NewFakeClientWithObjects(pool, free, m)
		Expect(err).NotTo(HaveOccurred())

		h := NewDisksHandler(c)
		h.now = func() time.Time { return referenceTime }
		_, err = h.Handle(ctx, pool)
		Expect(err).NotTo(HaveOccurred())

		Expect(getVM(ctx, c, "web-a").Spec.BlockDeviceRefs).To(ContainElement(v1alpha2.BlockDeviceSpecRef{Kind: v1alpha2.DiskDevice, Name: "web-cache-1"}))
		vd, ok := diskExists(ctx, c, "web-cache-1")
		Expect(ok).To(BeTrue())
		Expect(vd.Annotations).NotTo(HaveKey(poollabels.FreeSince)) // cleared on reuse
	})

	It("does not manage disks for a Terminating member", func() {
		pool := newPool(1)
		pool.Spec.VirtualDiskTemplates = []v1alpha2.VirtualDiskTemplateSpec{diskTemplate("system", v1alpha2.VirtualDiskReclaimDelete)}
		term := newMemberVM(pool, "web-term", v1alpha2.MachineRunning, referenceTime, true)
		c, err := testutil.NewFakeClientWithObjects(pool, term)
		Expect(err).NotTo(HaveOccurred())

		_, err = NewDisksHandler(c).Handle(ctx, pool)
		Expect(err).NotTo(HaveOccurred())

		_, ok := diskExists(ctx, c, "web-term-system")
		Expect(ok).To(BeFalse())
	})

	It("detaches a colliding reuse disk from the stuck member (fallback)", func() {
		pool := newPool(2)
		pool.Spec.VirtualDiskTemplates = []v1alpha2.VirtualDiskTemplateSpec{diskTemplate("cache", v1alpha2.VirtualDiskReclaimRetain)}
		shared := reuseDisk(pool, "web-cache-shared", v1alpha2.DiskReady)

		keeper := newMemberVM(pool, "web-keeper", v1alpha2.MachineRunning, referenceTime, false)
		keeper.Spec.BlockDeviceRefs = []v1alpha2.BlockDeviceSpecRef{{Kind: v1alpha2.DiskDevice, Name: "web-cache-shared"}}
		keeper.Status.Conditions = []metav1.Condition{{Type: vmcondition.TypeBlockDevicesReady.String(), Status: metav1.ConditionTrue, Reason: "Ready"}}

		stuck := newMemberVM(pool, "web-stuck", v1alpha2.MachineRunning, referenceTime, false)
		stuck.Spec.BlockDeviceRefs = []v1alpha2.BlockDeviceSpecRef{{Kind: v1alpha2.DiskDevice, Name: "web-cache-shared"}}
		stuck.Status.Conditions = []metav1.Condition{{Type: vmcondition.TypeBlockDevicesReady.String(), Status: metav1.ConditionFalse, Reason: "InUseByAnother"}}

		c, err := testutil.NewFakeClientWithObjects(pool, shared, keeper, stuck)
		Expect(err).NotTo(HaveOccurred())

		_, err = NewDisksHandler(c).Handle(ctx, pool)
		Expect(err).NotTo(HaveOccurred())

		sharedRef := v1alpha2.BlockDeviceSpecRef{Kind: v1alpha2.DiskDevice, Name: "web-cache-shared"}
		Expect(getVM(ctx, c, "web-keeper").Spec.BlockDeviceRefs).To(ContainElement(sharedRef))   // keeper (BlockDevicesReady=True) keeps it
		Expect(getVM(ctx, c, "web-stuck").Spec.BlockDeviceRefs).NotTo(ContainElement(sharedRef)) // stuck one detached
	})

	Context("GC of free reuse disks", func() {
		ttl := &metav1.Duration{Duration: 30 * time.Minute}

		// gcPool has no members, so ensureRetainDisk never reuses the free disks
		// under test and the GC pass operates on them.
		gcPool := func(keep int32, ttl *metav1.Duration) *v1alpha2.VirtualMachinePool {
			p := newPool(0)
			p.Spec.VirtualDiskTemplates = []v1alpha2.VirtualDiskTemplateSpec{retainTemplate("cache", keep, ttl)}
			return p
		}

		handlerAt := func(c client.Client, now time.Time) *DisksHandler {
			h := NewDisksHandler(c)
			h.now = func() time.Time { return now }
			return h
		}

		It("stamps free-since on a newly free disk and keeps it", func() {
			pool := gcPool(0, ttl)
			free := reuseDisk(pool, "web-cache-1", v1alpha2.DiskReady)
			c, err := testutil.NewFakeClientWithObjects(pool, free)
			Expect(err).NotTo(HaveOccurred())

			_, err = handlerAt(c, referenceTime).Handle(ctx, pool)
			Expect(err).NotTo(HaveOccurred())

			vd, ok := diskExists(ctx, c, "web-cache-1")
			Expect(ok).To(BeTrue()) // just freed (age 0) — kept
			Expect(vd.Annotations).To(HaveKey(poollabels.FreeSince))
		})

		It("deletes a free disk older than ttl and outside the keep buffer", func() {
			pool := gcPool(0, ttl)
			free := reuseDisk(pool, "web-cache-1", v1alpha2.DiskReady)
			free.Annotations = map[string]string{poollabels.FreeSince: referenceTime.Add(-time.Hour).UTC().Format(time.RFC3339)}
			c, err := testutil.NewFakeClientWithObjects(pool, free)
			Expect(err).NotTo(HaveOccurred())

			_, err = handlerAt(c, referenceTime).Handle(ctx, pool)
			Expect(err).NotTo(HaveOccurred())

			_, ok := diskExists(ctx, c, "web-cache-1")
			Expect(ok).To(BeFalse()) // 1h free > 30m ttl, keep=0 → collected
		})

		It("keeps the warm buffer even past ttl", func() {
			pool := gcPool(1, ttl) // keep 1
			free := reuseDisk(pool, "web-cache-1", v1alpha2.DiskReady)
			free.Annotations = map[string]string{poollabels.FreeSince: referenceTime.Add(-time.Hour).UTC().Format(time.RFC3339)}
			c, err := testutil.NewFakeClientWithObjects(pool, free)
			Expect(err).NotTo(HaveOccurred())

			_, err = handlerAt(c, referenceTime).Handle(ctx, pool)
			Expect(err).NotTo(HaveOccurred())

			_, ok := diskExists(ctx, c, "web-cache-1")
			Expect(ok).To(BeTrue()) // within the keep buffer
		})

		It("does not collect anything when ttl is nil", func() {
			pool := gcPool(0, nil) // no ttl
			free := reuseDisk(pool, "web-cache-1", v1alpha2.DiskReady)
			free.Annotations = map[string]string{poollabels.FreeSince: referenceTime.Add(-100 * time.Hour).UTC().Format(time.RFC3339)}
			c, err := testutil.NewFakeClientWithObjects(pool, free)
			Expect(err).NotTo(HaveOccurred())

			_, err = handlerAt(c, referenceTime).Handle(ctx, pool)
			Expect(err).NotTo(HaveOccurred())

			_, ok := diskExists(ctx, c, "web-cache-1")
			Expect(ok).To(BeTrue()) // no ttl → never aged out
		})
	})
})
