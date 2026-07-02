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
		// id allocation) and a diff would re-patch forever. In this slice a
		// replica has no per-replica spec, so the whole spec follows the template
		// (per-replica disk refs are merged in a later slice).
		if m.GetAnnotations()[poollabels.PatchedTemplateHash] != desiredHash {
			patched := m.DeepCopy()
			patched.Spec = *tmplSpec.DeepCopy()
			// Preserve per-replica block device refs (e.g. disks the pool attached
			// to this member) — they are not part of the shared template and must
			// survive the spec patch.
			patched.Spec.BlockDeviceRefs = mergeBlockDeviceRefs(tmplSpec.BlockDeviceRefs, m.Spec.BlockDeviceRefs)
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

// mergeBlockDeviceRefs returns the template's block device refs plus any refs
// the member carries that the template does not (per-replica disks the pool
// attached). It keeps the template's order and appends the extras.
func mergeBlockDeviceRefs(templateRefs, memberRefs []v1alpha2.BlockDeviceSpecRef) []v1alpha2.BlockDeviceSpecRef {
	inTemplate := make(map[v1alpha2.BlockDeviceSpecRef]struct{}, len(templateRefs))
	for _, r := range templateRefs {
		inTemplate[r] = struct{}{}
	}
	merged := append([]v1alpha2.BlockDeviceSpecRef{}, templateRefs...)
	for _, r := range memberRefs {
		if _, ok := inTemplate[r]; !ok {
			merged = append(merged, r)
		}
	}
	return merged
}

// awaitingRestart reports whether the VM has pending disruptive changes waiting
// for a restart to apply.
func awaitingRestart(vm *v1alpha2.VirtualMachine) bool {
	c := meta.FindStatusCondition(vm.Status.Conditions, vmcondition.TypeAwaitingRestartToApplyConfiguration.String())
	return c != nil && c.Status == metav1.ConditionTrue
}
