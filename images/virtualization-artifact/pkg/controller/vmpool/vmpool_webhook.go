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
	"github.com/deckhouse/virtualization-controller/pkg/controller/vm"
	"github.com/deckhouse/virtualization-controller/pkg/featuregates"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
)

// SetupValidationWebhook registers the validating webhook for the pool object. It
// validates the embedded virtualMachineTemplate spec up front (at pool
// create/update) instead of only surfacing a bad template as a FailedCreate event
// once the controller tries to create a member. Self-gated by the feature gate,
// like the controller and the scale webhook.
func SetupValidationWebhook(mgr manager.Manager, log *log.Logger) error {
	if !featuregates.Default().Enabled(featuregates.VirtualMachinePool) {
		return nil
	}
	return builder.WebhookManagedBy(mgr).
		For(&v1alpha2.VirtualMachinePool{}).
		WithValidator(&poolValidator{
			vmValidator: vm.NewTemplateSpecValidator(mgr.GetClient(), featuregates.Default(), log),
		}).
		Complete()
}

// poolValidator validates a VirtualMachinePool by running the template-spec
// validators against a VirtualMachine synthesized from the pool template.
type poolValidator struct {
	vmValidator *vm.Validator
}

func (v *poolValidator) ValidateCreate(ctx context.Context, obj runtime.Object) (admission.Warnings, error) {
	pool, ok := obj.(*v1alpha2.VirtualMachinePool)
	if !ok {
		return nil, fmt.Errorf("expected a VirtualMachinePool but got a %T", obj)
	}
	return v.vmValidator.ValidateCreate(ctx, vmFromTemplate(pool))
}

func (v *poolValidator) ValidateUpdate(ctx context.Context, _, newObj runtime.Object) (admission.Warnings, error) {
	pool, ok := newObj.(*v1alpha2.VirtualMachinePool)
	if !ok {
		return nil, fmt.Errorf("expected a VirtualMachinePool but got a %T", newObj)
	}
	return v.vmValidator.ValidateCreate(ctx, vmFromTemplate(pool))
}

func (v *poolValidator) ValidateDelete(_ context.Context, _ runtime.Object) (admission.Warnings, error) {
	return nil, nil
}

// vmFromTemplate builds the VirtualMachine that the pool would create from its
// template, so the VM validators see the same spec a real replica would carry.
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
