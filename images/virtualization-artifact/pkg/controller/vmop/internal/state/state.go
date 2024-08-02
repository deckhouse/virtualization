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
	"sync"

	"k8s.io/apimachinery/pkg/types"
	virtv1 "kubevirt.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/deckhouse/virtualization-controller/pkg/controller/service"
	"github.com/deckhouse/virtualization-controller/pkg/sdk/framework/helper"
	virtv2 "github.com/deckhouse/virtualization/api/core/v1alpha2"
)

type VMOperationState interface {
	VMOP() *service.Resource[*virtv2.VirtualMachineOperation, virtv2.VirtualMachineOperationStatus]
	VirtualMachine(ctx context.Context) (*virtv2.VirtualMachine, error)
	KVVM(ctx context.Context) (*virtv1.VirtualMachine, error)
	KVVMI(ctx context.Context) (*virtv1.VirtualMachineInstance, error)
	OtherVMOPIsInProgress(ctx context.Context) (bool, error)
}

func New(c client.Client, vmop *service.Resource[*virtv2.VirtualMachineOperation, virtv2.VirtualMachineOperationStatus]) VMOperationState {
	return &state{client: c, vmop: vmop}
}

type state struct {
	client client.Client
	mu     sync.RWMutex
	vmop   *service.Resource[*virtv2.VirtualMachineOperation, virtv2.VirtualMachineOperationStatus]
	vm     *virtv2.VirtualMachine
	kvvm   *virtv1.VirtualMachine
	kvvmi  *virtv1.VirtualMachineInstance
}

func (s *state) VMOP() *service.Resource[*virtv2.VirtualMachineOperation, virtv2.VirtualMachineOperationStatus] {
	return s.vmop
}

func (s *state) VirtualMachine(ctx context.Context) (*virtv2.VirtualMachine, error) {
	if s.vm != nil {
		return s.vm, nil
	}

	vmName := s.vmop.Current().Spec.VirtualMachine
	vmNs := s.vmop.Current().GetNamespace()
	vm, err := helper.FetchObject(ctx,
		types.NamespacedName{Name: vmName, Namespace: vmNs},
		s.client,
		&virtv2.VirtualMachine{})
	if err != nil {
		return nil, fmt.Errorf("unable to get VM %q: %w", vmName, err)
	}
	s.vm = vm

	return s.vm, nil
}

func (s *state) KVVM(ctx context.Context) (*virtv1.VirtualMachine, error) {
	if s.vmop == nil || s.vm == nil {
		return nil, nil
	}
	if s.kvvm != nil {
		return s.kvvm, nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	kvvm := &virtv1.VirtualMachine{}
	key := types.NamespacedName{
		Name:      s.vmop.Current().Spec.VirtualMachine,
		Namespace: s.vmop.Current().GetNamespace(),
	}
	err := s.client.Get(ctx, key, kvvm)
	if err != nil {
		return nil, err
	}
	s.kvvm = kvvm
	return kvvm, nil
}

func (s *state) KVVMI(ctx context.Context) (*virtv1.VirtualMachineInstance, error) {
	if s.vmop == nil || s.vm == nil {
		return nil, nil
	}
	if s.kvvmi != nil {
		return s.kvvmi, nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	kvvmi := &virtv1.VirtualMachineInstance{}
	key := types.NamespacedName{
		Name:      s.vmop.Current().Spec.VirtualMachine,
		Namespace: s.vmop.Current().GetNamespace(),
	}
	err := s.client.Get(ctx, key, kvvmi)
	if err != nil {
		return nil, err
	}
	s.kvvmi = kvvmi
	return kvvmi, nil
}

// OtherVMOPIsInProgress check if there is at least one VMOP for the same VM in progress phase.
func (s *state) OtherVMOPIsInProgress(ctx context.Context) (bool, error) {
	var vmops virtv2.VirtualMachineOperationList
	err := s.client.List(ctx, &vmops, client.InNamespace(s.vmop.Current().GetNamespace()))
	if err != nil {
		return false, err
	}
	vmName := s.vmop.Current().Spec.VirtualMachine

	for _, vmop := range vmops.Items {
		if vmop.GetName() == s.vmop.Current().GetName() || vmop.Spec.VirtualMachine != vmName {
			continue
		}
		if vmop.Status.Phase == virtv2.VMOPPhaseInProgress {
			return true, nil
		}
	}
	return false, nil
}
