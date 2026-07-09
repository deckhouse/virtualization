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

package vmpool

import (
	"context"
	"fmt"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	"github.com/deckhouse/deckhouse/pkg/log"
	"github.com/deckhouse/virtualization-controller/pkg/controller/vd"
	"github.com/deckhouse/virtualization-controller/pkg/controller/vm"
	"github.com/deckhouse/virtualization-controller/pkg/featuregates"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
)

// maxPoolNameLength keeps generated member names ("<pool>-<5 chars>") within the
// 63-character limit a VirtualMachine name must satisfy.
const maxPoolNameLength = 57

// SetupValidationWebhook validates the pool's template specs on create/update, so
// a bad template is rejected up front instead of only as a FailedCreate event.
// Self-gated by the feature gate.
func SetupValidationWebhook(mgr manager.Manager, log *log.Logger) error {
	if !featuregates.Default().Enabled(featuregates.VirtualMachinePool) {
		return nil
	}
	return builder.WebhookManagedBy(mgr).
		For(&v1alpha2.VirtualMachinePool{}).
		WithValidator(&poolValidator{
			vmValidator:   vm.NewTemplateSpecValidator(mgr.GetClient(), featuregates.Default(), log),
			diskValidator: vd.NewTemplateSpecValidator(mgr.GetClient()),
		}).
		Complete()
}

// poolValidator checks the pool name length, then runs the template-spec
// validators against a VirtualMachine and VirtualDisks built from the template.
type poolValidator struct {
	vmValidator   *vm.Validator
	diskValidator *vd.Validator
}

func (v *poolValidator) validate(ctx context.Context, pool *v1alpha2.VirtualMachinePool) (admission.Warnings, error) {
	if len(pool.GetName()) > maxPoolNameLength {
		return nil, fmt.Errorf(
			"VirtualMachinePool name %q is too long: it must be at most %d characters so that generated VirtualMachine names stay within the 63-character limit",
			pool.GetName(), maxPoolNameLength,
		)
	}

	warnings, err := v.vmValidator.ValidateCreate(ctx, vmFromTemplate(pool))
	if err != nil {
		return nil, err
	}

	for i := range pool.Spec.VirtualDiskTemplates {
		warn, err := v.diskValidator.ValidateCreate(ctx, diskFromTemplate(pool, pool.Spec.VirtualDiskTemplates[i]))
		if err != nil {
			return nil, err
		}
		warnings = append(warnings, warn...)
	}
	return warnings, nil
}

func (v *poolValidator) ValidateCreate(ctx context.Context, obj runtime.Object) (admission.Warnings, error) {
	pool, ok := obj.(*v1alpha2.VirtualMachinePool)
	if !ok {
		return nil, fmt.Errorf("expected a VirtualMachinePool but got a %T", obj)
	}
	return v.validate(ctx, pool)
}

func (v *poolValidator) ValidateUpdate(ctx context.Context, _, newObj runtime.Object) (admission.Warnings, error) {
	pool, ok := newObj.(*v1alpha2.VirtualMachinePool)
	if !ok {
		return nil, fmt.Errorf("expected a VirtualMachinePool but got a %T", newObj)
	}
	return v.validate(ctx, pool)
}

func (v *poolValidator) ValidateDelete(_ context.Context, _ runtime.Object) (admission.Warnings, error) {
	return nil, nil
}

// vmFromTemplate builds the VirtualMachine the pool would create, so the VM
// validators see the same spec a real replica would carry.
func vmFromTemplate(pool *v1alpha2.VirtualMachinePool) *v1alpha2.VirtualMachine {
	tmpl := pool.Spec.VirtualMachineTemplate
	return &v1alpha2.VirtualMachine{
		ObjectMeta: metav1.ObjectMeta{
			Namespace:    pool.GetNamespace(),
			GenerateName: pool.GetName() + "-",
			Labels:       tmpl.Metadata.Labels,
			Annotations:  tmpl.Metadata.Annotations,
		},
		Spec: *tmpl.Spec.DeepCopy(),
	}
}

// diskFromTemplate builds the VirtualDisk the pool would create from a disk
// template, so the disk validators see the same spec a real disk would carry.
func diskFromTemplate(pool *v1alpha2.VirtualMachinePool, diskTemplate v1alpha2.VirtualDiskTemplateSpec) *v1alpha2.VirtualDisk {
	return &v1alpha2.VirtualDisk{
		ObjectMeta: metav1.ObjectMeta{
			Namespace:    pool.GetNamespace(),
			GenerateName: pool.GetName() + "-" + diskTemplate.Name + "-",
		},
		Spec: *diskTemplate.Spec.DeepCopy(),
	}
}
