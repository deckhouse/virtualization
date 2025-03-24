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
	"cmp"
	"context"
	"fmt"
	"log/slog"
	"slices"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	virtv1 "kubevirt.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/deckhouse/virtualization-controller/pkg/common/annotations"
	"github.com/deckhouse/virtualization-controller/pkg/common/object"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
)

func NewOneShotMigrationService(client client.Client, prefix string) *OneShotMigrationService {
	return &OneShotMigrationService{
		client: client,
		log:    slog.Default(),
		prefix: prefix,
	}
}

type OneShotMigrationService struct {
	client client.Client
	log    *slog.Logger
	prefix string
}

func (s *OneShotMigrationService) SetLogger(log *slog.Logger) {
	s.log = log
}

func (s *OneShotMigrationService) OnceMigrate(ctx context.Context, vm *v1alpha2.VirtualMachine, annoMarker, annoTrigger string) error {
	kvvmi := &virtv1.VirtualMachineInstance{}
	if err := s.client.Get(ctx, object.NamespacedName(vm), kvvmi); err != nil {
		return client.IgnoreNotFound(err)
	}

	desiredValue := kvvmi.GetAnnotations()[annoMarker]

	if desiredValue == annoTrigger {
		s.log.Debug("Migration already attempted for this trigger, skip...", slog.String("annotationValue", annoTrigger))
		return nil
	}

	workloadUpdateVmops, unmanagedVmops, err := s.listVmopMigrate(ctx, vm.GetName(), vm.GetNamespace())
	if err != nil {
		return err
	}

	if inProgressExist(unmanagedVmops) {
		s.log.Debug("VirtualMachine already migrate, skip...")
		return nil
	}

	if len(workloadUpdateVmops) > 0 {
		s.log.Debug("VirtualMachine already migrate by workload-updater, skip...")
		if desiredValue == "" {
			vmop := getInProgressOrFirst(workloadUpdateVmops)
			trigger := annoTrigger
			if ann := vmop.GetAnnotations()[annoMarker]; ann != "" {
				trigger = ann
			}
			if err = s.setTriggerToKVVMI(ctx, kvvmi, annoMarker, trigger); err != nil {
				return err
			}
		}
		return nil
	}

	s.log.Info("Create VMOP")

	vmop := newVMOP(s.prefix, vm.GetNamespace(), vm.GetName(), annoMarker, annoTrigger)
	if err = s.client.Create(ctx, vmop); err != nil {
		return err
	}

	if err = s.setTriggerToKVVMI(ctx, kvvmi, annoMarker, annoTrigger); err != nil {
		return err
	}

	return nil
}

func (s *OneShotMigrationService) listVmopMigrate(ctx context.Context, vmName, vmNamespace string) ([]v1alpha2.VirtualMachineOperation, []v1alpha2.VirtualMachineOperation, error) {
	vmopList := &v1alpha2.VirtualMachineOperationList{}
	if err := s.client.List(ctx, vmopList, client.InNamespace(vmNamespace)); err != nil {
		return nil, nil, fmt.Errorf("failed to list virtual machine operations: %w", err)
	}
	var (
		workloadUpdateVmops []v1alpha2.VirtualMachineOperation
		unmanagedVmops      []v1alpha2.VirtualMachineOperation
	)
	for _, vmop := range vmopList.Items {
		if vmop.Spec.VirtualMachine == vmName &&
			(vmop.Spec.Type == v1alpha2.VMOPTypeMigrate || vmop.Spec.Type == v1alpha2.VMOPTypeEvict) &&
			!vmopFinish(&vmop) {
			if _, exist := vmop.GetAnnotations()[annotations.AnnVMOPWorkloadUpdate]; exist {
				workloadUpdateVmops = append(workloadUpdateVmops, vmop)
			} else {
				unmanagedVmops = append(unmanagedVmops, vmop)
			}
		}
	}
	return workloadUpdateVmops, unmanagedVmops, nil
}

func (s *OneShotMigrationService) setTriggerToKVVMI(ctx context.Context, kvvmi *virtv1.VirtualMachineInstance, annoMarker, annoTrigger string) error {
	return object.PresentAnnotation(ctx, s.client, kvvmi, annoMarker, annoTrigger)
}

func vmopFinish(vmop *v1alpha2.VirtualMachineOperation) bool {
	return vmop == nil || vmop.Status.Phase == v1alpha2.VMOPPhaseCompleted || vmop.Status.Phase == v1alpha2.VMOPPhaseFailed
}

func inProgressExist(vmops []v1alpha2.VirtualMachineOperation) bool {
	for _, vmop := range vmops {
		if vmop.Status.Phase == v1alpha2.VMOPPhaseInProgress {
			return true
		}
	}
	return false
}

func getInProgressOrFirst(vmops []v1alpha2.VirtualMachineOperation) *v1alpha2.VirtualMachineOperation {
	for _, vmop := range vmops {
		if vmop.Status.Phase == v1alpha2.VMOPPhaseInProgress {
			return &vmop
		}
	}
	slices.SortFunc(vmops, func(a, b v1alpha2.VirtualMachineOperation) int {
		return cmp.Compare(a.GetCreationTimestamp().Unix(), b.GetCreationTimestamp().Unix())
	})
	return &vmops[0]
}

func newVMOP(prefix, namespace, vmName, annoMarker, annoTrigger string) *v1alpha2.VirtualMachineOperation {
	return &v1alpha2.VirtualMachineOperation{
		TypeMeta: metav1.TypeMeta{
			APIVersion: v1alpha2.SchemeGroupVersion.String(),
			Kind:       v1alpha2.VirtualMachineOperationKind,
		},
		ObjectMeta: metav1.ObjectMeta{
			GenerateName: prefix,
			Namespace:    namespace,
			Annotations: map[string]string{
				annotations.AnnVMOPWorkloadUpdate: "true",
				annoMarker:                        annoTrigger,
			},
		},
		Spec: v1alpha2.VirtualMachineOperationSpec{
			VirtualMachine: vmName,
			Type:           v1alpha2.VMOPTypeEvict,
		},
	}
}
