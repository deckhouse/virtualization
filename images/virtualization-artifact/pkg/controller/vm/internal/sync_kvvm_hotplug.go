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

package internal

import (
	"context"
	"fmt"
	"time"

	corev1 "k8s.io/api/core/v1"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	virtv1 "kubevirt.io/api/core/v1"

	sizingpolicy "github.com/deckhouse/virtualization-controller/pkg/common/sizing_policy"
	"github.com/deckhouse/virtualization-controller/pkg/controller/conditions"
	"github.com/deckhouse/virtualization-controller/pkg/controller/kvbuilder"
	"github.com/deckhouse/virtualization-controller/pkg/controller/service/hotplugmigration"
	"github.com/deckhouse/virtualization-controller/pkg/controller/vm/internal/state"
	"github.com/deckhouse/virtualization-controller/pkg/controller/vmchange"
	"github.com/deckhouse/virtualization-controller/pkg/featuregates"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/vmopcondition"
)

// hotplugState describes CPU and memory changes accepted by a running virtual machine that its
// guest has not received yet.
type hotplugState struct {
	cpu    bool
	memory bool
	// autoscaled marks a change made by the autoscaler alone: it never asks for a restart. The
	// user sees only "Auto" in the spec, so the messages name the core fraction the autoscaler
	// selected and the one the virtual machine runs with, when it is known.
	autoscaled bool
	selected   string
	running    string

	// message goes to the condition; details is the node's text and goes to the event only.
	message string
	details string
	// warning marks changes that are not making progress on their own.
	warning bool
	// restartRequired marks changes that will not reach the guest without a restart.
	restartRequired bool
}

// giveUp ends the message for changes the platform stops applying on its own: a change made by
// the user asks for a restart, an autoscaler step leaves the virtual machine as it is.
func (st *hotplugState) giveUp() {
	switch {
	case !st.autoscaled:
		st.restartRequired = true
		st.message += " Restart the virtual machine to apply the changes."
	case st.running != "":
		st.message += fmt.Sprintf(" The core fraction stays at %s.", st.running)
	default:
		st.message += " The core fraction stays as it is."
	}
}

// introduceAutoscalerStep names the autoscaler step the message is about.
func (st *hotplugState) introduceAutoscalerStep() {
	if st.selected != "" {
		st.message = fmt.Sprintf("The autoscaler has selected a CPU core fraction of %s. ", st.selected) + st.message
	}
}

// resources names the changed resources as used in messages.
func (st *hotplugState) resources() string {
	switch {
	case st.cpu && st.memory:
		return "CPU and memory"
	case st.cpu:
		return "CPU"
	default:
		return "memory"
	}
}

func (st *hotplugState) eventReason() string {
	switch {
	case st.cpu && st.memory:
		return v1alpha2.ReasonVMCPUAndMemoryResizing
	case st.cpu:
		return v1alpha2.ReasonVMCPUResizing
	default:
		return v1alpha2.ReasonVMMemoryResizing
	}
}

func (st *hotplugState) eventType() string {
	if st.warning {
		return corev1.EventTypeWarning
	}
	return corev1.EventTypeNormal
}

func (st *hotplugState) eventMessage() string {
	if st.details == "" {
		return st.message
	}
	return st.message + " " + st.details
}

// observeHotplug reports the CPU and memory changes the running virtual machine still has to
// receive, or nil when there are none. The KVVMI keeps a condition per resource for as long as
// the guest runs the old value.
func (h *SyncKvvmHandler) observeHotplug(ctx context.Context, s state.VirtualMachineState, vm *v1alpha2.VirtualMachine, kvvmi *virtv1.VirtualMachineInstance) (*hotplugState, error) {
	if kvvmi == nil {
		return nil, nil
	}

	st := &hotplugState{
		cpu:    h.inplaceResize.CPUChange(kvvmi),
		memory: h.inplaceResize.MemoryChange(kvvmi),
	}
	if !st.cpu && !st.memory {
		return nil, nil
	}

	vmops, err := s.VMOPs(ctx)
	if err != nil {
		return nil, err
	}
	pod, err := s.Pod(ctx)
	if err != nil {
		return nil, err
	}
	plan := hotplugmigration.Evaluate(hotplugmigration.Input{Now: time.Now(), VM: vm, KVVMI: kvvmi, Pod: pod, VMOPs: vmops})
	if plan.Autoscaled {
		st.autoscaled = true
		st.selected = sizingpolicy.RecommendedCoreFraction(vm)
		if pod != nil {
			if fraction, ok := pod.GetAnnotations()[kvbuilder.CPUResourcesRequestsFractionAnnotation]; ok {
				st.running = fraction + "%"
			}
		}
		defer st.introduceAutoscalerStep()
	}

	// A migration created for this configuration supersedes whatever the in-place attempt said.
	if describeHotplugMigrations(st, plan) {
		return st, nil
	}

	var refusal string
	if h.inplaceResize.InProgress(kvvmi) {
		inProgress, refused, err := h.describeInplaceResize(ctx, st, kvvmi, vm.Spec.CPU.CoreFraction)
		if err != nil {
			return nil, err
		}
		if inProgress {
			return st, nil
		}
		refusal = refused
	}

	h.describeHotplugMigrationPlan(st, vm, refusal)
	return st, nil
}

// describeInplaceResize fills st for an in-place resize that is still going and reports true.
// For a resize the node has turned down it reports false and the sentence that says why.
func (h *SyncKvvmHandler) describeInplaceResize(ctx context.Context, st *hotplugState, kvvmi *virtv1.VirtualMachineInstance, coreFraction string) (bool, string, error) {
	resources := st.resources()

	cond := h.inplaceResize.ResizeCondition(kvvmi)
	switch cond.Reason {
	case virtv1.VirtualMachineInstanceReasonPodResizeCompleted:
		st.message = fmt.Sprintf("The %s changes are being applied to the virtual machine.", resources)
		return true, "", nil
	case "":
		// Either the resize has just started, or it was rejected outright and the condition never comes.
		st.message = fmt.Sprintf("The %s changes are accepted; waiting for the node to confirm the in-place change.", resources)
		return true, "", nil
	case virtv1.VirtualMachineInstanceReasonPodResizePending, virtv1.VirtualMachineInstanceReasonPodResizeInProgress:
	default:
		st.message = fmt.Sprintf("The %s changes are being applied in place.", resources)
		st.details = fmt.Sprintf("The node reports %s: %s", cond.Reason, cond.Message)
		return true, "", nil
	}

	rejection, err := h.inplaceResize.PodResizeRejection(ctx, kvvmi)
	if err != nil {
		return false, "", fmt.Errorf("check whether the node turned the in-place change down: %w", err)
	}
	if rejection != "" && cond.Message != "" {
		st.details = "The node reports: " + cond.Message
	}

	switch rejection {
	case "":
		st.message = fmt.Sprintf("The %s changes are being applied in place.", resources)
		return true, "", nil
	case corev1.PodReasonInfeasible:
		// The known case: coreFraction 100% pins the machine to dedicated cores on a node with
		// the static CPU manager policy.
		if isCoreFractionGuaranteed(coreFraction) {
			return false, fmt.Sprintf("The node cannot apply the %s changes in place to a virtual machine with coreFraction 100%%.", resources), nil
		}
		return false, fmt.Sprintf("The node cannot apply the %s changes in place.", resources), nil
	case corev1.PodReasonDeferred:
		return false, fmt.Sprintf("The node has no free resources to apply the %s changes in place.", resources), nil
	default:
		return false, fmt.Sprintf("Applying the %s changes in place failed on the node.", resources), nil
	}
}

// describeHotplugMigrationPlan fills st for changes no migration has been created for yet.
func (h *SyncKvvmHandler) describeHotplugMigrationPlan(st *hotplugState, vm *v1alpha2.VirtualMachine, refusal string) {
	resources := st.resources()
	prefix := ""
	if refusal != "" {
		prefix = refusal + " "
	}

	if (st.cpu && !h.featureGate.Enabled(featuregates.HotplugCPUWithLiveMigration)) ||
		(st.memory && !h.featureGate.Enabled(featuregates.HotplugMemoryWithLiveMigration)) {
		st.warning = true
		st.message = prefix + fmt.Sprintf("Live migration to apply the %s changes is disabled in the module settings.", resources)
		st.giveUp()
		return
	}

	if h.isVMNonMigratable(vm) {
		if st.autoscaled {
			// The node keeps the change pending and applies it once it has room.
			st.message = prefix + fmt.Sprintf("The virtual machine cannot be migrated to another node, so the %s changes wait until its node can apply them in place.", resources)
			return
		}
		st.warning = true
		st.message = prefix + fmt.Sprintf("The virtual machine cannot be migrated to apply the %s changes.", resources)
		st.giveUp()
		return
	}

	st.message = prefix + fmt.Sprintf("The %s changes will be applied by live migration.", resources)
}

// describeHotplugMigrations fills st from the migrations of the virtual machine and reports
// whether they decide its state. Otherwise a migration is yet to be created.
func describeHotplugMigrations(st *hotplugState, plan hotplugmigration.Plan) bool {
	resources := st.resources()

	switch plan.Phase {
	case hotplugmigration.PhaseMigrating:
		st.message = fmt.Sprintf("The %s changes are being applied by live migration.", resources)
		completed, _ := conditions.GetCondition(vmopcondition.TypeCompleted, plan.Active.Status.Conditions)
		if plan.Active.Status.Phase == v1alpha2.VMOPPhasePending && completed.Message != "" {
			st.message += " The live migration has not started yet: " + withoutTrailingPeriod(completed.Message) + "."
		}
	case hotplugmigration.PhaseSettling:
		st.message = fmt.Sprintf("The %s changes are being applied to the virtual machine.", resources)
	case hotplugmigration.PhaseNotApplied:
		st.warning = true
		st.message = fmt.Sprintf("The virtual machine was migrated to apply the %s changes, but they have not been applied.", resources)
		st.giveUp()
	case hotplugmigration.PhaseRetrying:
		st.warning = true
		st.message = fmt.Sprintf("Live migration to apply the %s changes %s; the next attempt is at %s.",
			resources, describeFailures(plan.Failed), plan.At.UTC().Format(time.RFC3339))
	case hotplugmigration.PhaseExhausted:
		st.warning = true
		st.message = fmt.Sprintf("Live migration to apply the %s changes %s.", resources, describeFailures(plan.Failed))
		st.giveUp()
	default:
		return false
	}
	return true
}

// describeFailures tells how many times the migrations failed and why the last one did.
func describeFailures(failed []*v1alpha2.VirtualMachineOperation) string {
	times := "failed"
	if len(failed) > 1 {
		times = fmt.Sprintf("failed %d times", len(failed))
	}
	completed, _ := conditions.GetCondition(vmopcondition.TypeCompleted, failed[len(failed)-1].Status.Conditions)
	if completed.Message == "" {
		return times
	}
	return times + ": " + withoutTrailingPeriod(completed.Message)
}

// hotplugRestartAwaitingChanges lists the pending CPU and memory changes in the format of
// status.restartAwaitingChanges.
func hotplugRestartAwaitingChanges(st *hotplugState, vm *v1alpha2.VirtualMachine) ([]apiextensionsv1.JSON, error) {
	var changes vmchange.SpecChanges

	if st.cpu && vm.Status.Resources.CPU.Cores > 0 && vm.Status.Resources.CPU.Cores != vm.Spec.CPU.Cores {
		changes.Add(vmchange.FieldChange{
			Operation:      vmchange.ChangeReplace,
			Path:           "cpu.cores",
			CurrentValue:   vm.Status.Resources.CPU.Cores,
			DesiredValue:   vm.Spec.CPU.Cores,
			ActionRequired: vmchange.ActionRestart,
			RestartMessage: st.message,
		})
	}
	if st.memory && !vm.Status.Resources.Memory.Size.IsZero() && vm.Status.Resources.Memory.Size.Cmp(vm.Spec.Memory.Size) != 0 {
		changes.Add(vmchange.FieldChange{
			Operation:      vmchange.ChangeReplace,
			Path:           "memory.size",
			CurrentValue:   vm.Status.Resources.Memory.Size,
			DesiredValue:   vm.Spec.Memory.Size,
			ActionRequired: vmchange.ActionRestart,
		})
	}

	if changes.IsEmpty() {
		return nil, nil
	}
	return changes.ConvertPendingRestartChanges()
}

// isCoreFractionGuaranteed reports whether coreFraction is 100%, which makes the CPU requests
// equal the limits.
func isCoreFractionGuaranteed(coreFraction string) bool {
	if coreFraction == "" {
		coreFraction = vmchange.DefaultCPUCoreFraction
	}
	fraction, err := sizingpolicy.ParsePercent(coreFraction)
	if err != nil {
		return false
	}
	return fraction >= sizingpolicy.MaxCoreFraction
}

func withoutTrailingPeriod(s string) string {
	if len(s) > 0 && s[len(s)-1] == '.' {
		return s[:len(s)-1]
	}
	return s
}
