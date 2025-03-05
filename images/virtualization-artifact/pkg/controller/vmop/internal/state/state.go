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

package state

import (
	"context"
	"fmt"

	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/deckhouse/virtualization-controller/pkg/common/object"
	"github.com/deckhouse/virtualization-controller/pkg/controller/reconciler"
	virtv2 "github.com/deckhouse/virtualization/api/core/v1alpha2"
)

type VMOperationState interface {
	VirtualMachineOperation() *reconciler.Resource[*virtv2.VirtualMachineOperation, virtv2.VirtualMachineOperationStatus]
	VirtualMachine(ctx context.Context) (*virtv2.VirtualMachine, error)
}

func New(c client.Client, vmop *reconciler.Resource[*virtv2.VirtualMachineOperation, virtv2.VirtualMachineOperationStatus]) VMOperationState {
	return &state{client: c, vmop: vmop}
}

type state struct {
	client client.Client
	vmop   *reconciler.Resource[*virtv2.VirtualMachineOperation, virtv2.VirtualMachineOperationStatus]
	vm     *virtv2.VirtualMachine
}

func (s *state) VirtualMachineOperation() *reconciler.Resource[*virtv2.VirtualMachineOperation, virtv2.VirtualMachineOperationStatus] {
	return s.vmop
}

func (s *state) VirtualMachine(ctx context.Context) (*virtv2.VirtualMachine, error) {
	if s.vm != nil {
		return s.vm, nil
	}

	vmName := s.vmop.Current().Spec.VirtualMachine
	vmNs := s.vmop.Current().GetNamespace()
	vm, err := object.FetchObject(ctx,
		types.NamespacedName{Name: vmName, Namespace: vmNs},
		s.client,
		&virtv2.VirtualMachine{})
	if err != nil {
		return nil, fmt.Errorf("unable to get VM %q: %w", vmName, err)
	}
	s.vm = vm

	return s.vm, nil
}
