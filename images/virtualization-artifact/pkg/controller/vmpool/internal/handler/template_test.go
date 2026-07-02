//go:build EE
// +build EE

/*
Copyright 2026 Flant JSC
Licensed under the Deckhouse Platform Enterprise Edition (EE) license. See https://github.com/deckhouse/deckhouse/blob/main/ee/LICENSE
*/

package handler

import (
	"context"

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

func getVM(ctx context.Context, c client.Client, name string) *v1alpha2.VirtualMachine {
	vm := &v1alpha2.VirtualMachine{}
	Expect(c.Get(ctx, types.NamespacedName{Namespace: poolNamespace, Name: name}, vm)).To(Succeed())
	return vm
}

var _ = Describe("TemplateHandler", func() {
	var ctx context.Context
	BeforeEach(func() {
		ctx = context.Background()
	})

	poolWithRunPolicy := func(p v1alpha2.RunPolicy) *v1alpha2.VirtualMachinePool {
		pool := newPool(1)
		pool.Spec.VirtualMachineTemplate.Spec.RunPolicy = p
		return pool
	}

	It("patches a lagging replica's spec and records the patched revision", func() {
		pool := poolWithRunPolicy(v1alpha2.AlwaysOnPolicy)
		m := newMemberVM(pool, "web-a", v1alpha2.MachineRunning, referenceTime, false)
		m.Spec.RunPolicy = v1alpha2.AlwaysOnUnlessStoppedManually // differs from template
		c, err := testutil.NewFakeClientWithObjects(pool, m)
		Expect(err).NotTo(HaveOccurred())

		_, err = NewTemplateHandler(c).Handle(ctx, pool)
		Expect(err).NotTo(HaveOccurred())

		got := getVM(ctx, c, "web-a")
		Expect(got.Spec.RunPolicy).To(Equal(v1alpha2.AlwaysOnPolicy))
		Expect(got.Annotations).To(HaveKeyWithValue(poollabels.PatchedTemplateHash, poollabels.ComputeTemplateHash(pool)))
		// The effectively-applied label is only set on a subsequent pass.
		Expect(got.Labels).NotTo(HaveKeyWithValue(poollabels.TemplateHash, poollabels.ComputeTemplateHash(pool)))
	})

	It("marks the replica on the current template once patched and not awaiting restart", func() {
		pool := poolWithRunPolicy(v1alpha2.AlwaysOnPolicy)
		hash := poollabels.ComputeTemplateHash(pool)
		m := newMemberVM(pool, "web-a", v1alpha2.MachineRunning, referenceTime, false)
		m.Annotations = map[string]string{poollabels.PatchedTemplateHash: hash}
		m.Labels[poollabels.TemplateHash] = "old"
		m.Spec.RunPolicy = v1alpha2.AlwaysOnPolicy
		c, err := testutil.NewFakeClientWithObjects(pool, m)
		Expect(err).NotTo(HaveOccurred())

		_, err = NewTemplateHandler(c).Handle(ctx, pool)
		Expect(err).NotTo(HaveOccurred())

		Expect(getVM(ctx, c, "web-a").Labels).To(HaveKeyWithValue(poollabels.TemplateHash, hash))
	})

	It("keeps the old revision label while the replica awaits a restart", func() {
		pool := poolWithRunPolicy(v1alpha2.AlwaysOnPolicy)
		hash := poollabels.ComputeTemplateHash(pool)
		m := newMemberVM(pool, "web-a", v1alpha2.MachineRunning, referenceTime, false)
		m.Annotations = map[string]string{poollabels.PatchedTemplateHash: hash}
		m.Labels[poollabels.TemplateHash] = "old"
		m.Spec.RunPolicy = v1alpha2.AlwaysOnPolicy
		m.Status.Conditions = []metav1.Condition{{
			Type:   vmcondition.TypeAwaitingRestartToApplyConfiguration.String(),
			Status: metav1.ConditionTrue,
			Reason: "PendingRestart",
		}}
		c, err := testutil.NewFakeClientWithObjects(pool, m)
		Expect(err).NotTo(HaveOccurred())

		_, err = NewTemplateHandler(c).Handle(ctx, pool)
		Expect(err).NotTo(HaveOccurred())

		Expect(getVM(ctx, c, "web-a").Labels[poollabels.TemplateHash]).To(Equal("old"))
	})

	It("does not re-patch or relabel a stable replica", func() {
		pool := poolWithRunPolicy(v1alpha2.AlwaysOnPolicy)
		hash := poollabels.ComputeTemplateHash(pool)
		m := newMemberVM(pool, "web-a", v1alpha2.MachineRunning, referenceTime, false)
		m.Annotations = map[string]string{poollabels.PatchedTemplateHash: hash}
		m.Labels[poollabels.TemplateHash] = hash
		m.Spec.RunPolicy = v1alpha2.AlwaysOnPolicy
		c, err := testutil.NewFakeClientWithObjects(pool, m)
		Expect(err).NotTo(HaveOccurred())

		before := getVM(ctx, c, "web-a").ResourceVersion
		_, err = NewTemplateHandler(c).Handle(ctx, pool)
		Expect(err).NotTo(HaveOccurred())
		Expect(getVM(ctx, c, "web-a").ResourceVersion).To(Equal(before)) // no write happened
	})
})
