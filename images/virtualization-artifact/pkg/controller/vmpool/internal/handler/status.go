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
	"fmt"
	"slices"
	"strings"

	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/deckhouse/virtualization-controller/pkg/controller/vmpool/internal/poollabels"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/vmcondition"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/vmpoolcondition"
)

const statusHandlerName = "status"

// StatusHandler computes the pool status. It runs last in the chain, so the
// status reflects what the other handlers did in this pass.
type StatusHandler struct {
	client client.Client
}

func NewStatusHandler(c client.Client) *StatusHandler {
	return &StatusHandler{client: c}
}

func (h *StatusHandler) Name() string { return statusHandlerName }

func (h *StatusHandler) Handle(ctx context.Context, pool *v1alpha2.VirtualMachinePool) (reconcile.Result, error) {
	// A pool being deleted keeps its last status: its members go away with it.
	if pool.GetDeletionTimestamp() != nil {
		return reconcile.Result{}, nil
	}

	members, err := poollabels.ListMembers(ctx, h.client, pool)
	if err != nil {
		return reconcile.Result{}, fmt.Errorf("list pool members: %w", err)
	}
	var disks v1alpha2.VirtualDiskList
	if err := h.client.List(ctx, &disks,
		client.InNamespace(pool.GetNamespace()),
		client.MatchingLabels{poollabels.PoolUID: string(pool.GetUID())},
	); err != nil {
		return reconcile.Result{}, fmt.Errorf("list pool disks: %w", err)
	}
	diskTemplateOf := make(map[string]string, len(disks.Items))
	for i := range disks.Items {
		diskTemplateOf[disks.Items[i].Name] = disks.Items[i].GetLabels()[poollabels.DiskTemplate]
	}

	updateStatus(pool, members, diskTemplateOf, ReportFrom(ctx))
	return reconcile.Result{}, nil
}

// poolView is what the status is computed from: the members classified once.
type poolView struct {
	desired int
	// all counts every member, Terminating included: it still holds capacity.
	all int
	// live members are the non-Terminating ones.
	live int
	// created counts live members that got every disk they were created with.
	created        int
	ready          int
	updated        int
	restartPending int
	// notReady are the live members that are not ready, sorted by name.
	notReady    []*v1alpha2.VirtualMachine
	terminating []*v1alpha2.VirtualMachine
}

func newPoolView(pool *v1alpha2.VirtualMachinePool, members []v1alpha2.VirtualMachine, diskTemplateOf map[string]string, desiredHash string) poolView {
	v := poolView{desired: int(ptr.Deref(pool.Spec.Replicas, 0)), all: len(members)}
	templates := make(map[string]bool, len(pool.Spec.VirtualDiskTemplates))
	for i := range pool.Spec.VirtualDiskTemplates {
		templates[pool.Spec.VirtualDiskTemplates[i].Name] = true
	}
	for i := range members {
		vm := &members[i]
		if vm.GetDeletionTimestamp() != nil {
			v.terminating = append(v.terminating, vm)
			continue
		}
		v.live++
		if isReady(vm) {
			v.ready++
		} else {
			v.notReady = append(v.notReady, vm)
		}
		// A replica still waiting for its first disks is a scaling matter, not a
		// rollout one: Synced looks at created replicas only.
		if isCreated(vm, templates) {
			v.created++
			if isUpToDate(vm, desiredHash, templates, diskTemplateOf) {
				v.updated++
			}
		}
		// Patched to the desired revision but the disruptive part awaits a restart.
		if vm.GetAnnotations()[poollabels.PatchedTemplateHash] == desiredHash && awaitingRestart(vm) {
			v.restartPending++
		}
	}
	byName := func(a, b *v1alpha2.VirtualMachine) int { return strings.Compare(a.GetName(), b.GetName()) }
	slices.SortFunc(v.notReady, byName)
	slices.SortFunc(v.terminating, byName)
	return v
}

// isReady: a migrating replica keeps serving, so it is as ready as a running one.
func isReady(vm *v1alpha2.VirtualMachine) bool {
	return vm.Status.Phase == v1alpha2.MachineRunning || vm.Status.Phase == v1alpha2.MachineMigrating
}

// isCreated: a new replica carries the disk templates as placeholder refs until
// the disks handler resolves each to a real disk. Only current templates count:
// a placeholder of a removed template is never resolved.
func isCreated(vm *v1alpha2.VirtualMachine, templates map[string]bool) bool {
	for _, ref := range vm.Spec.BlockDeviceRefs {
		if ref.Kind == v1alpha2.DiskDevice && templates[ref.Name] {
			return false
		}
	}
	return true
}

// isUpToDate: the replica runs the current template in full — the spec is applied
// without a pending restart, and it references a disk of every current disk
// template and none of a removed one. Checked on the spec: a stopped replica has
// no attachment status.
func isUpToDate(vm *v1alpha2.VirtualMachine, desiredHash string, templates map[string]bool, diskTemplateOf map[string]string) bool {
	if vm.GetLabels()[poollabels.TemplateHash] != desiredHash || awaitingRestart(vm) {
		return false
	}
	covered := make(map[string]bool, len(templates))
	for _, ref := range vm.Spec.BlockDeviceRefs {
		if ref.Kind != v1alpha2.DiskDevice {
			continue
		}
		tmpl, managed := diskTemplateOf[ref.Name]
		if !managed {
			continue
		}
		if !templates[tmpl] {
			return false
		}
		covered[tmpl] = true
	}
	return len(covered) == len(templates)
}

func updateStatus(pool *v1alpha2.VirtualMachinePool, members []v1alpha2.VirtualMachine, diskTemplateOf map[string]string, report *passReport) {
	desiredHash := poollabels.ComputeTemplateHash(pool)
	v := newPoolView(pool, members, diskTemplateOf, desiredHash)

	pool.Status.ObservedGeneration = pool.GetGeneration()
	pool.Status.Replicas = int32(v.all)
	pool.Status.ReadyReplicas = int32(v.ready)
	pool.Status.UpdatedReplicas = int32(v.updated)
	pool.Status.RestartPendingReplicas = int32(v.restartPending)
	pool.Status.DesiredTemplateHash = desiredHash
	pool.Status.Selector = poollabels.StatusSelector(pool)

	setAvailable(pool, v)
	setProgressing(pool, v, report)
	setSynced(pool, v, report)
}

func setCondition(pool *v1alpha2.VirtualMachinePool, t vmpoolcondition.Type, status metav1.ConditionStatus, reason, message string) {
	meta.SetStatusCondition(&pool.Status.Conditions, metav1.Condition{
		Type:               t.String(),
		Status:             status,
		Reason:             reason,
		ObservedGeneration: pool.GetGeneration(),
		Message:            message,
	})
}

func setAvailable(pool *v1alpha2.VirtualMachinePool, v poolView) {
	if v.ready >= v.desired {
		setCondition(pool, vmpoolcondition.TypeAvailable, metav1.ConditionTrue, vmpoolcondition.ReasonAllReplicasReady.String(), "")
		return
	}
	if v.live >= v.desired && allStoppedByPolicy(v.notReady) {
		setCondition(pool, vmpoolcondition.TypeAvailable, metav1.ConditionFalse, vmpoolcondition.ReasonReplicasStopped.String(), stoppedMessage(v))
		return
	}
	setCondition(pool, vmpoolcondition.TypeAvailable, metav1.ConditionFalse, vmpoolcondition.ReasonInsufficientReadyReplicas.String(), notReadyMessage(v))
}

// allStoppedByPolicy: every not ready replica is powered off or changing its power
// state, under a run policy that lets it stay off. Under AlwaysOn a stopped
// replica is a problem, not an intent.
func allStoppedByPolicy(notReady []*v1alpha2.VirtualMachine) bool {
	for _, vm := range notReady {
		switch vm.Status.Phase {
		case v1alpha2.MachineStopped, v1alpha2.MachineStopping, v1alpha2.MachineStarting:
		default:
			return false
		}
		switch vm.Spec.RunPolicy {
		case v1alpha2.ManualPolicy, v1alpha2.AlwaysOffPolicy, v1alpha2.AlwaysOnUnlessStoppedManually:
		default:
			return false
		}
	}
	return len(notReady) > 0
}

func stoppedMessage(v poolView) string {
	n := len(v.notReady)
	for _, vm := range v.notReady {
		// Starting or stopping on its own: no hint, the replica is already moving.
		if vm.Status.Phase != v1alpha2.MachineStopped {
			return fmt.Sprintf("%d of %d %s stopped or changing the power state.", n, v.desired, replicasAre(n))
		}
	}
	msg := fmt.Sprintf("%d of %d %s stopped.", n, v.desired, replicasAre(n))
	policy := v.notReady[0].Spec.RunPolicy
	for _, vm := range v.notReady {
		if vm.Spec.RunPolicy != policy {
			return msg + " Start " + them(n) + " to restore the pool capacity."
		}
	}
	switch policy {
	case v1alpha2.ManualPolicy:
		return msg + " The run policy is Manual: start the virtual machines manually or change the run policy."
	case v1alpha2.AlwaysOffPolicy:
		return msg + " The run policy is AlwaysOff: change the run policy to start " + them(n) + "."
	default:
		return msg + " Start " + them(n) + " to restore the pool capacity."
	}
}

// notReadyMessage breaks the not ready replicas down by phase and names one, so
// the user knows which virtual machine to look at.
func notReadyMessage(v poolView) string {
	var parts []string
	if missing := v.desired - v.all; missing > 0 {
		parts = append(parts, fmt.Sprintf("%d not created yet", missing))
	}
	phases := map[string]int{}
	for _, vm := range v.notReady {
		phases[string(vm.Status.Phase)]++
	}
	if len(v.terminating) > 0 {
		phases[string(v1alpha2.MachineTerminating)] += len(v.terminating)
	}
	names := make([]string, 0, len(phases))
	for p := range phases {
		names = append(names, p)
	}
	slices.Sort(names)
	for _, p := range names {
		label := p
		if label == "" {
			label = "without a phase"
		}
		parts = append(parts, fmt.Sprintf("%d %s", phases[p], label))
	}

	msg := fmt.Sprintf("%d of %d replicas are ready", v.ready, v.desired)
	if len(parts) > 0 {
		msg += ": " + strings.Join(parts, ", ")
	}
	msg += "."
	if vm := firstProblem(v); vm != nil {
		if vm.Status.Phase == "" {
			msg += fmt.Sprintf(" VirtualMachine %q has no phase yet", vm.GetName())
		} else {
			msg += fmt.Sprintf(" VirtualMachine %q is %s", vm.GetName(), vm.Status.Phase)
		}
		if c := meta.FindStatusCondition(vm.Status.Conditions, vmcondition.TypeRunning.String()); c != nil && c.Reason != "" {
			msg += fmt.Sprintf(" (%s)", c.Reason)
		}
		msg += "."
	}
	return msg
}

// firstProblem picks the replica to name: a not ready one that is not simply
// stopped by its policy, else a stuck Terminating one.
func firstProblem(v poolView) *v1alpha2.VirtualMachine {
	for _, vm := range v.notReady {
		if !allStoppedByPolicy([]*v1alpha2.VirtualMachine{vm}) {
			return vm
		}
	}
	if len(v.terminating) > 0 {
		return v.terminating[0]
	}
	return nil
}

func setProgressing(pool *v1alpha2.VirtualMachinePool, v poolView, report *passReport) {
	// A replica still waiting for its disks is not created yet; Terminating ones
	// still hold capacity, as in the sync handler.
	count := v.created + len(v.terminating)
	converged := count == v.desired
	t := vmpoolcondition.TypeProgressing
	switch {
	case report.creation != "":
		setCondition(pool, t, metav1.ConditionFalse, vmpoolcondition.ReasonReplicaCreationFailed.String(), scalingMessage(v, count)+" "+report.creation)
	case report.deletion != "":
		setCondition(pool, t, metav1.ConditionFalse, vmpoolcondition.ReasonReplicaDeletionFailed.String(), scalingMessage(v, count)+" "+report.deletion)
	case converged:
		// Steady state: drop the condition instead of parking it at False, so the UI does
		// not show a permanent inactive block. Matches the VirtualMachine Migrating condition.
		meta.RemoveStatusCondition(&pool.Status.Conditions, t.String())
	case !report.scaleAttempted && lastFailure(pool):
		// The sync handler waited for its previous actions this pass and tried
		// nothing, so the last failure still stands.
	case pool.Spec.ScaleDownPolicy == v1alpha2.ScaleDownPolicyExplicit && v.live > v.desired:
		setCondition(pool, t, metav1.ConditionFalse, vmpoolcondition.ReasonScaleDownBlocked.String(),
			fmt.Sprintf("%s, %s. The Explicit scale-down policy removes replicas only by name: use the scaleDownWith subresource.", replicasExist(v.live), requested(v.desired)))
	default:
		setCondition(pool, t, metav1.ConditionTrue, vmpoolcondition.ReasonReplicasProgressing.String(), scalingMessage(v, count))
	}
}

func lastFailure(pool *v1alpha2.VirtualMachinePool) bool {
	c := meta.FindStatusCondition(pool.Status.Conditions, vmpoolcondition.TypeProgressing.String())
	return c != nil && (c.Reason == vmpoolcondition.ReasonReplicaCreationFailed.String() || c.Reason == vmpoolcondition.ReasonReplicaDeletionFailed.String())
}

func scalingMessage(v poolView, count int) string {
	if count < v.desired {
		return fmt.Sprintf("Scaling up: %d of %d replicas are created.", count, v.desired)
	}
	return fmt.Sprintf("Scaling down: %s, %s.", replicasExist(count), requested(v.desired))
}

func setSynced(pool *v1alpha2.VirtualMachinePool, v poolView, report *passReport) {
	t := vmpoolcondition.TypeSynced
	switch {
	case report.update != "":
		msg := fmt.Sprintf("%d of %d replicas cannot apply the template", len(report.updateFailed), v.created)
		if v.restartPending > 0 {
			msg += fmt.Sprintf("; %d of %d replicas await a restart", v.restartPending, v.created)
		}
		setCondition(pool, t, metav1.ConditionFalse, vmpoolcondition.ReasonReplicaUpdateFailed.String(), msg+". "+report.update)
	case v.updated >= v.created:
		setCondition(pool, t, metav1.ConditionTrue, vmpoolcondition.ReasonPoolSynced.String(), "")
	case v.restartPending > 0:
		// Some replicas are patched but wait for a restart that will not happen
		// on its own under restartApprovalMode: Manual.
		setCondition(pool, t, metav1.ConditionFalse, vmpoolcondition.ReasonRestartPendingApproval.String(),
			fmt.Sprintf("%d of %d replicas await a restart to apply configuration.", v.restartPending, v.created))
	default:
		setCondition(pool, t, metav1.ConditionFalse, vmpoolcondition.ReasonRolloutInProgress.String(),
			fmt.Sprintf("%d of %d replicas are on the current virtualMachineTemplate.", v.updated, v.created))
	}
}

func replicasAre(n int) string {
	if n == 1 {
		return "replica is"
	}
	return "replicas are"
}

func replicasExist(n int) string {
	if n == 1 {
		return "1 replica exists"
	}
	return fmt.Sprintf("%d replicas exist", n)
}

func requested(n int) string {
	if n == 1 {
		return "1 is requested"
	}
	return fmt.Sprintf("%d are requested", n)
}

func them(n int) string {
	if n == 1 {
		return "it"
	}
	return "them"
}
