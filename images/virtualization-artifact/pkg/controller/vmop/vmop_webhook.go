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

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	"github.com/deckhouse/deckhouse/pkg/log"
	"github.com/deckhouse/virtualization-controller/pkg/common/object"
	"github.com/deckhouse/virtualization-controller/pkg/controller/validator"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
)

func NewValidator(c client.Client, log *log.Logger) admission.CustomValidator {
	return validator.NewValidator[*v1alpha2.VirtualMachineOperation](log.
		With("controller", "vmop-controller").
		With("webhook", "validation"),
	).WithCreateValidators(&deprecateMigrateValidator{}, NewLocalVirtualDiskValidator(c))
}

type LocalVirtualDiskValidator struct {
	client client.Client
}

func NewLocalVirtualDiskValidator(client client.Client) *LocalVirtualDiskValidator {
	return &LocalVirtualDiskValidator{client: client}
}

func (v *LocalVirtualDiskValidator) ValidateCreate(ctx context.Context, vmop *v1alpha2.VirtualMachineOperation) (admission.Warnings, error) {
	if vmop.Spec.Type != v1alpha2.VMOPTypeEvict && vmop.Spec.Type != v1alpha2.VMOPTypeMigrate {
		return nil, nil
	}

	vm, err := object.FetchObject(ctx, types.NamespacedName{
		Namespace: vmop.Namespace,
		Name:      vmop.Spec.VirtualMachine,
	}, v.client, &v1alpha2.VirtualMachine{})
	if err != nil {
		return nil, fmt.Errorf("failed to fetch virtual machine %s: %w", vmop.Spec.VirtualMachine, err)
	}

	if vm == nil {
		return nil, nil
	}

	var hasHotplugs bool
	var hasRWO bool

	for _, bdRef := range vm.Status.BlockDeviceRefs {
		if bdRef.Hotplugged {
			hasHotplugs = true
		}

		switch bdRef.Kind {
		case v1alpha2.VirtualDiskKind:
			var vd *v1alpha2.VirtualDisk
			vd, err = object.FetchObject(ctx, types.NamespacedName{
				Namespace: vm.Namespace,
				Name:      bdRef.Name,
			}, v.client, &v1alpha2.VirtualDisk{})
			if err != nil {
				return nil, fmt.Errorf("failed to fetch virtual disk %s: %w", bdRef.Name, err)
			}

			if vd == nil || vd.Status.Target.PersistentVolumeClaim == "" {
				return nil, nil
			}

			var isRWO bool
			isRWO, err = v.isRWOPersistentVolumeClaim(ctx, vd.Status.Target.PersistentVolumeClaim, vm.Namespace)
			if err != nil {
				return nil, err
			}

			hasRWO = hasRWO || isRWO
		case v1alpha2.VirtualImageKind:
			var vi *v1alpha2.VirtualImage
			vi, err = object.FetchObject(ctx, types.NamespacedName{
				Namespace: vm.Namespace,
				Name:      bdRef.Name,
			}, v.client, &v1alpha2.VirtualImage{})
			if err != nil {
				return nil, fmt.Errorf("failed to fetch virtual image %s: %w", bdRef.Name, err)
			}

			if vi == nil || vi.Status.Target.PersistentVolumeClaim == "" {
				return nil, nil
			}

			var isRWO bool
			isRWO, err = v.isRWOPersistentVolumeClaim(ctx, vi.Status.Target.PersistentVolumeClaim, vm.Namespace)
			if err != nil {
				return nil, err
			}

			hasRWO = hasRWO || isRWO
		}
	}

	if hasRWO && hasHotplugs {
		return nil, errors.New("for now, migration of the rwo virtual disk is not allowed if the virtual machine has hot-plugged block devices")
	}

	return nil, nil
}

func (v *LocalVirtualDiskValidator) isRWOPersistentVolumeClaim(ctx context.Context, pvcName, pvcNamespace string) (bool, error) {
	pvc, err := object.FetchObject(ctx, types.NamespacedName{
		Namespace: pvcNamespace,
		Name:      pvcName,
	}, v.client, &corev1.PersistentVolumeClaim{})
	if err != nil {
		return false, fmt.Errorf("failed to fetch pvc %s: %w", pvcName, err)
	}

	if pvc == nil {
		return false, nil
	}

	for _, mode := range pvc.Status.AccessModes {
		if mode == corev1.ReadWriteOnce {
			return true, nil
		}
	}

	return false, nil
}

type deprecateMigrateValidator struct{}

func (v *deprecateMigrateValidator) ValidateCreate(_ context.Context, vmop *v1alpha2.VirtualMachineOperation) (admission.Warnings, error) {
	// TODO: Delete me after v0.15
	if vmop.Spec.Type == v1alpha2.VMOPTypeMigrate {
		return admission.Warnings{"The Migrate type is deprecated, consider using Evict operation"}, nil
	}

	return admission.Warnings{}, nil
}
