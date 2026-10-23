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

// Package hotplugmigration decides when a CPU or memory change of a running virtual machine is
// applied by live migration. The workload updater acts on the decision and the virtual machine
// controller reports it, so both read the same Plan.
package hotplugmigration

import (
	"fmt"
	"slices"
	"strings"
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	virtv1 "kubevirt.io/api/core/v1"

	"github.com/deckhouse/virtualization-controller/pkg/common/annotations"
	commonvmop "github.com/deckhouse/virtualization-controller/pkg/common/vmop"
	"github.com/deckhouse/virtualization-controller/pkg/controller/conditions"
	"github.com/deckhouse/virtualization-controller/pkg/controller/kvbuilder"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/vmopcondition"
)

// Annotation is set on the migrations created for a configuration (with the Key as the value) and
// on the KVVMI, where it holds the Mark.
const Annotation = annotations.AnnVMOPWorkloadUpdateHotplugResourcesSum

// MaxAttempts is how many failed live migrations of the launcher pod are tolerated for a
// configuration before the platform stops trying.
const MaxAttempts = 4

// SettleTimeout is how long the runtime is given to report the configuration as applied after a
// live migration has completed: virt-handler finishes the hotplug on the target only then.
const SettleTimeout = time.Minute

// retryDelays separate an attempt from the previous failure.
var retryDelays = [MaxAttempts - 1]time.Duration{5 * time.Minute, 10 * time.Minute, 15 * time.Minute}

type Phase string

const (
	// PhaseDue means a migration is to be created now.
	PhaseDue Phase = "Due"
	// PhaseMigrating means a migration of the virtual machine is not finished yet.
	PhaseMigrating Phase = "Migrating"
	// PhaseSettling means a migration for the configuration has completed and the runtime is
	// expected to report it as applied soon.
	PhaseSettling Phase = "Settling"
	// PhaseNotApplied means the runtime still reports the configuration pending after a migration
	// was repeated for it.
	PhaseNotApplied Phase = "NotApplied"
	// PhaseRetrying means the last migration failed and the next one waits until Plan.At.
	PhaseRetrying Phase = "Retrying"
	// PhaseExhausted means MaxAttempts migrations have failed.
	PhaseExhausted Phase = "Exhausted"
)

type Input struct {
	Now   time.Time
	VM    *v1alpha2.VirtualMachine
	KVVMI *virtv1.VirtualMachineInstance
	// Pod is the launcher pod the virtual machine runs in. Nil counts every migration.
	Pod   *corev1.Pod
	VMOPs []*v1alpha2.VirtualMachineOperation
}

type Plan struct {
	Phase Phase
	// Key identifies the configuration the runtime is asked to apply.
	Key string
	// Autoscaled reports a change made by the autoscaler alone: such a change never asks for a
	// restart, and its failures count towards one budget whatever the value is.
	Autoscaled bool
	// Mark is the value the KVVMI annotation must hold.
	Mark string
	// RecordMark reports that the KVVMI holds another mark. Nothing is recorded while a migration is
	// unfinished, and a KVVMI without a mark gets none until a migration has produced its launcher pod.
	RecordMark bool
	// Active is the unfinished migration of the virtual machine, if any.
	Active *v1alpha2.VirtualMachineOperation
	// Failed lists the failed migrations counted against the configuration, oldest first.
	Failed []*v1alpha2.VirtualMachineOperation
	// At is when the settling ends or the next attempt is allowed.
	At time.Time
}

// Evaluate decides what to do about the CPU and memory change the KVVMI reports pending.
//
// A configuration the runtime still reports pending after a migration is migrated once more, so a
// node that applies it late costs a migration rather than a restart. The Mark records the pod a
// migration produced, the configuration it was created for and whether the migration was the
// repeated one; the configuration is dropped as soon as the pod is asked for another one.
func Evaluate(in Input) Plan {
	plan := Plan{
		Key:        Key(in.KVVMI),
		Autoscaled: Autoscaled(in.VM, in.KVVMI),
	}

	var podCreated time.Time
	if in.Pod != nil {
		podCreated = in.Pod.GetCreationTimestamp().Time
	}

	ops, active := migrations(in.VMOPs)
	plan.Active = active
	mark, produced := currentMark(in, ops)
	plan.Mark = mark.String()
	held := in.KVVMI.GetAnnotations()[Annotation]
	// The KVVMI reports the target pod before the migration that produced it completes, and the mark
	// of a pod is not recomputed once recorded.
	plan.RecordMark = active == nil && plan.Mark != held && (held != "" || produced != nil)

	if plan.Active != nil {
		plan.Phase = PhaseMigrating
		return plan
	}

	if mark.key != "" {
		var completedAt time.Time
		if produced != nil {
			completedAt = finishedAt(produced)
		}
		settled := settle(plan, completedAt, in.Now)
		if settled.Phase == PhaseSettling || mark.retried {
			return settled
		}
	}

	for _, vmop := range ops {
		// The migration that produced the launcher pod is accounted by the mark. The creation
		// timestamps have a precision of a second, so the pod may carry the same one.
		if vmop == produced || vmop.GetCreationTimestamp().Time.Before(podCreated) {
			continue
		}
		key := vmop.GetAnnotations()[Annotation]
		switch vmop.Status.Phase {
		case v1alpha2.VMOPPhaseCompleted:
			// The migration has completed, but the launcher pod it produced is not seen yet.
			if key == plan.Key {
				return settle(plan, finishedAt(vmop), in.Now)
			}
		case v1alpha2.VMOPPhaseFailed:
			if plan.Autoscaled || key == plan.Key {
				plan.Failed = append(plan.Failed, vmop)
			}
		}
	}

	switch n := len(plan.Failed); {
	case n >= MaxAttempts:
		plan.Phase = PhaseExhausted
	case n > 0 && in.Now.Before(finishedAt(plan.Failed[n-1]).Add(retryDelays[n-1])):
		plan.Phase = PhaseRetrying
		plan.At = finishedAt(plan.Failed[n-1]).Add(retryDelays[n-1])
	default:
		plan.Phase = PhaseDue
	}
	return plan
}

// migrations returns the migrations made for configurations, oldest first, and the unfinished
// migration of the virtual machine, if any.
func migrations(vmops []*v1alpha2.VirtualMachineOperation) ([]*v1alpha2.VirtualMachineOperation, *v1alpha2.VirtualMachineOperation) {
	var (
		ops    []*v1alpha2.VirtualMachineOperation
		active *v1alpha2.VirtualMachineOperation
	)
	for _, vmop := range vmops {
		if !commonvmop.IsMigration(vmop) {
			continue
		}
		if !commonvmop.IsFinished(vmop) {
			active = vmop
		}
		if _, ok := vmop.GetAnnotations()[Annotation]; ok && isManaged(vmop) {
			ops = append(ops, vmop)
		}
	}
	slices.SortFunc(ops, func(a, b *v1alpha2.VirtualMachineOperation) int {
		return a.GetCreationTimestamp().Compare(b.GetCreationTimestamp().Time)
	})
	return ops, active
}

// currentMark returns the mark of the launcher pod and the completed migration that produced it.
// A mark of another pod is replaced: the configuration is taken from the migration that produced
// the pod, and dropped once the pod is asked for another one.
func currentMark(in Input, ops []*v1alpha2.VirtualMachineOperation) (Mark, *v1alpha2.VirtualMachineOperation) {
	var podUID types.UID
	var produced *v1alpha2.VirtualMachineOperation
	if in.Pod != nil {
		podUID = in.Pod.GetUID()
		produced = producedBy(in.Pod.GetCreationTimestamp().Time, ops)
	}

	mark := parseMark(in.KVVMI.GetAnnotations()[Annotation])
	if mark.pod != podUID {
		previous := mark
		mark = Mark{pod: podUID}
		if produced != nil {
			mark.key = produced.GetAnnotations()[Annotation]
			// The previous pod kept the mark of the same configuration, so the runtime did not apply it there.
			mark.retried = previous.pod != "" && mark.key != "" && previous.key == mark.key
		}
	}
	if mark.key != Key(in.KVVMI) {
		mark.key = ""
		mark.retried = false
	}
	return mark, produced
}

func settle(plan Plan, completedAt, now time.Time) Plan {
	plan.At = completedAt.Add(SettleTimeout)
	if now.Before(plan.At) {
		plan.Phase = PhaseSettling
	} else {
		plan.Phase = PhaseNotApplied
	}
	return plan
}

// Key identifies the CPU and memory configuration the KVVMI asks the runtime to apply. It is read
// from the KVVMI rather than the VirtualMachine spec: "Auto" there stands for a different core
// fraction at every autoscaler step, while the KVVMI holds the percentage itself.
func Key(kvvmi *virtv1.VirtualMachineInstance) string {
	var memory string
	if m := kvvmi.Spec.Domain.Memory; m != nil && m.Guest != nil {
		memory = m.Guest.String()
	}
	return fmt.Sprintf("cpu.vcpus=%d,cpu.coreFraction=%s,memory.guest=%s",
		vcpus(kvvmi.Spec.Domain.CPU), kvvmi.GetAnnotations()[kvbuilder.CPUResourcesRequestsFractionAnnotation], memory)
}

// Autoscaled reports whether the pending change comes from the autoscaler alone: the core fraction
// is "Auto" and the number of vCPUs and the guest memory are already applied.
func Autoscaled(vm *v1alpha2.VirtualMachine, kvvmi *virtv1.VirtualMachineInstance) bool {
	if vm == nil || vm.Spec.CPU.CoreFraction != v1alpha2.CoreFractionAuto {
		return false
	}
	if current := kvvmi.Status.CurrentCPUTopology; current != nil && vcpus(kvvmi.Spec.Domain.CPU) != vcpus(&virtv1.CPU{
		Sockets: current.Sockets,
		Cores:   current.Cores,
		Threads: current.Threads,
	}) {
		return false
	}
	if m, s := kvvmi.Spec.Domain.Memory, kvvmi.Status.Memory; m != nil && m.Guest != nil && s != nil && s.GuestRequested != nil {
		return m.Guest.Cmp(*s.GuestRequested) == 0
	}
	return true
}

// Mark is the "<launcher pod UID>/<key>[/retried]" record on the KVVMI: the configuration a live
// migration produced that pod for, or an empty key once the pod has been asked for another one,
// and whether that migration was repeated for a configuration the previous pod did not apply. A
// value of another format, such as the one older versions wrote, belongs to no pod.
type Mark struct {
	pod     types.UID
	key     string
	retried bool
}

const retriedSuffix = "/retried"

func parseMark(s string) Mark {
	pod, key, ok := strings.Cut(s, "/")
	if !ok {
		return Mark{}
	}
	key, retried := strings.CutSuffix(key, retriedSuffix)
	return Mark{pod: types.UID(pod), key: key, retried: retried}
}

func (m Mark) String() string {
	if m.pod == "" {
		return ""
	}
	if m.retried {
		return string(m.pod) + "/" + m.key + retriedSuffix
	}
	return string(m.pod) + "/" + m.key
}

// producedBy returns the completed migration during which the launcher pod was created.
func producedBy(podCreated time.Time, ops []*v1alpha2.VirtualMachineOperation) *v1alpha2.VirtualMachineOperation {
	for i := len(ops) - 1; i >= 0; i-- {
		vmop := ops[i]
		if vmop.Status.Phase != v1alpha2.VMOPPhaseCompleted {
			continue
		}
		if !vmop.GetCreationTimestamp().After(podCreated) && !finishedAt(vmop).Before(podCreated) {
			return vmop
		}
	}
	return nil
}

// finishedAt is when the operation reached its final phase.
func finishedAt(vmop *v1alpha2.VirtualMachineOperation) time.Time {
	if completed, ok := conditions.GetCondition(vmopcondition.TypeCompleted, vmop.Status.Conditions); ok && !completed.LastTransitionTime.IsZero() {
		return completed.LastTransitionTime.Time
	}
	return vmop.GetCreationTimestamp().Time
}

func isManaged(vmop *v1alpha2.VirtualMachineOperation) bool {
	_, ok := vmop.GetAnnotations()[annotations.AnnVMOPWorkloadUpdate]
	return ok
}

func vcpus(cpu *virtv1.CPU) int64 {
	if cpu == nil {
		return 0
	}
	return int64(max(cpu.Sockets, 1)) * int64(max(cpu.Cores, 1)) * int64(max(cpu.Threads, 1))
}
