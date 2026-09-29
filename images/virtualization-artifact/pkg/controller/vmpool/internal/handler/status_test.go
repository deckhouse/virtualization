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

package handler

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/deckhouse/virtualization-controller/pkg/common/testutil"
	"github.com/deckhouse/virtualization-controller/pkg/controller/vmpool/internal/poollabels"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/vmcondition"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/vmpoolcondition"
)

var _ = Describe("StatusHandler", func() {
	var ctx context.Context

	BeforeEach(func() {
		ctx = context.Background()
	})

	handleIn := func(ctx context.Context, pool *v1alpha2.VirtualMachinePool, objs ...client.Object) {
		c, err := testutil.NewFakeClientWithObjects(append([]client.Object{pool}, objs...)...)
		Expect(err).NotTo(HaveOccurred())
		_, err = NewStatusHandler(c).Handle(ctx, pool)
		Expect(err).NotTo(HaveOccurred())
	}
	handle := func(pool *v1alpha2.VirtualMachinePool, objs ...client.Object) {
		handleIn(ctx, pool, objs...)
	}

	Context("steady state", func() {
		It("reports every replica ready and drops Progressing", func() {
			pool := newPool(2)
			handle(pool,
				newMemberVM(pool, "web-aaaaa", v1alpha2.MachineRunning, referenceTime, false),
				newMemberVM(pool, "web-bbbbb", v1alpha2.MachineRunning, referenceTime, false),
			)

			Expect(pool.Status.Replicas).To(Equal(int32(2)))
			Expect(pool.Status.ReadyReplicas).To(Equal(int32(2)))
			Expect(pool.Status.Selector).To(ContainSubstring(string(poolUID)))
			Expect(meta.IsStatusConditionTrue(pool.Status.Conditions, vmpoolcondition.TypeAvailable.String())).To(BeTrue())
			// Steady state: the Progressing condition is removed, not kept at False.
			Expect(meta.FindStatusCondition(pool.Status.Conditions, vmpoolcondition.TypeProgressing.String())).To(BeNil())
		})

		It("counts a Stopped member but not as ready", func() {
			pool := newPool(1)
			handle(pool, newMemberVM(pool, "web-stopped", v1alpha2.MachineStopped, referenceTime, false))

			Expect(pool.Status.Replicas).To(Equal(int32(1)))
			Expect(pool.Status.ReadyReplicas).To(Equal(int32(0)))
			Expect(meta.IsStatusConditionFalse(pool.Status.Conditions, vmpoolcondition.TypeAvailable.String())).To(BeTrue())
		})
	})

	Context("template revision", func() {
		It("reports Synced when every replica is on the current template hash", func() {
			pool := newPool(2)
			hash := poollabels.ComputeTemplateHash(pool)
			m1 := newMemberVM(pool, "web-a", v1alpha2.MachineRunning, referenceTime, false)
			m2 := newMemberVM(pool, "web-b", v1alpha2.MachineRunning, referenceTime, false)
			m1.Labels[poollabels.TemplateHash] = hash
			m2.Labels[poollabels.TemplateHash] = hash
			handle(pool, m1, m2)

			Expect(pool.Status.DesiredTemplateHash).To(Equal(hash))
			Expect(pool.Status.UpdatedReplicas).To(Equal(int32(2)))
			Expect(meta.IsStatusConditionTrue(pool.Status.Conditions, vmpoolcondition.TypeSynced.String())).To(BeTrue())
		})

		It("reports Synced=False when a replica lags on an old hash", func() {
			pool := newPool(2)
			hash := poollabels.ComputeTemplateHash(pool)
			current := newMemberVM(pool, "web-a", v1alpha2.MachineRunning, referenceTime, false)
			lagging := newMemberVM(pool, "web-b", v1alpha2.MachineRunning, referenceTime, false)
			current.Labels[poollabels.TemplateHash] = hash
			lagging.Labels[poollabels.TemplateHash] = "stale"
			handle(pool, current, lagging)

			Expect(pool.Status.UpdatedReplicas).To(Equal(int32(1)))
			Expect(meta.IsStatusConditionFalse(pool.Status.Conditions, vmpoolcondition.TypeSynced.String())).To(BeTrue())
		})
	})

	It("keeps the last status of a pool being deleted", func() {
		pool := newPool(1)
		now := metav1.Now()
		pool.DeletionTimestamp = &now
		pool.Finalizers = []string{"test.local/keep"}
		handle(pool)

		Expect(pool.Status.Conditions).To(BeEmpty())
	})

	Context("availability", func() {
		It("counts a migrating replica as ready", func() {
			pool := newPool(1)
			handle(pool, newMemberVM(pool, "web-a", v1alpha2.MachineMigrating, referenceTime, false))

			Expect(pool.Status.ReadyReplicas).To(Equal(int32(1)))
			Expect(conditionReason(pool, vmpoolcondition.TypeAvailable)).To(Equal(vmpoolcondition.ReasonAllReplicasReady.String()))
		})

		It("reports ReplicasStopped with a hint for the Manual run policy", func() {
			pool := newPool(2)
			handle(pool,
				withRunPolicy(newMemberVM(pool, "web-a", v1alpha2.MachineStopped, referenceTime, false), v1alpha2.ManualPolicy),
				withRunPolicy(newMemberVM(pool, "web-b", v1alpha2.MachineStopping, referenceTime, false), v1alpha2.ManualPolicy),
			)

			c := meta.FindStatusCondition(pool.Status.Conditions, vmpoolcondition.TypeAvailable.String())
			Expect(c.Status).To(Equal(metav1.ConditionFalse))
			Expect(c.Reason).To(Equal(vmpoolcondition.ReasonReplicasStopped.String()))
			Expect(c.Message).To(Equal("2 of 2 replicas are stopped or changing the power state."))
		})

		It("hints the Manual run policy when every replica is stopped", func() {
			pool := newPool(2)
			handle(pool,
				withRunPolicy(newMemberVM(pool, "web-a", v1alpha2.MachineStopped, referenceTime, false), v1alpha2.ManualPolicy),
				withRunPolicy(newMemberVM(pool, "web-b", v1alpha2.MachineStopped, referenceTime, false), v1alpha2.ManualPolicy),
			)

			Expect(conditionMessage(pool, vmpoolcondition.TypeAvailable)).To(Equal("2 of 2 replicas are stopped. The run policy is Manual: start the virtual machines manually or change the run policy."))
		})

		It("hints to start the replicas under AlwaysOnUnlessStoppedManually", func() {
			pool := newPool(1)
			handle(pool, withRunPolicy(newMemberVM(pool, "web-a", v1alpha2.MachineStopped, referenceTime, false), v1alpha2.AlwaysOnUnlessStoppedManually))

			Expect(conditionMessage(pool, vmpoolcondition.TypeAvailable)).To(Equal("1 of 1 replica is stopped. Start it to restore the pool capacity."))
		})

		It("does not tell to start manually under AlwaysOff", func() {
			pool := newPool(1)
			handle(pool, withRunPolicy(newMemberVM(pool, "web-a", v1alpha2.MachineStopped, referenceTime, false), v1alpha2.AlwaysOffPolicy))

			Expect(conditionMessage(pool, vmpoolcondition.TypeAvailable)).To(Equal("1 of 1 replica is stopped. The run policy is AlwaysOff: change the run policy to start it."))
		})

		It("treats a stopped replica under AlwaysOn as a problem and names it", func() {
			pool := newPool(2)
			handle(pool,
				withRunPolicy(newMemberVM(pool, "web-a", v1alpha2.MachineRunning, referenceTime, false), v1alpha2.AlwaysOnPolicy),
				withRunPolicy(newMemberVM(pool, "web-b", v1alpha2.MachineStopped, referenceTime, false), v1alpha2.AlwaysOnPolicy),
			)

			Expect(conditionReason(pool, vmpoolcondition.TypeAvailable)).To(Equal(vmpoolcondition.ReasonInsufficientReadyReplicas.String()))
			Expect(conditionMessage(pool, vmpoolcondition.TypeAvailable)).To(Equal(`1 of 2 replicas are ready: 1 Stopped. VirtualMachine "web-b" is Stopped.`))
		})

		It("does not report ReplicasStopped while replicas are still missing", func() {
			pool := newPool(2)
			handle(pool, withRunPolicy(newMemberVM(pool, "web-a", v1alpha2.MachineStopped, referenceTime, false), v1alpha2.ManualPolicy))

			Expect(conditionReason(pool, vmpoolcondition.TypeAvailable)).To(Equal(vmpoolcondition.ReasonInsufficientReadyReplicas.String()))
			Expect(conditionMessage(pool, vmpoolcondition.TypeAvailable)).To(Equal("0 of 2 replicas are ready: 1 not created yet, 1 Stopped."))
		})

		It("breaks not ready replicas down by phase and names one with its Running reason", func() {
			pool := newPool(3)
			pending := newMemberVM(pool, "web-b", v1alpha2.MachinePending, referenceTime, false)
			pending.Status.Conditions = []metav1.Condition{{Type: vmcondition.TypeRunning.String(), Status: metav1.ConditionFalse, Reason: "PodNotStarted"}}
			handle(pool,
				newMemberVM(pool, "web-a", v1alpha2.MachineRunning, referenceTime, false),
				pending,
				withRunPolicy(newMemberVM(pool, "web-c", v1alpha2.MachineStopped, referenceTime, false), v1alpha2.ManualPolicy),
			)

			Expect(conditionMessage(pool, vmpoolcondition.TypeAvailable)).To(Equal(`1 of 3 replicas are ready: 1 Pending, 1 Stopped. VirtualMachine "web-b" is Pending (PodNotStarted).`))
		})

		It("tells to start the replicas under AlwaysOnUnlessStoppedManually, also while starting", func() {
			pool := newPool(2)
			handle(pool,
				withRunPolicy(newMemberVM(pool, "web-a", v1alpha2.MachineStopped, referenceTime, false), v1alpha2.AlwaysOnUnlessStoppedManually),
				withRunPolicy(newMemberVM(pool, "web-b", v1alpha2.MachineStarting, referenceTime, false), v1alpha2.AlwaysOnUnlessStoppedManually),
			)

			Expect(conditionReason(pool, vmpoolcondition.TypeAvailable)).To(Equal(vmpoolcondition.ReasonReplicasStopped.String()))
			Expect(conditionMessage(pool, vmpoolcondition.TypeAvailable)).To(Equal("2 of 2 replicas are stopped or changing the power state."))
		})

		It("gives a generic hint when the stopped replicas have different run policies", func() {
			pool := newPool(2)
			handle(pool,
				withRunPolicy(newMemberVM(pool, "web-a", v1alpha2.MachineStopped, referenceTime, false), v1alpha2.ManualPolicy),
				withRunPolicy(newMemberVM(pool, "web-b", v1alpha2.MachineStopped, referenceTime, false), v1alpha2.AlwaysOffPolicy),
			)

			Expect(conditionMessage(pool, vmpoolcondition.TypeAvailable)).To(Equal("2 of 2 replicas are stopped. Start them to restore the pool capacity."))
		})

		It("is available for a pool scaled to zero and keeps zero counters", func() {
			pool := newPool(0)
			handle(pool)

			Expect(pool.Status.Replicas).To(BeZero())
			Expect(pool.Status.ReadyReplicas).To(BeZero())
			Expect(conditionReason(pool, vmpoolcondition.TypeAvailable)).To(Equal(vmpoolcondition.ReasonAllReplicasReady.String()))
			Expect(meta.FindStatusCondition(pool.Status.Conditions, vmpoolcondition.TypeProgressing.String())).To(BeNil())
		})

		It("labels a replica that has no phase yet", func() {
			pool := newPool(1)
			handle(pool, newMemberVM(pool, "web-a", "", referenceTime, false))

			Expect(conditionMessage(pool, vmpoolcondition.TypeAvailable)).To(Equal(`0 of 1 replicas are ready: 1 without a phase. VirtualMachine "web-a" has no phase yet.`))
		})

		It("shows a replica stuck in Terminating", func() {
			pool := newPool(1)
			handle(pool, newMemberVM(pool, "web-a", v1alpha2.MachineRunning, referenceTime, true))

			Expect(conditionMessage(pool, vmpoolcondition.TypeAvailable)).To(Equal(`0 of 1 replicas are ready: 1 Terminating. VirtualMachine "web-a" is Running.`))
		})
	})

	Context("progressing", func() {
		It("reports a creation failure recorded in this pass", func() {
			pool := newPool(2)
			rctx := WithReport(ctx)
			ReportFrom(rctx).ScaleAttempted()
			ReportFrom(rctx).CreationFailed("Cannot create a VirtualMachine from the template: boom.")
			handleIn(rctx, pool)

			c := meta.FindStatusCondition(pool.Status.Conditions, vmpoolcondition.TypeProgressing.String())
			Expect(c.Status).To(Equal(metav1.ConditionFalse))
			Expect(c.Reason).To(Equal(vmpoolcondition.ReasonReplicaCreationFailed.String()))
			Expect(c.Message).To(Equal("Scaling up: 0 of 2 replicas are created. Cannot create a VirtualMachine from the template: boom."))
		})

		It("keeps the last failure while the sync handler only waited", func() {
			pool := newPool(2)
			setCondition(pool, vmpoolcondition.TypeProgressing, metav1.ConditionFalse, vmpoolcondition.ReasonReplicaCreationFailed.String(), "old")
			handleIn(WithReport(ctx), pool)

			Expect(conditionReason(pool, vmpoolcondition.TypeProgressing)).To(Equal(vmpoolcondition.ReasonReplicaCreationFailed.String()))
		})

		It("keeps a deletion failure too while the sync handler only waited", func() {
			pool := newPool(1)
			setCondition(pool, vmpoolcondition.TypeProgressing, metav1.ConditionFalse, vmpoolcondition.ReasonReplicaDeletionFailed.String(), "old")
			handleIn(WithReport(ctx), pool,
				newMemberVM(pool, "web-a", v1alpha2.MachineRunning, referenceTime, false),
				newMemberVM(pool, "web-b", v1alpha2.MachineRunning, referenceTime, false),
			)

			Expect(conditionReason(pool, vmpoolcondition.TypeProgressing)).To(Equal(vmpoolcondition.ReasonReplicaDeletionFailed.String()))
		})

		It("clears the failure once the sync handler tried again without one", func() {
			pool := newPool(2)
			setCondition(pool, vmpoolcondition.TypeProgressing, metav1.ConditionFalse, vmpoolcondition.ReasonReplicaCreationFailed.String(), "old")
			rctx := WithReport(ctx)
			ReportFrom(rctx).ScaleAttempted()
			handleIn(rctx, pool)

			Expect(conditionReason(pool, vmpoolcondition.TypeProgressing)).To(Equal(vmpoolcondition.ReasonReplicasProgressing.String()))
			Expect(conditionMessage(pool, vmpoolcondition.TypeProgressing)).To(Equal("Scaling up: 0 of 2 replicas are created."))
		})

		It("reports scaling down with the numbers", func() {
			pool := newPool(1)
			rctx := WithReport(ctx)
			ReportFrom(rctx).ScaleAttempted()
			handleIn(rctx, pool,
				newMemberVM(pool, "web-a", v1alpha2.MachineRunning, referenceTime, false),
				newMemberVM(pool, "web-b", v1alpha2.MachineRunning, referenceTime, true),
			)

			Expect(conditionMessage(pool, vmpoolcondition.TypeProgressing)).To(Equal("Scaling down: 2 replicas exist, 1 is requested."))
		})

		It("reports a deletion failure recorded in this pass", func() {
			pool := newPool(1)
			rctx := WithReport(ctx)
			ReportFrom(rctx).ScaleAttempted()
			ReportFrom(rctx).DeletionFailed(`Cannot delete VirtualMachine "web-b": access is denied. Contact your administrator.`)
			handleIn(rctx, pool,
				newMemberVM(pool, "web-a", v1alpha2.MachineRunning, referenceTime, false),
				newMemberVM(pool, "web-b", v1alpha2.MachineRunning, referenceTime, false),
			)

			Expect(conditionReason(pool, vmpoolcondition.TypeProgressing)).To(Equal(vmpoolcondition.ReasonReplicaDeletionFailed.String()))
			Expect(conditionMessage(pool, vmpoolcondition.TypeProgressing)).To(Equal(`Scaling down: 2 replicas exist, 1 is requested. Cannot delete VirtualMachine "web-b": access is denied. Contact your administrator.`))
		})

		It("puts a failure above the success of the same pass", func() {
			pool := newPool(3)
			rctx := WithReport(ctx)
			ReportFrom(rctx).ScaleAttempted()
			ReportFrom(rctx).CreationFailed("Cannot create a VirtualMachine from the template: boom.")
			handleIn(rctx, pool, newMemberVM(pool, "web-a", v1alpha2.MachineRunning, referenceTime, false))

			Expect(conditionReason(pool, vmpoolcondition.TypeProgressing)).To(Equal(vmpoolcondition.ReasonReplicaCreationFailed.String()))
			Expect(conditionMessage(pool, vmpoolcondition.TypeProgressing)).To(HavePrefix("Scaling up: 1 of 3 replicas are created."))
		})

		It("reports ScaleDownBlocked under Explicit with surplus replicas", func() {
			pool := newPool(1)
			pool.Spec.ScaleDownPolicy = v1alpha2.ScaleDownPolicyExplicit
			handle(pool,
				newMemberVM(pool, "web-a", v1alpha2.MachineRunning, referenceTime, false),
				newMemberVM(pool, "web-b", v1alpha2.MachineRunning, referenceTime, false),
			)

			Expect(conditionReason(pool, vmpoolcondition.TypeProgressing)).To(Equal(vmpoolcondition.ReasonScaleDownBlocked.String()))
			Expect(conditionMessage(pool, vmpoolcondition.TypeProgressing)).To(Equal("2 replicas exist, 1 is requested. The Explicit scale-down policy removes replicas only by name: use the scaleDownWith subresource."))
		})

		It("does not block when the surplus is already Terminating under Explicit", func() {
			pool := newPool(1)
			pool.Spec.ScaleDownPolicy = v1alpha2.ScaleDownPolicyExplicit
			handle(pool,
				newMemberVM(pool, "web-a", v1alpha2.MachineRunning, referenceTime, false),
				newMemberVM(pool, "web-b", v1alpha2.MachineRunning, referenceTime, true),
			)

			Expect(conditionReason(pool, vmpoolcondition.TypeProgressing)).To(Equal(vmpoolcondition.ReasonReplicasProgressing.String()))
		})

		It("keeps Progressing while a replica waits for its first disk", func() {
			pool := newPool(1)
			pool.Spec.VirtualDiskTemplates = []v1alpha2.VirtualDiskTemplateSpec{diskTemplate("root", v1alpha2.VirtualDiskReclaimDelete)}
			handle(pool, withDiskRefs(newMemberVM(pool, "web-a", v1alpha2.MachinePending, referenceTime, false), "root"))

			Expect(conditionMessage(pool, vmpoolcondition.TypeProgressing)).To(Equal("Scaling up: 0 of 1 replicas are created."))
			// Not a rollout matter: the replica is not created yet.
			Expect(conditionReason(pool, vmpoolcondition.TypeSynced)).To(Equal(vmpoolcondition.ReasonPoolSynced.String()))
		})
	})

	Context("synced", func() {
		It("is not synced while a replica lacks the disk of a new template", func() {
			pool := newPool(1)
			pool.Spec.VirtualDiskTemplates = []v1alpha2.VirtualDiskTemplateSpec{
				diskTemplate("root", v1alpha2.VirtualDiskReclaimDelete),
				diskTemplate("data", v1alpha2.VirtualDiskReclaimDelete),
			}
			vm := withDiskRefs(newMemberVM(pool, "web-a", v1alpha2.MachineRunning, referenceTime, false), "web-a-root")
			vm.Labels[poollabels.TemplateHash] = poollabels.ComputeTemplateHash(pool)
			handle(pool, vm, labeledDisk(pool, "web-a-root", "root"))

			Expect(pool.Status.UpdatedReplicas).To(Equal(int32(0)))
			Expect(conditionReason(pool, vmpoolcondition.TypeSynced)).To(Equal(vmpoolcondition.ReasonRolloutInProgress.String()))
		})

		It("is not synced while a replica keeps the disk of a removed template", func() {
			pool := newPool(1)
			pool.Spec.VirtualDiskTemplates = []v1alpha2.VirtualDiskTemplateSpec{diskTemplate("root", v1alpha2.VirtualDiskReclaimDelete)}
			vm := withDiskRefs(newMemberVM(pool, "web-a", v1alpha2.MachineRunning, referenceTime, false), "web-a-root", "web-a-old")
			vm.Labels[poollabels.TemplateHash] = poollabels.ComputeTemplateHash(pool)
			handle(pool, vm, labeledDisk(pool, "web-a-root", "root"), labeledDisk(pool, "web-a-old", "old"))

			Expect(conditionReason(pool, vmpoolcondition.TypeSynced)).To(Equal(vmpoolcondition.ReasonRolloutInProgress.String()))
		})

		It("reports RestartPendingApproval even when the replica carries the current hash", func() {
			pool := newPool(1)
			hash := poollabels.ComputeTemplateHash(pool)
			vm := newMemberVM(pool, "web-a", v1alpha2.MachineRunning, referenceTime, false)
			vm.Labels[poollabels.TemplateHash] = hash
			vm.Annotations = map[string]string{poollabels.PatchedTemplateHash: hash}
			vm.Status.Conditions = []metav1.Condition{{Type: vmcondition.TypeAwaitingRestartToApplyConfiguration.String(), Status: metav1.ConditionTrue, Reason: "RestartRequired"}}
			handle(pool, vm)

			Expect(conditionReason(pool, vmpoolcondition.TypeSynced)).To(Equal(vmpoolcondition.ReasonRestartPendingApproval.String()))
		})

		It("names both the failure and the pending restart", func() {
			pool := newPool(2)
			hash := poollabels.ComputeTemplateHash(pool)
			waiting := newMemberVM(pool, "web-a", v1alpha2.MachineRunning, referenceTime, false)
			waiting.Annotations = map[string]string{poollabels.PatchedTemplateHash: hash}
			waiting.Status.Conditions = []metav1.Condition{{Type: vmcondition.TypeAwaitingRestartToApplyConfiguration.String(), Status: metav1.ConditionTrue, Reason: "RestartRequired"}}
			failing := newMemberVM(pool, "web-b", v1alpha2.MachineRunning, referenceTime, false)
			rctx := WithReport(ctx)
			ReportFrom(rctx).UpdateFailed("web-b", `Cannot apply the template to VirtualMachine "web-b": boom.`)
			handleIn(rctx, pool, waiting, failing)

			Expect(conditionReason(pool, vmpoolcondition.TypeSynced)).To(Equal(vmpoolcondition.ReasonReplicaUpdateFailed.String()))
			Expect(conditionMessage(pool, vmpoolcondition.TypeSynced)).To(Equal(`1 of 2 replicas cannot apply the template; 1 of 2 replicas await a restart. Cannot apply the template to VirtualMachine "web-b": boom.`))
		})

		It("reports an update failure recorded in this pass", func() {
			pool := newPool(1)
			vm := newMemberVM(pool, "web-a", v1alpha2.MachineRunning, referenceTime, false)
			vm.Labels[poollabels.TemplateHash] = "stale"
			rctx := WithReport(ctx)
			ReportFrom(rctx).UpdateFailed("web-a", `Cannot apply the template to VirtualMachine "web-a": boom.`)
			handleIn(rctx, pool, vm)

			c := meta.FindStatusCondition(pool.Status.Conditions, vmpoolcondition.TypeSynced.String())
			Expect(c.Reason).To(Equal(vmpoolcondition.ReasonReplicaUpdateFailed.String()))
			Expect(c.Message).To(Equal(`1 of 1 replicas cannot apply the template. Cannot apply the template to VirtualMachine "web-a": boom.`))
		})
	})
})

func conditionReason(pool *v1alpha2.VirtualMachinePool, t vmpoolcondition.Type) string {
	c := meta.FindStatusCondition(pool.Status.Conditions, t.String())
	Expect(c).NotTo(BeNil())
	return c.Reason
}

func conditionMessage(pool *v1alpha2.VirtualMachinePool, t vmpoolcondition.Type) string {
	c := meta.FindStatusCondition(pool.Status.Conditions, t.String())
	Expect(c).NotTo(BeNil())
	return c.Message
}

func withRunPolicy(vm *v1alpha2.VirtualMachine, policy v1alpha2.RunPolicy) *v1alpha2.VirtualMachine {
	vm.Spec.RunPolicy = policy
	return vm
}

func withDiskRefs(vm *v1alpha2.VirtualMachine, names ...string) *v1alpha2.VirtualMachine {
	for _, n := range names {
		vm.Spec.BlockDeviceRefs = append(vm.Spec.BlockDeviceRefs, v1alpha2.BlockDeviceSpecRef{Kind: v1alpha2.DiskDevice, Name: n})
	}
	return vm
}
