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

// TemplateHandler propagates virtualMachineTemplate changes to live replicas in
// place: it patches each replica's spec to the desired revision, then marks the
// replica as on that revision once the change has taken effect. Hot vs restart is
// decided by the VM layer.
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

		// Step 1: patch the spec to the desired revision. Keyed on an annotation,
		// not a spec diff — the apiserver defaults fields, so a diff would re-patch
		// forever.
		if vm.GetAnnotations()[poollabels.PatchedTemplateHash] != desiredHash {
			applyTemplateSpec(vm, tmplSpec)
			if vm.Annotations == nil {
				vm.Annotations = make(map[string]string)
			}
			vm.Annotations[poollabels.PatchedTemplateHash] = desiredHash
			if err := h.client.Update(ctx, vm); err != nil {
				errs = errors.Join(errs, fmt.Errorf("patch replica %s to template: %w", vm.GetName(), err))
			}
			continue
		}

		// Step 2: spec is on the desired revision. Advance the revision label only
		// once the VM is no longer awaiting a restart to apply it.
		if awaitingRestart(vm) {
			continue
		}
		if vm.GetLabels()[poollabels.TemplateHash] != desiredHash {
			if vm.Labels == nil {
				vm.Labels = make(map[string]string)
			}
			vm.Labels[poollabels.TemplateHash] = desiredHash
			if err := h.client.Update(ctx, vm); err != nil {
				errs = errors.Join(errs, fmt.Errorf("mark replica %s on current template: %w", vm.GetName(), err))
			}
		}
	}

	return reconcile.Result{}, errs
}

// applyTemplateSpec sets vm's spec to the template's but keeps the member's own
// blockDeviceRefs: the disks handler has already resolved the template's disk
// placeholders to this member's real disks, so copying the placeholders back would
// dangle. blockDeviceRefs edits reach a replica on its next recreation, not in place.
func applyTemplateSpec(vm *v1alpha2.VirtualMachine, tmplSpec v1alpha2.VirtualMachineSpec) {
	refs := vm.Spec.BlockDeviceRefs
	vm.Spec = *tmplSpec.DeepCopy()
	vm.Spec.BlockDeviceRefs = refs
}

// awaitingRestart reports whether the VM has pending disruptive changes waiting
// for a restart to apply.
func awaitingRestart(vm *v1alpha2.VirtualMachine) bool {
	c := meta.FindStatusCondition(vm.Status.Conditions, vmcondition.TypeAwaitingRestartToApplyConfiguration.String())
	return c != nil && c.Status == metav1.ConditionTrue
}
