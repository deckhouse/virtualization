//go:build EE
// +build EE

/*
Copyright 2026 Flant JSC
Licensed under the Deckhouse Platform Enterprise Edition (EE) license. See https://github.com/deckhouse/deckhouse/blob/main/ee/LICENSE
*/

package handler

import (
	"context"
	"errors"
	"fmt"

	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/deckhouse/virtualization-controller/pkg/controller/vmpool/internal/poollabels"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/vmcondition"
)

const templateHandlerName = "template"

// TemplateHandler propagates virtualMachineTemplate changes to live replicas
// in place (the pool owns the member spec): it patches each replica's spec to
// the desired revision, then marks the replica as effectively on that revision
// once the change has taken effect. Whether a change is applied hot or needs a
// restart is decided by the VM layer — the pool does not duplicate that.
type TemplateHandler struct {
	client client.Client
}

func NewTemplateHandler(c client.Client) *TemplateHandler {
	return &TemplateHandler{client: c}
}

func (h *TemplateHandler) Name() string { return templateHandlerName }

func (h *TemplateHandler) Handle(ctx context.Context, pool *v1alpha2.VirtualMachinePool) (reconcile.Result, error) {
	if pool.GetDeletionTimestamp() != nil {
		return reconcile.Result{}, nil
	}

	members, err := poollabels.ListMembers(ctx, h.client, pool)
	if err != nil {
		return reconcile.Result{}, fmt.Errorf("list pool members: %w", err)
	}

	desiredHash := poollabels.ComputeTemplateHash(pool)
	tmplSpec := pool.Spec.VirtualMachineTemplate.Spec

	var errs error
	for i := range members {
		m := &members[i]
		if m.GetDeletionTimestamp() != nil {
			continue
		}

		// Step 1: bring the spec to the desired revision. Keyed on an annotation,
		// not a spec diff, because the apiserver mutates the spec (defaulting,
		// id allocation) and a diff would re-patch forever.
		if m.GetAnnotations()[poollabels.PatchedTemplateHash] != desiredHash {
			patched := m.DeepCopy()
			patched.Spec = *tmplSpec.DeepCopy()
			// Block device refs are per-replica: the disks handler has already
			// resolved the template's disk-template placeholders to this member's
			// concrete disks (e.g. "system" to "web-a-system", or a reuse disk).
			// Keep the member's resolved refs; re-copying the template placeholders
			// would dangle (no such disk) and duplicate the resolved ref. Disk
			// changes are reconciled by the disks handler, not the template rollout.
			patched.Spec.BlockDeviceRefs = append([]v1alpha2.BlockDeviceSpecRef(nil), m.Spec.BlockDeviceRefs...)
			if patched.Annotations == nil {
				patched.Annotations = map[string]string{}
			}
			patched.Annotations[poollabels.PatchedTemplateHash] = desiredHash
			if err := h.client.Update(ctx, patched); err != nil {
				errs = errors.Join(errs, fmt.Errorf("patch replica %s to template: %w", m.GetName(), err))
			}
			continue
		}

		// Step 2: the spec is on the desired revision. Mark the revision label as
		// effectively applied only once the disruptive part is no longer pending;
		// while the VM awaits a restart the label stays on the old revision.
		if awaitingRestart(m) {
			continue
		}
		if m.GetLabels()[poollabels.TemplateHash] != desiredHash {
			updated := m.DeepCopy()
			if updated.Labels == nil {
				updated.Labels = map[string]string{}
			}
			updated.Labels[poollabels.TemplateHash] = desiredHash
			if err := h.client.Update(ctx, updated); err != nil {
				errs = errors.Join(errs, fmt.Errorf("mark replica %s on current template: %w", m.GetName(), err))
			}
		}
	}

	return reconcile.Result{}, errs
}

// awaitingRestart reports whether the VM has pending disruptive changes waiting
// for a restart to apply.
func awaitingRestart(vm *v1alpha2.VirtualMachine) bool {
	c := meta.FindStatusCondition(vm.Status.Conditions, vmcondition.TypeAwaitingRestartToApplyConfiguration.String())
	return c != nil && c.Status == metav1.ConditionTrue
}
