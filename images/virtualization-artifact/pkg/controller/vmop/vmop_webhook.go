/*
Copyright 2024 Flant JSC

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

package vmop

import (
	"context"
	"errors"
	"fmt"
	"slices"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/validation"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	"github.com/deckhouse/deckhouse/pkg/log"
	commonvmop "github.com/deckhouse/virtualization-controller/pkg/common/vmop"
	"github.com/deckhouse/virtualization-controller/pkg/controller/validator"
	"github.com/deckhouse/virtualization-controller/pkg/featuregates"
	"github.com/deckhouse/virtualization-controller/pkg/version"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
)

func NewValidator(c client.Client, log *log.Logger) admission.CustomValidator {
	return validator.NewValidator[*v1alpha2.VirtualMachineOperation](log.
		With("controller", "vmop-controller").
		With("webhook", "validation"),
	).WithCreateValidators(
		&nodeSelectorValidator{},
		&localStorageMigrationValidator{client: c},
	)
}

type nodeSelectorValidator struct{}

func (n *nodeSelectorValidator) ValidateCreate(_ context.Context, vmop *v1alpha2.VirtualMachineOperation) (admission.Warnings, error) {
	if vmop.Spec.Migrate != nil && vmop.Spec.Migrate.NodeSelector != nil {
		if !featuregates.Default().Enabled(featuregates.TargetMigration) {
			return admission.Warnings{}, errors.New("the `nodeSelector` field is not available in the Community Edition version")
		}

		err := n.validateNodeSelector(vmop.Spec.Migrate.NodeSelector)
		if err != nil {
			return admission.Warnings{}, err
		}
	}

	return admission.Warnings{}, nil
}

func (n *nodeSelectorValidator) validateNodeSelector(nodeSelector map[string]string) error {
	for k, v := range nodeSelector {
		if errs := validation.IsQualifiedName(k); len(errs) != 0 {
			return fmt.Errorf("invalid label key in the `nodeSelector` field: %v", errs)
		}

		if errs := validation.IsValidLabelValue(v); len(errs) != 0 {
			return fmt.Errorf("invalid label value in the `nodeSelector` field: %v", errs)
		}
	}

	return nil
}

type localStorageMigrationValidator struct {
	client client.Client
}

func (v *localStorageMigrationValidator) ValidateCreate(ctx context.Context, vmop *v1alpha2.VirtualMachineOperation) (admission.Warnings, error) {
	if version.GetEdition() != version.EditionCE {
		return nil, nil
	}

	if !commonvmop.IsMigration(vmop) {
		return nil, nil
	}

	vm := &v1alpha2.VirtualMachine{}
	err := v.client.Get(ctx, types.NamespacedName{Name: vmop.Spec.VirtualMachine, Namespace: vmop.Namespace}, vm)
	if err != nil {
		return nil, fmt.Errorf("failed to get VirtualMachine: %w", err)
	}

	for _, bda := range vm.Status.BlockDeviceRefs {
		if bda.Kind != v1alpha2.DiskDevice {
			continue
		}

		vd := &v1alpha2.VirtualDisk{}
		err := v.client.Get(ctx, types.NamespacedName{Name: bda.Name, Namespace: vmop.Namespace}, vd)
		if err != nil {
			return nil, fmt.Errorf("failed to get VirtualDisk %s: %w", bda.Name, err)
		}

		if vd.Status.Target.PersistentVolumeClaim == "" {
			continue
		}

		pvc := &corev1.PersistentVolumeClaim{}
		err = v.client.Get(ctx, types.NamespacedName{Name: vd.Status.Target.PersistentVolumeClaim, Namespace: vmop.Namespace}, pvc)
		if err != nil {
			return nil, fmt.Errorf("failed to get PersistentVolumeClaim %s: %w", vd.Status.Target.PersistentVolumeClaim, err)
		}

		if !slices.Contains(pvc.Spec.AccessModes, corev1.ReadWriteMany) {
			return nil, fmt.Errorf("migration of VirtualMachines with local (RWO) storage is only available in the Enterprise Edition (EE)")
		}
	}

	return nil, nil
}
