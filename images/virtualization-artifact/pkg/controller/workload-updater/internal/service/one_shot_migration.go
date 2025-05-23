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

package service

import (
	"context"
	"fmt"
	"log/slog"

	virtv1 "kubevirt.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	vmopbuilder "github.com/deckhouse/virtualization-controller/pkg/builder/vmop"
	"github.com/deckhouse/virtualization-controller/pkg/common/annotations"
	"github.com/deckhouse/virtualization-controller/pkg/common/object"
	commonvmop "github.com/deckhouse/virtualization-controller/pkg/common/vmop"
	"github.com/deckhouse/virtualization-controller/pkg/logger"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
)

func NewOneShotMigrationService(client client.Client, prefix string) *OneShotMigrationService {
	return &OneShotMigrationService{
		client: client,
		prefix: prefix,
	}
}

type OneShotMigrationService struct {
	client client.Client
	prefix string
}

func (s *OneShotMigrationService) OnceMigrate(ctx context.Context, vm *v1alpha2.VirtualMachine, annotationKey, annotationExpectedValue string) (bool, error) {
	kvvmi := &virtv1.VirtualMachineInstance{}
	if err := s.client.Get(ctx, object.NamespacedName(vm), kvvmi); err != nil {
		return false, client.IgnoreNotFound(err)
	}

	desiredValue := kvvmi.GetAnnotations()[annotationKey]

	log := logger.FromContext(ctx)

	if desiredValue == annotationExpectedValue {
		log.Debug("Migration already attempted for this trigger. Skipping...",
			slog.String("annotationKey", annotationKey),
			slog.String("annotationValue", annotationExpectedValue))
		return false, nil
	}

	workloadUpdateVMOPs, unmanagedVMOPs, err := s.listVMOPMigrate(ctx, vm.GetName(), vm.GetNamespace())
	if err != nil {
		return false, err
	}

	if commonvmop.InProgressOrPendingExists(unmanagedVMOPs) {
		log.Debug("The virtual machine is either in the process of migration or waiting to start migration. Skipping...")
		return false, nil
	}

	if len(workloadUpdateVMOPs) > 0 {
		log.Debug("The virtual machine is either being migrated by the workload updater or is scheduled for migration. Skipping...")
	} else {
		log.Info("Create VMOP")
		vmop := newVMOP(s.prefix, vm.GetNamespace(), vm.GetName())
		if err = s.client.Create(ctx, vmop); err != nil {
			return false, err
		}
	}

	if err = s.setAnnoExpectedValueToKVVMI(ctx, kvvmi, annotationKey, annotationExpectedValue); err != nil {
		return false, err
	}

	return true, nil
}

func (s *OneShotMigrationService) listVMOPMigrate(ctx context.Context, vmName, vmNamespace string) ([]v1alpha2.VirtualMachineOperation, []v1alpha2.VirtualMachineOperation, error) {
	vmopList := &v1alpha2.VirtualMachineOperationList{}
	if err := s.client.List(ctx, vmopList, client.InNamespace(vmNamespace)); err != nil {
		return nil, nil, fmt.Errorf("failed to list virtual machine operations: %w", err)
	}
	var (
		workloadUpdateVMOPs []v1alpha2.VirtualMachineOperation
		unmanagedVMOPs      []v1alpha2.VirtualMachineOperation
	)
	for _, vmop := range vmopList.Items {
		if vmop.Spec.VirtualMachine == vmName && commonvmop.IsMigration(&vmop) && !commonvmop.IsFinished(&vmop) {
			if _, exists := vmop.GetAnnotations()[annotations.AnnVMOPWorkloadUpdate]; exists {
				workloadUpdateVMOPs = append(workloadUpdateVMOPs, vmop)
			} else {
				unmanagedVMOPs = append(unmanagedVMOPs, vmop)
			}
		}
	}
	return workloadUpdateVMOPs, unmanagedVMOPs, nil
}

func (s *OneShotMigrationService) setAnnoExpectedValueToKVVMI(ctx context.Context, kvvmi *virtv1.VirtualMachineInstance, annotationKey, annotationExpectedValue string) error {
	return object.EnsureAnnotation(ctx, s.client, kvvmi, annotationKey, annotationExpectedValue)
}

func newVMOP(prefix, namespace, vmName string) *v1alpha2.VirtualMachineOperation {
	return vmopbuilder.New(
		vmopbuilder.WithGenerateName(prefix),
		vmopbuilder.WithNamespace(namespace),
		vmopbuilder.WithAnnotation(annotations.AnnVMOPWorkloadUpdate, "true"),
		vmopbuilder.WithType(v1alpha2.VMOPTypeEvict),
		vmopbuilder.WithVirtualMachine(vmName),
	)
}
