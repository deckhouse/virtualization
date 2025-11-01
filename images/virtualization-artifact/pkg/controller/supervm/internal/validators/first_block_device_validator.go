/*
Copyright 2025 Flant JSC

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

package validators

import (
	"context"
	"fmt"
	"reflect"

	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	"github.com/deckhouse/virtualization-controller/pkg/common/object"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
)

const (
	viMainErrorMessage  = "a non-CDROM VirtualImage cannot occupy the first position in block devices"
	cviMainErrorMessage = "a non-CDROM ClusterVirtualImage cannot occupy the first position in block devices"
)

type FirstBlockDeviceValidator struct {
	client client.Client
}

func NewFirstDiskValidator(client client.Client) *FirstBlockDeviceValidator {
	return &FirstBlockDeviceValidator{client: client}
}

func (v *FirstBlockDeviceValidator) ValidateCreate(ctx context.Context, vm *v1alpha2.VirtualMachine) (admission.Warnings, error) {
	return v.Validate(ctx, vm)
}

func (v *FirstBlockDeviceValidator) ValidateUpdate(ctx context.Context, oldVM, newVM *v1alpha2.VirtualMachine) (admission.Warnings, error) {
	if reflect.DeepEqual(oldVM.Spec.BlockDeviceRefs, newVM.Spec.BlockDeviceRefs) {
		return nil, nil
	}

	return v.Validate(ctx, newVM)
}

func (v *FirstBlockDeviceValidator) Validate(ctx context.Context, vm *v1alpha2.VirtualMachine) (admission.Warnings, error) {
	if len(vm.Spec.BlockDeviceRefs) == 0 {
		return nil, nil
	}

	switch vm.Spec.BlockDeviceRefs[0].Kind {
	case v1alpha2.ImageDevice:
		return nil, v.ValidateVI(ctx, vm.Spec.BlockDeviceRefs[0].Name, vm.GetNamespace())
	case v1alpha2.ClusterImageDevice:
		return nil, v.ValidateCVI(ctx, vm.Spec.BlockDeviceRefs[0].Name)
	}

	return nil, nil
}

func (v *FirstBlockDeviceValidator) ValidateCVI(ctx context.Context, name string) error {
	cvi, err := object.FetchObject(ctx, types.NamespacedName{Name: name}, v.client, &v1alpha2.ClusterVirtualImage{})
	if err != nil {
		return err
	}
	if cvi == nil {
		return nil
	}

	if !cvi.Status.CDROM && cvi.Status.Phase == v1alpha2.ImageReady {
		return fmt.Errorf(
			"%s: ClusterVirtualImage %s is not CDROM",
			cviMainErrorMessage,
			name,
		)
	}

	return nil
}

func (v *FirstBlockDeviceValidator) ValidateVI(ctx context.Context, name, namespace string) error {
	vi, err := object.FetchObject(ctx, types.NamespacedName{Name: name, Namespace: namespace}, v.client, &v1alpha2.VirtualImage{})
	if err != nil {
		return err
	}
	if vi == nil {
		return nil
	}

	if !vi.Status.CDROM && vi.Status.Phase == v1alpha2.ImageReady {
		return fmt.Errorf(
			"%s: VirtualImage %s is not CDROM",
			viMainErrorMessage,
			name,
		)
	}

	return nil
}
