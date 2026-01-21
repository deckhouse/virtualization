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

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/ptr"
	virtv1 "kubevirt.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/deckhouse/virtualization-controller/pkg/common/object"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
)

type MigrationService struct {
	client client.Client
}

func NewMigrationService(client client.Client) *MigrationService {
	return &MigrationService{
		client: client,
	}
}

func (s MigrationService) IsApplicableForVMPhase(phase v1alpha2.MachinePhase) bool {
	return phase == v1alpha2.MachineRunning
}

func (s MigrationService) IsApplicableForRunPolicy(runPolicy v1alpha2.RunPolicy) bool {
	return runPolicy == v1alpha2.ManualPolicy ||
		runPolicy == v1alpha2.AlwaysOnUnlessStoppedManually ||
		runPolicy == v1alpha2.AlwaysOnPolicy
}

func (s MigrationService) CreateMigration(ctx context.Context, vmop *v1alpha2.VirtualMachineOperation) error {
	vmim := &virtv1.VirtualMachineInstanceMigration{
		TypeMeta: metav1.TypeMeta{
			APIVersion: virtv1.SchemeGroupVersion.String(),
			Kind:       "VirtualMachineInstanceMigration",
		},
		ObjectMeta: metav1.ObjectMeta{
			Namespace: vmop.GetNamespace(),
			Name:      migrationName(vmop),
			OwnerReferences: []metav1.OwnerReference{
				{
					APIVersion:         v1alpha2.SchemeGroupVersion.String(),
					Kind:               v1alpha2.VirtualMachineOperationKind,
					Name:               vmop.GetName(),
					UID:                vmop.GetUID(),
					BlockOwnerDeletion: ptr.To(true),
					Controller:         ptr.To(true),
				},
			},
		},
		Spec: virtv1.VirtualMachineInstanceMigrationSpec{
			VMIName: vmop.Spec.VirtualMachine,
		},
	}

	if vmop.Spec.Migrate != nil && vmop.Spec.Migrate.NodeSelector != nil {
		vmim.Spec.AddedNodeSelector = vmop.Spec.Migrate.NodeSelector
	}

	return client.IgnoreAlreadyExists(s.client.Create(ctx, vmim))
}

func (s MigrationService) DeleteMigration(ctx context.Context, vmop *v1alpha2.VirtualMachineOperation) error {
	mig, err := s.GetMigration(ctx, vmop)
	if err != nil {
		return err
	}
	if mig == nil {
		return nil
	}
	err = s.client.Delete(ctx, mig)
	return client.IgnoreNotFound(err)
}

func (s MigrationService) GetMigration(ctx context.Context, vmop *v1alpha2.VirtualMachineOperation) (*virtv1.VirtualMachineInstanceMigration, error) {
	return object.FetchObject(ctx, types.NamespacedName{
		Name:      migrationName(vmop),
		Namespace: vmop.GetNamespace(),
	}, s.client, &virtv1.VirtualMachineInstanceMigration{})
}

const vmopPrefix = "vmop-"

func migrationName(vmop *v1alpha2.VirtualMachineOperation) string {
	return fmt.Sprintf("%s%s", vmopPrefix, vmop.GetName())
}
