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

package service

import (
	"context"

	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/deckhouse/virtualization-controller/pkg/controller/indexer"
	virtv2 "github.com/deckhouse/virtualization/api/core/v1alpha2"
)

type BlockDeviceService struct {
	client client.Client
}

func NewBlockDeviceService(client client.Client) *BlockDeviceService {
	return &BlockDeviceService{
		client: client,
	}
}

//nolint:stylecheck // TODO: fix to CountBlockDevicesAttachedToVM
func (s *BlockDeviceService) CountBlockDevicesAttachedToVm(ctx context.Context, vm *virtv2.VirtualMachine) (int, error) {
	count := len(vm.Spec.BlockDeviceRefs)

	var vmbdaList virtv2.VirtualMachineBlockDeviceAttachmentList

	err := s.client.List(ctx, &vmbdaList, client.InNamespace(vm.Namespace),
		&client.MatchingFields{
			indexer.IndexFieldVMBDAByVM: vm.Name,
		})
	if err != nil {
		return 0, err
	}

	count += len(vmbdaList.Items)

	return count, nil
}

//nolint:stylecheck // TODO: fix to CountBlockDevicesAttachedToVMName
func (s *BlockDeviceService) CountBlockDevicesAttachedToVmName(ctx context.Context, vmName, namespace string) (int, error) {
	count := 0
	var vm virtv2.VirtualMachine

	err := s.client.Get(ctx, client.ObjectKey{Name: vmName, Namespace: namespace}, &vm)
	if err == nil {
		count += len(vm.Spec.BlockDeviceRefs)
	} else if !k8serrors.IsNotFound(err) {
		return 0, err
	}

	var vmbdaList virtv2.VirtualMachineBlockDeviceAttachmentList

	err = s.client.List(ctx, &vmbdaList, client.InNamespace(namespace),
		&client.MatchingFields{
			indexer.IndexFieldVMBDAByVM: vmName,
		})
	if err != nil {
		return 0, err
	}

	count += len(vmbdaList.Items)

	return count, nil
}
