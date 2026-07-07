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
	"errors"
	"fmt"
	"sort"
	"time"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/deckhouse/virtualization-controller/pkg/controller/vmpool/internal/expectations"
	"github.com/deckhouse/virtualization-controller/pkg/controller/vmpool/internal/poollabels"
	"github.com/deckhouse/virtualization-controller/pkg/eventrecord"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/vmpoolcondition"
)

const syncHandlerName = "sync"

// expectationsRecheck is how soon reconcile retries while it waits for pending
// creations/deletions to settle in the informer cache. It is a safety net: the
// member watcher normally re-enqueues the pool as soon as the events arrive.
const expectationsRecheck = 15 * time.Second

// Event reasons for pool scaling, mirroring the ReplicaSet vocabulary so they
// are familiar in `kubectl describe`/`kubectl get events`.
const (
	reasonSuccessfulCreate = "SuccessfulCreate"
	reasonFailedCreate     = "FailedCreate"
	reasonSuccessfulDelete = "SuccessfulDelete"
	reasonFailedDelete     = "FailedDelete"
)

// SyncHandler keeps the number of pool members equal to spec.replicas: it
// creates missing replicas from the template and removes surplus ones, guarding
// every action with expectations so a lagging cache cannot cause double-acting.
type SyncHandler struct {
	client   client.Client
	exp      *expectations.Expectations
	recorder eventrecord.EventRecorderLogger
}

func NewSyncHandler(c client.Client, exp *expectations.Expectations, recorder eventrecord.EventRecorderLogger) *SyncHandler {
	return &SyncHandler{client: c, exp: exp, recorder: recorder}
}

func (h *SyncHandler) Name() string { return syncHandlerName }

func (h *SyncHandler) Handle(ctx context.Context, pool *v1alpha2.VirtualMachinePool) (reconcile.Result, error) {
	key := types.NamespacedName{Namespace: pool.GetNamespace(), Name: pool.GetName()}.String()

	// The pool is going away — its members are garbage-collected via ownerRef.
	// Drop the expectations entry so it does not leak.
	if pool.GetDeletionTimestamp() != nil {
		h.exp.Forget(key)
		return reconcile.Result{}, nil
	}

	members, err := h.listMembers(ctx, pool)
	if err != nil {
		return reconcile.Result{}, fmt.Errorf("list pool members: %w", err)
	}

	// Status always reflects the observed set, whether or not we act this pass.
	defer h.updateStatus(pool, members)

	// Do not create/delete again until previous actions are observed (or expire):
	// this is what stops a lagging cache from over-creating anonymous replicas.
	if !h.exp.Satisfied(key) {
		return reconcile.Result{RequeueAfter: expectationsRecheck}, nil
	}

	desired := int(ptr.Deref(pool.Spec.Replicas, 0))
	// live counts every member, including Terminating and Stopped: a Terminating
	// replica still holds capacity, and counting it prevents a premature
	// replacement (invariant 2).
	live := len(members)

	switch {
	case live < desired:
		return reconcile.Result{}, h.scaleUp(ctx, pool, key, desired-live)
	case live > desired:
		return reconcile.Result{}, h.scaleDown(ctx, pool, key, members, live-desired)
	default:
		return reconcile.Result{}, nil
	}
}

func (h *SyncHandler) listMembers(ctx context.Context, pool *v1alpha2.VirtualMachinePool) ([]v1alpha2.VirtualMachine, error) {
	return poollabels.ListMembers(ctx, h.client, pool)
}

func (h *SyncHandler) scaleUp(ctx context.Context, pool *v1alpha2.VirtualMachinePool, key string, n int) error {
	// Record the expectation before creating so a create event cannot be observed
	// before we start waiting for it.
	h.exp.ExpectCreations(key, n)
	var errs error
	for i := 0; i < n; i++ {
		vm := h.newMember(pool)
		if err := h.client.Create(ctx, vm); err != nil {
			// This creation will never be observed — stop waiting for it.
			h.exp.CreationObserved(key)
			h.recorder.Eventf(pool, corev1.EventTypeWarning, reasonFailedCreate,
				"Failed to create a VirtualMachine from the template: %v", err)
			errs = errors.Join(errs, fmt.Errorf("create replica: %w", err))
			continue
		}
		h.recorder.Eventf(pool, corev1.EventTypeNormal, reasonSuccessfulCreate,
			"Created VirtualMachine %q.", vm.GetName())
	}
	return errs
}

func (h *SyncHandler) scaleDown(ctx context.Context, pool *v1alpha2.VirtualMachinePool, key string, members []v1alpha2.VirtualMachine, surplus int) error {
	// Terminating members already count toward the reduction (invariant 2), so
	// subtract them and only remove additional healthy replicas for the remainder.
	terminating := 0
	candidates := make([]v1alpha2.VirtualMachine, 0, len(members))
	for i := range members {
		if members[i].GetDeletionTimestamp() != nil {
			terminating++
			continue
		}
		candidates = append(candidates, members[i])
	}

	toDelete := surplus - terminating
	if toDelete <= 0 {
		return nil
	}

	victims := pickVictims(pool.Spec.ScaleDownPolicy, candidates, toDelete)
	if len(victims) == 0 {
		// Explicit policy: anonymous scale-down is not allowed here — replicas are
		// removed only by address (scaleDownWith). The /scale path is additionally
		// blocked by an admission webhook.
		return nil
	}
	uids := make([]types.UID, 0, len(victims))
	for i := range victims {
		uids = append(uids, victims[i].GetUID())
	}
	h.exp.ExpectDeletions(key, uids)

	var errs error
	for i := range victims {
		if err := h.client.Delete(ctx, &victims[i]); err != nil {
			// Already gone or failed — stop waiting for that deletion event.
			h.exp.DeletionObserved(key, victims[i].GetUID())
			if !apierrors.IsNotFound(err) {
				h.recorder.Eventf(pool, corev1.EventTypeWarning, reasonFailedDelete,
					"Failed to delete VirtualMachine %q: %v", victims[i].GetName(), err)
				errs = errors.Join(errs, fmt.Errorf("delete replica %s: %w", victims[i].GetName(), err))
			}
			continue
		}
		h.recorder.Eventf(pool, corev1.EventTypeNormal, reasonSuccessfulDelete,
			"Deleted VirtualMachine %q.", victims[i].GetName())
	}
	return errs
}

// pickVictims chooses which replicas to remove during anonymous scale-down,
// honouring the pool's scaleDownPolicy. Explicit forbids anonymous removal, so
// it returns nothing — such pools shrink only through addressed removal.
func pickVictims(policy v1alpha2.ScaleDownPolicy, candidates []v1alpha2.VirtualMachine, n int) []v1alpha2.VirtualMachine {
	if n <= 0 || policy == v1alpha2.ScaleDownPolicyExplicit {
		return nil
	}
	oldestFirst := policy == v1alpha2.ScaleDownPolicyOldestFirst
	sort.SliceStable(candidates, func(i, j int) bool {
		ti := candidates[i].GetCreationTimestamp().Time
		tj := candidates[j].GetCreationTimestamp().Time
		if oldestFirst {
			return ti.Before(tj)
		}
		return tj.Before(ti) // NewestFirst: youngest removed first
	})
	if n > len(candidates) {
		n = len(candidates)
	}
	return candidates[:n]
}

func (h *SyncHandler) newMember(pool *v1alpha2.VirtualMachinePool) *v1alpha2.VirtualMachine {
	tmpl := pool.Spec.VirtualMachineTemplate

	labels := make(map[string]string, len(tmpl.Metadata.Labels)+3)
	for k, v := range tmpl.Metadata.Labels {
		labels[k] = v
	}
	for k, v := range poollabels.Member(pool) {
		labels[k] = v
	}
	// Stamp the revision the replica is created on.
	labels[poollabels.TemplateHash] = poollabels.ComputeTemplateHash(pool)

	var annotations map[string]string
	if len(tmpl.Metadata.Annotations) > 0 {
		annotations = make(map[string]string, len(tmpl.Metadata.Annotations))
		for k, v := range tmpl.Metadata.Annotations {
			annotations[k] = v
		}
	}

	// The replica's block devices are the template's blockDeviceRefs verbatim: the
	// user lists the disk-template placeholders (kind VirtualDisk, name = a
	// virtualDiskTemplates entry) and any shared images, in boot order. The disks
	// handler resolves each VirtualDisk placeholder in place to this replica's
	// concrete disk (Delete -> <vm>-<tmpl>, Retain -> a reuse disk); image refs are
	// left untouched (shared, read-only). The bijection is guaranteed by admission.
	spec := *tmpl.Spec.DeepCopy()

	return &v1alpha2.VirtualMachine{
		ObjectMeta: metav1.ObjectMeta{
			GenerateName:    pool.GetName() + "-",
			Namespace:       pool.GetNamespace(),
			Labels:          labels,
			Annotations:     annotations,
			OwnerReferences: []metav1.OwnerReference{*metav1.NewControllerRef(pool, v1alpha2.VirtualMachinePoolGVK)},
		},
		Spec: spec,
	}
}

func (h *SyncHandler) updateStatus(pool *v1alpha2.VirtualMachinePool, members []v1alpha2.VirtualMachine) {
	desiredHash := poollabels.ComputeTemplateHash(pool)

	ready := 0
	liveNonTerminating := 0
	updated := 0
	restartPending := 0
	for i := range members {
		if members[i].GetDeletionTimestamp() != nil {
			continue
		}
		liveNonTerminating++
		if members[i].Status.Phase == v1alpha2.MachineRunning {
			ready++
		}
		if members[i].GetLabels()[poollabels.TemplateHash] == desiredHash {
			updated++
		}
		// Patched to the desired revision but the disruptive part awaits a restart.
		if members[i].GetAnnotations()[poollabels.PatchedTemplateHash] == desiredHash && awaitingRestart(&members[i]) {
			restartPending++
		}
	}
	desired := int(ptr.Deref(pool.Spec.Replicas, 0))

	pool.Status.ObservedGeneration = pool.GetGeneration()
	pool.Status.Replicas = int32(len(members))
	pool.Status.ReadyReplicas = int32(ready)
	pool.Status.UpdatedReplicas = int32(updated)
	pool.Status.RestartPendingReplicas = int32(restartPending)
	pool.Status.DesiredTemplateHash = desiredHash
	pool.Status.Selector = poollabels.StatusSelector(pool)

	availableStatus := metav1.ConditionFalse
	availableReason := vmpoolcondition.ReasonInsufficientReadyReplicas
	availableMessage := fmt.Sprintf("Only %d of %d replicas are ready.", ready, desired)
	if ready >= desired {
		availableStatus = metav1.ConditionTrue
		availableReason = vmpoolcondition.ReasonAllReplicasReady
		availableMessage = fmt.Sprintf("All %d replicas are ready.", desired)
	}
	meta.SetStatusCondition(&pool.Status.Conditions, metav1.Condition{
		Type:               vmpoolcondition.TypeAvailable.String(),
		Status:             availableStatus,
		Reason:             availableReason.String(),
		ObservedGeneration: pool.GetGeneration(),
		Message:            availableMessage,
	})

	progressingStatus := metav1.ConditionFalse
	progressingReason := vmpoolcondition.ReasonPoolStable
	progressingMessage := "No replica changes in progress."
	if len(members) != desired {
		progressingStatus = metav1.ConditionTrue
		progressingReason = vmpoolcondition.ReasonReplicasProgressing
		progressingMessage = fmt.Sprintf("Converging to %d replicas (currently %d).", desired, len(members))
	}
	meta.SetStatusCondition(&pool.Status.Conditions, metav1.Condition{
		Type:               vmpoolcondition.TypeProgressing.String(),
		Status:             progressingStatus,
		Reason:             progressingReason.String(),
		ObservedGeneration: pool.GetGeneration(),
		Message:            progressingMessage,
	})

	syncedStatus := metav1.ConditionTrue
	syncedReason := vmpoolcondition.ReasonPoolSynced
	syncedMessage := "All replicas are on the current virtualMachineTemplate."
	if updated < liveNonTerminating {
		syncedStatus = metav1.ConditionFalse
		syncedReason = vmpoolcondition.ReasonRolloutInProgress
		syncedMessage = fmt.Sprintf("%d of %d replicas are on the current virtualMachineTemplate.", updated, liveNonTerminating)
		if restartPending > 0 {
			// Some replicas are patched but wait for a restart that will not happen
			// on its own under restartApprovalMode: Manual.
			syncedReason = vmpoolcondition.ReasonRestartPendingApproval
			syncedMessage = fmt.Sprintf("%d of %d replicas await a restart to apply configuration.", restartPending, liveNonTerminating)
		}
	}
	meta.SetStatusCondition(&pool.Status.Conditions, metav1.Condition{
		Type:               vmpoolcondition.TypeSynced.String(),
		Status:             syncedStatus,
		Reason:             syncedReason.String(),
		ObservedGeneration: pool.GetGeneration(),
		Message:            syncedMessage,
	})
}
