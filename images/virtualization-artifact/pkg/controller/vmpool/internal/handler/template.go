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
		vm := &members[i]
		if vm.GetDeletionTimestamp() != nil {
			continue
		}

		// Step 1: bring the spec to the desired revision. Keyed on an annotation,
		// not a spec diff, because the apiserver mutates the spec (defaulting,
		// id allocation) and a diff would re-patch forever.
		if vm.GetAnnotations()[poollabels.PatchedTemplateHash] != desiredHash {
			patched := applyTemplateSpec(vm, tmplSpec)
			if patched.Annotations == nil {
				patched.Annotations = make(map[string]string)
			}
			patched.Annotations[poollabels.PatchedTemplateHash] = desiredHash
			if err := h.client.Update(ctx, patched); err != nil {
				errs = errors.Join(errs, fmt.Errorf("patch replica %s to template: %w", vm.GetName(), err))
			}
			continue
		}

		// Step 2: the spec is on the desired revision. Mark the revision label as
		// effectively applied only once the disruptive part is no longer pending;
		// while the VM awaits a restart the label stays on the old revision.
		if awaitingRestart(vm) {
			continue
		}
		if vm.GetLabels()[poollabels.TemplateHash] != desiredHash {
			updated := vm.DeepCopy()
			if updated.Labels == nil {
				updated.Labels = make(map[string]string)
			}
			updated.Labels[poollabels.TemplateHash] = desiredHash
			if err := h.client.Update(ctx, updated); err != nil {
				errs = errors.Join(errs, fmt.Errorf("mark replica %s on current template: %w", vm.GetName(), err))
			}
		}
	}

	return reconcile.Result{}, errs
}

// applyTemplateSpec returns a copy of vm with its spec brought to the pool
// template's spec while keeping the member's own blockDeviceRefs. The template
// lists disk-template placeholders (kind VirtualDisk, name = a virtualDiskTemplates
// entry) plus any shared images; the disks handler has already resolved each
// placeholder to this member's concrete disk (e.g. "system" -> "web-a-system", or a
// reuse disk). Re-copying the template placeholders would dangle (no such disk) and
// duplicate the resolved ref. Disk add/remove is reconciled by the disks handler;
// blockDeviceRefs edits (reorder, added/removed image) reach a live replica on its
// next recreation (rotation/scale-up), like other disruptive template changes the
// pool does not force.
func applyTemplateSpec(vm *v1alpha2.VirtualMachine, tmplSpec v1alpha2.VirtualMachineSpec) *v1alpha2.VirtualMachine {
	patched := vm.DeepCopy()
	refs := patched.Spec.BlockDeviceRefs
	patched.Spec = *tmplSpec.DeepCopy()
	patched.Spec.BlockDeviceRefs = refs
	return patched
}

// awaitingRestart reports whether the VM has pending disruptive changes waiting
// for a restart to apply.
func awaitingRestart(vm *v1alpha2.VirtualMachine) bool {
	c := meta.FindStatusCondition(vm.Status.Conditions, vmcondition.TypeAwaitingRestartToApplyConfiguration.String())
	return c != nil && c.Status == metav1.ConditionTrue
}
