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

package hotplugmigration

import (
	"testing"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	virtv1 "kubevirt.io/api/core/v1"

	"github.com/deckhouse/virtualization-controller/pkg/common/annotations"
	"github.com/deckhouse/virtualization-controller/pkg/controller/kvbuilder"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/vmopcondition"
)

func TestHotplugMigration(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "HotplugMigration")
}

var _ = Describe("Evaluate", func() {
	t0 := time.Date(2026, 10, 16, 12, 0, 0, 0, time.UTC)

	newVM := func(coreFraction string) *v1alpha2.VirtualMachine {
		vm := &v1alpha2.VirtualMachine{}
		vm.Spec.CPU.Cores = 1
		vm.Spec.CPU.CoreFraction = coreFraction
		return vm
	}

	// newKVVMI returns a KVVMI that runs one vCPU at 10% and asks the runtime for fraction percent.
	newKVVMI := func(fraction string) *virtv1.VirtualMachineInstance {
		return &virtv1.VirtualMachineInstance{
			ObjectMeta: metav1.ObjectMeta{
				Annotations: map[string]string{kvbuilder.CPUResourcesRequestsFractionAnnotation: fraction},
			},
			Spec: virtv1.VirtualMachineInstanceSpec{
				Domain: virtv1.DomainSpec{
					CPU:    &virtv1.CPU{Sockets: 1, Cores: 1, Threads: 1},
					Memory: &virtv1.Memory{Guest: ptrQuantity("1Gi")},
				},
			},
			Status: virtv1.VirtualMachineInstanceStatus{
				CurrentCPUTopology: &virtv1.CPUTopology{Sockets: 1, Cores: 1, Threads: 1},
				Memory:             &virtv1.MemoryStatus{GuestRequested: ptrQuantity("1Gi")},
			},
		}
	}

	newPod := func(uid string, created time.Time) *corev1.Pod {
		return &corev1.Pod{ObjectMeta: metav1.ObjectMeta{UID: types.UID(uid), CreationTimestamp: metav1.NewTime(created)}}
	}

	newVMOP := func(phase v1alpha2.VMOPPhase, key string, created, finished time.Time) *v1alpha2.VirtualMachineOperation {
		vmop := &v1alpha2.VirtualMachineOperation{
			ObjectMeta: metav1.ObjectMeta{
				CreationTimestamp: metav1.NewTime(created),
				Annotations: map[string]string{
					annotations.AnnVMOPWorkloadUpdate: "true",
					Annotation:                        key,
				},
			},
			Spec:   v1alpha2.VirtualMachineOperationSpec{Type: v1alpha2.VMOPTypeEvict},
			Status: v1alpha2.VirtualMachineOperationStatus{Phase: phase},
		}
		if !finished.IsZero() {
			vmop.Status.Conditions = []metav1.Condition{{
				Type:               vmopcondition.TypeCompleted.String(),
				Status:             metav1.ConditionFalse,
				LastTransitionTime: metav1.NewTime(finished),
			}}
		}
		return vmop
	}

	key := func(fraction string) string { return Key(newKVVMI(fraction)) }

	It("builds the key from the KVVMI, so every autoscaler step gets its own", func() {
		Expect(key("30")).To(Equal("cpu.vcpus=1,cpu.coreFraction=30,memory.guest=1Gi"))
		Expect(key("30")).NotTo(Equal(key("50")))
	})

	It("migrates when nothing has been tried for the launcher pod", func() {
		plan := Evaluate(Input{Now: t0, VM: newVM("10%"), KVVMI: newKVVMI("50"), Pod: newPod("pod-1", t0.Add(-time.Hour))})
		Expect(plan.Phase).To(Equal(PhaseDue))
		Expect(plan.Mark).To(Equal("pod-1/"))
	})

	It("waits for any unfinished migration of the virtual machine", func() {
		user := newVMOP(v1alpha2.VMOPPhaseInProgress, "", t0.Add(-time.Minute), time.Time{})
		delete(user.Annotations, annotations.AnnVMOPWorkloadUpdate)
		delete(user.Annotations, Annotation)
		plan := Evaluate(Input{Now: t0, VM: newVM("10%"), KVVMI: newKVVMI("50"), Pod: newPod("pod-1", t0.Add(-time.Hour)), VMOPs: []*v1alpha2.VirtualMachineOperation{user}})
		Expect(plan.Phase).To(Equal(PhaseMigrating))
		Expect(plan.Active).To(Equal(user))
	})

	// In the spec both steps are "Auto"; the key tells them apart.
	It("migrates every autoscaler step that the node turns down", func() {
		first := newVMOP(v1alpha2.VMOPPhaseCompleted, key("30"), t0.Add(-20*time.Minute), t0.Add(-19*time.Minute))
		pod := newPod("pod-2", t0.Add(-19*time.Minute-30*time.Second))
		kvvmi := newKVVMI("50")
		kvvmi.Annotations[Annotation] = "pod-1/" + key("30")

		plan := Evaluate(Input{Now: t0, VM: newVM(string(v1alpha2.CoreFractionAuto)), KVVMI: kvvmi, Pod: pod, VMOPs: []*v1alpha2.VirtualMachineOperation{first}})
		Expect(plan.Phase).To(Equal(PhaseDue))
		Expect(plan.Mark).To(Equal("pod-2/"))
	})

	It("migrates a configuration again after the launcher pod has been asked for another one", func() {
		// 10% -> 50% migrated into pod-2, then 10% applied in place, now 50% again.
		migrated := newVMOP(v1alpha2.VMOPPhaseCompleted, key("50"), t0.Add(-20*time.Minute), t0.Add(-19*time.Minute))
		pod := newPod("pod-2", t0.Add(-19*time.Minute-30*time.Second))

		atTen := newKVVMI("10")
		plan := Evaluate(Input{Now: t0.Add(-10 * time.Minute), KVVMI: atTen, Pod: pod, VMOPs: []*v1alpha2.VirtualMachineOperation{migrated}})
		Expect(plan.Mark).To(Equal("pod-2/"), "the value of another configuration drops the mark")

		atFifty := newKVVMI("50")
		atFifty.Annotations[Annotation] = plan.Mark
		plan = Evaluate(Input{Now: t0, KVVMI: atFifty, Pod: pod, VMOPs: []*v1alpha2.VirtualMachineOperation{migrated}})
		Expect(plan.Phase).To(Equal(PhaseDue))
	})

	It("migrates a configuration again into the launcher pod created in the same second as its migration", func() {
		// 10% -> 50% migrated into pod-2, 10% applied in place, now 50% again on a node without room.
		created := t0.Add(-20 * time.Minute)
		migrated := newVMOP(v1alpha2.VMOPPhaseCompleted, key("50"), created, created.Add(15*time.Second))
		pod := newPod("pod-2", created)
		kvvmi := newKVVMI("50")
		kvvmi.Annotations[Annotation] = "pod-2/"

		plan := Evaluate(Input{Now: t0, KVVMI: kvvmi, Pod: pod, VMOPs: []*v1alpha2.VirtualMachineOperation{migrated}})
		Expect(plan.Phase).To(Equal(PhaseDue))
		Expect(plan.Mark).To(Equal("pod-2/"))
	})

	Describe("the mark to record", func() {
		It("is not recorded for a launcher pod no migration has produced", func() {
			plan := Evaluate(Input{Now: t0, KVVMI: newKVVMI("50"), Pod: newPod("pod-1", t0.Add(-time.Hour))})
			Expect(plan.RecordMark).To(BeFalse())
		})

		It("replaces the mark of the previous launcher pod once another configuration is asked for in place", func() {
			// 10% migrated into pod-2, the KVVMI asks for 50% now.
			created := t0.Add(-20 * time.Minute)
			migrated := newVMOP(v1alpha2.VMOPPhaseCompleted, key("10"), created, created.Add(15*time.Second))
			kvvmi := newKVVMI("50")
			kvvmi.Annotations[Annotation] = "pod-1/"
			plan := Evaluate(Input{Now: t0, KVVMI: kvvmi, Pod: newPod("pod-2", created), VMOPs: []*v1alpha2.VirtualMachineOperation{migrated}})
			Expect(plan.RecordMark).To(BeTrue())
			Expect(plan.Mark).To(Equal("pod-2/"))
		})

		It("is recorded for a launcher pod a migration produced, even if no mark was held", func() {
			created := t0.Add(-20 * time.Minute)
			migrated := newVMOP(v1alpha2.VMOPPhaseCompleted, key("10"), created, created.Add(15*time.Second))
			plan := Evaluate(Input{Now: t0, KVVMI: newKVVMI("50"), Pod: newPod("pod-2", created), VMOPs: []*v1alpha2.VirtualMachineOperation{migrated}})
			Expect(plan.RecordMark).To(BeTrue())
			Expect(plan.Mark).To(Equal("pod-2/"))
		})

		It("waits for the migration that switched the virtual machine to the launcher pod to complete", func() {
			// The KVVMI reports the target pod a few seconds before the operation completes.
			created := t0.Add(-20 * time.Second)
			migration := newVMOP(v1alpha2.VMOPPhaseInProgress, key("50"), created, time.Time{})
			kvvmi := newKVVMI("50")
			kvvmi.Annotations[Annotation] = "pod-1/"
			pod := newPod("pod-2", created)

			plan := Evaluate(Input{Now: t0, KVVMI: kvvmi, Pod: pod, VMOPs: []*v1alpha2.VirtualMachineOperation{migration}})
			Expect(plan.RecordMark).To(BeFalse())

			migration.Status.Phase = v1alpha2.VMOPPhaseCompleted
			migration.Status.Conditions = []metav1.Condition{{Type: vmopcondition.TypeCompleted.String(), LastTransitionTime: metav1.NewTime(t0)}}
			plan = Evaluate(Input{Now: t0, KVVMI: kvvmi, Pod: pod, VMOPs: []*v1alpha2.VirtualMachineOperation{migration}})
			Expect(plan.RecordMark).To(BeTrue())
			Expect(plan.Mark).To(Equal("pod-2/" + key("50")))
		})
	})

	Context("when a migration for the configuration produced the launcher pod", func() {
		var (
			migrated *v1alpha2.VirtualMachineOperation
			pod      *corev1.Pod
		)

		BeforeEach(func() {
			migrated = newVMOP(v1alpha2.VMOPPhaseCompleted, key("50"), t0.Add(-2*time.Minute), t0.Add(-30*time.Second))
			pod = newPod("pod-2", t0.Add(-time.Minute))
		})

		It("gives the runtime time to report the configuration as applied", func() {
			plan := Evaluate(Input{Now: t0, KVVMI: newKVVMI("50"), Pod: pod, VMOPs: []*v1alpha2.VirtualMachineOperation{migrated}})
			Expect(plan.Phase).To(Equal(PhaseSettling))
			Expect(plan.At).To(Equal(t0.Add(30 * time.Second)))
			Expect(plan.Mark).To(Equal("pod-2/" + key("50")))
		})

		It("migrates once more when the runtime keeps the configuration pending", func() {
			kvvmi := newKVVMI("50")
			kvvmi.Annotations[Annotation] = "pod-2/" + key("50")
			plan := Evaluate(Input{Now: t0.Add(time.Minute), KVVMI: kvvmi, Pod: pod, VMOPs: []*v1alpha2.VirtualMachineOperation{migrated}})
			Expect(plan.Phase).To(Equal(PhaseDue))
			Expect(plan.RecordMark).To(BeFalse())
		})

		It("backs off the repeated migration when it fails", func() {
			kvvmi := newKVVMI("50")
			kvvmi.Annotations[Annotation] = "pod-2/" + key("50")
			retry := newVMOP(v1alpha2.VMOPPhaseFailed, key("50"), t0.Add(30*time.Second), t0.Add(time.Minute))
			plan := Evaluate(Input{Now: t0.Add(2 * time.Minute), KVVMI: kvvmi, Pod: pod, VMOPs: []*v1alpha2.VirtualMachineOperation{migrated, retry}})
			Expect(plan.Phase).To(Equal(PhaseRetrying))
			Expect(plan.At).To(Equal(t0.Add(6 * time.Minute)))
		})

		It("does not migrate a third time into the pod the repeated migration produced", func() {
			kvvmi := newKVVMI("50")
			kvvmi.Annotations[Annotation] = "pod-2/" + key("50") + "/retried"
			plan := Evaluate(Input{Now: t0.Add(time.Minute), KVVMI: kvvmi, Pod: pod, VMOPs: []*v1alpha2.VirtualMachineOperation{migrated}})
			Expect(plan.Phase).To(Equal(PhaseNotApplied))
			Expect(plan.RecordMark).To(BeFalse())
		})

		It("still does not migrate a third time once the operation is garbage collected", func() {
			kvvmi := newKVVMI("50")
			kvvmi.Annotations[Annotation] = "pod-2/" + key("50") + "/retried"
			plan := Evaluate(Input{Now: t0.Add(25 * time.Hour), KVVMI: kvvmi, Pod: pod})
			Expect(plan.Phase).To(Equal(PhaseNotApplied))
		})

		It("marks the pod as produced by the repeated migration when the previous pod kept the same configuration", func() {
			kvvmi := newKVVMI("50")
			kvvmi.Annotations[Annotation] = "pod-1/" + key("50")
			plan := Evaluate(Input{Now: t0, KVVMI: kvvmi, Pod: pod, VMOPs: []*v1alpha2.VirtualMachineOperation{migrated}})
			Expect(plan.Mark).To(Equal("pod-2/" + key("50") + "/retried"))
			Expect(plan.RecordMark).To(BeTrue())
			Expect(plan.Phase).To(Equal(PhaseSettling))
		})

		It("does not take a migration for a new configuration for a repeated one", func() {
			kvvmi := newKVVMI("50")
			kvvmi.Annotations[Annotation] = "pod-1/" + key("30")
			plan := Evaluate(Input{Now: t0, KVVMI: kvvmi, Pod: pod, VMOPs: []*v1alpha2.VirtualMachineOperation{migrated}})
			Expect(plan.Mark).To(Equal("pod-2/" + key("50")))
		})

		It("drops the repeated mark once the pod is asked for another configuration", func() {
			kvvmi := newKVVMI("30")
			kvvmi.Annotations[Annotation] = "pod-2/" + key("50") + "/retried"
			plan := Evaluate(Input{Now: t0, KVVMI: kvvmi, Pod: pod, VMOPs: []*v1alpha2.VirtualMachineOperation{migrated}})
			Expect(plan.Mark).To(Equal("pod-2/"))
			Expect(plan.Phase).To(Equal(PhaseDue))
		})
	})

	It("does not take a migration started by the user for one made for the configuration", func() {
		user := newVMOP(v1alpha2.VMOPPhaseCompleted, "", t0.Add(-20*time.Minute), t0.Add(-19*time.Minute))
		delete(user.Annotations, annotations.AnnVMOPWorkloadUpdate)
		delete(user.Annotations, Annotation)
		pod := newPod("pod-2", t0.Add(-19*time.Minute-30*time.Second))
		plan := Evaluate(Input{Now: t0, KVVMI: newKVVMI("50"), Pod: pod, VMOPs: []*v1alpha2.VirtualMachineOperation{user}})
		Expect(plan.Phase).To(Equal(PhaseDue))
		Expect(plan.Mark).To(Equal("pod-2/"))
	})

	It("waits for the cache to show the launcher pod a completed migration produced", func() {
		completed := newVMOP(v1alpha2.VMOPPhaseCompleted, key("50"), t0.Add(-time.Minute), t0.Add(-10*time.Second))
		source := newPod("pod-1", t0.Add(-time.Hour))
		plan := Evaluate(Input{Now: t0, KVVMI: newKVVMI("50"), Pod: source, VMOPs: []*v1alpha2.VirtualMachineOperation{completed}})
		Expect(plan.Phase).To(Equal(PhaseSettling))
	})

	Describe("failed migrations", func() {
		pod := newPod("pod-1", t0.Add(-time.Hour))

		failures := func(keys ...string) []*v1alpha2.VirtualMachineOperation {
			var vmops []*v1alpha2.VirtualMachineOperation
			for i, k := range keys {
				finished := t0.Add(time.Duration(i-len(keys)) * time.Minute)
				vmops = append(vmops, newVMOP(v1alpha2.VMOPPhaseFailed, k, finished.Add(-30*time.Second), finished))
			}
			return vmops
		}

		DescribeTable("space the attempts out",
			func(failed int, after time.Duration, expected Phase) {
				var keys []string
				for range failed {
					keys = append(keys, key("50"))
				}
				vmops := failures(keys...)
				last := vmops[len(vmops)-1]
				now := finishedAt(last).Add(after)
				plan := Evaluate(Input{Now: now, KVVMI: newKVVMI("50"), Pod: pod, VMOPs: vmops})
				Expect(plan.Phase).To(Equal(expected))
				Expect(plan.Failed).To(HaveLen(failed))
				if expected == PhaseRetrying {
					Expect(plan.At).To(Equal(finishedAt(last).Add(retryDelays[failed-1])))
				}
			},
			Entry("the second attempt waits 5 minutes", 1, 4*time.Minute, PhaseRetrying),
			Entry("the second attempt follows after 5 minutes", 1, 5*time.Minute, PhaseDue),
			Entry("the third attempt waits 10 minutes", 2, 9*time.Minute, PhaseRetrying),
			Entry("the third attempt follows after 10 minutes", 2, 10*time.Minute, PhaseDue),
			Entry("the fourth attempt waits 15 minutes", 3, 14*time.Minute, PhaseRetrying),
			Entry("the fourth attempt follows after 15 minutes", 3, 15*time.Minute, PhaseDue),
			Entry("the fourth failure ends the attempts", 4, time.Hour, PhaseExhausted),
		)

		It("starts over for a configuration changed by the user", func() {
			plan := Evaluate(Input{Now: t0, VM: newVM("10%"), KVVMI: newKVVMI("50"), Pod: pod, VMOPs: failures(key("20"), key("20"), key("20"), key("20"))})
			Expect(plan.Phase).To(Equal(PhaseDue))
			Expect(plan.Failed).To(BeEmpty())
		})

		It("keeps one budget for the autoscaler steps, so a hovering autoscaler does not migrate again and again", func() {
			plan := Evaluate(Input{Now: t0, VM: newVM(string(v1alpha2.CoreFractionAuto)), KVVMI: newKVVMI("50"), Pod: pod, VMOPs: failures(key("20"), key("50"), key("20"), key("50"))})
			Expect(plan.Autoscaled).To(BeTrue())
			Expect(plan.Phase).To(Equal(PhaseExhausted))
		})

		It("forgets the failures of the previous launcher pod", func() {
			newPodAfterRestart := newPod("pod-2", t0.Add(-time.Second))
			plan := Evaluate(Input{Now: t0, KVVMI: newKVVMI("50"), Pod: newPodAfterRestart, VMOPs: failures(key("50"), key("50"), key("50"), key("50"))})
			Expect(plan.Phase).To(Equal(PhaseDue))
		})
	})

	It("ignores a mark of the format older versions wrote", func() {
		kvvmi := newKVVMI("50")
		kvvmi.Annotations[Annotation] = "cpu.cores=1,cpu.coreFraction=Auto,memory.size=1Gi"
		plan := Evaluate(Input{Now: t0, KVVMI: kvvmi, Pod: newPod("pod-1", t0.Add(-time.Hour))})
		Expect(plan.Phase).To(Equal(PhaseDue))
		Expect(plan.Mark).To(Equal("pod-1/"))
	})

	DescribeTable("tells the autoscaler step from a change made by the user",
		func(coreFraction string, mutate func(*virtv1.VirtualMachineInstance), expected bool) {
			kvvmi := newKVVMI("50")
			if mutate != nil {
				mutate(kvvmi)
			}
			Expect(Autoscaled(newVM(coreFraction), kvvmi)).To(Equal(expected))
		},
		Entry("only the core fraction of an Auto VM is pending", string(v1alpha2.CoreFractionAuto), nil, true),
		Entry("the core fraction is explicit", "50%", nil, false),
		Entry("the number of vCPUs is pending too", string(v1alpha2.CoreFractionAuto), func(kvvmi *virtv1.VirtualMachineInstance) {
			kvvmi.Spec.Domain.CPU.Sockets = 2
		}, false),
		Entry("the memory is pending too", string(v1alpha2.CoreFractionAuto), func(kvvmi *virtv1.VirtualMachineInstance) {
			kvvmi.Spec.Domain.Memory.Guest = ptrQuantity("2Gi")
		}, false),
	)
})

func ptrQuantity(s string) *resource.Quantity {
	q := resource.MustParse(s)
	return &q
}
