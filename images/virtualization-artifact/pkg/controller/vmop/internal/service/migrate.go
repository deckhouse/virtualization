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

	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/ptr"
	virtv1 "kubevirt.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/deckhouse/virtualization-controller/pkg/common/object"
	commonvmop "github.com/deckhouse/virtualization-controller/pkg/common/vmop"
	"github.com/deckhouse/virtualization-controller/pkg/controller/conditions"
	virtv2 "github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/vmopcondition"
)

func NewMigrateOperation(client client.Client, vmop *virtv2.VirtualMachineOperation) *MigrateOperation {
	return &MigrateOperation{
		client: client,
		vmop:   vmop,
	}
}

type MigrateOperation struct {
	client client.Client
	vmop   *virtv2.VirtualMachineOperation
}

func (o MigrateOperation) Do(ctx context.Context) error {
	return o.createMigration(ctx)
}

func (o MigrateOperation) Cancel(ctx context.Context) (bool, error) {
	mig, err := o.getMigration(ctx)
	if err != nil {
		return false, err
	}
	if mig == nil {
		return true, nil
	}
	err = o.client.Delete(ctx, mig)
	if k8serrors.IsNotFound(err) {
		return true, nil
	}
	return false, err
}

func (o MigrateOperation) IsApplicableForVMPhase(phase virtv2.MachinePhase) bool {
	return phase == virtv2.MachineRunning
}

func (o MigrateOperation) IsApplicableForRunPolicy(runPolicy virtv2.RunPolicy) bool {
	return runPolicy == virtv2.ManualPolicy ||
		runPolicy == virtv2.AlwaysOnUnlessStoppedManually ||
		runPolicy == virtv2.AlwaysOnPolicy
}

func (o MigrateOperation) GetInProgressReason(ctx context.Context) (vmopcondition.ReasonCompleted, error) {
	migration, err := o.getMigration(ctx)
	if err != nil {
		return vmopcondition.ReasonCompleted(conditions.ReasonUnknown), err
	}
	if migration == nil {
		return vmopcondition.ReasonMigrationPending, nil
	}
	reason := mapMigrationPhaseToReason[migration.Status.Phase]
	return reason, nil
}

func (o MigrateOperation) IsFinalState() bool {
	return isFinalState(o.vmop)
}

func (o MigrateOperation) IsComplete(ctx context.Context) (bool, string, error) {
	mig, err := o.getMigration(ctx)
	if err != nil {
		return false, "", err
	}
	if mig != nil {
		switch mig.Status.Phase {
		case virtv1.MigrationSucceeded:
			return true, "", nil
		case virtv1.MigrationFailed:
			msg := "Migration failed"
			if mig.Status.MigrationState != nil && mig.Status.MigrationState.FailureReason != "" {
				msg += ": " + mig.Status.MigrationState.FailureReason
			}
			return true, msg + ".", nil
		default:
			return false, "", nil
		}
	}
	kvvmi := &virtv1.VirtualMachineInstance{}
	if err = o.client.Get(ctx, virtualMachineKeyByVmop(o.vmop), kvvmi); err != nil {
		if k8serrors.IsNotFound(err) {
			return false, "Migration failed because the virtual machine is currently not running.", nil
		}
		return false, "", err
	}

	migrationState := kvvmi.Status.MigrationState
	if migrationState != nil && migrationState.EndTimestamp != nil && isAfterSignalSentOrCreation(migrationState.EndTimestamp.Time, o.vmop) {
		reason := ""
		if migrationState.Failed {
			reason = fmt.Sprintf("Migration failed: %s", migrationState.FailureReason)
		}
		return true, reason, nil
	}
	return true, "Migration was canceled", nil
}

func (o MigrateOperation) createMigration(ctx context.Context) error {
	return o.client.Create(ctx, &virtv1.VirtualMachineInstanceMigration{
		TypeMeta: metav1.TypeMeta{
			APIVersion: virtv1.SchemeGroupVersion.String(),
			Kind:       "VirtualMachineInstanceMigration",
		},
		ObjectMeta: metav1.ObjectMeta{
			Namespace: o.vmop.GetNamespace(),
			Name:      KubevirtMigrationName(o.vmop),
			OwnerReferences: []metav1.OwnerReference{
				{
					APIVersion:         virtv2.SchemeGroupVersion.String(),
					Kind:               virtv2.VirtualMachineOperationKind,
					Name:               o.vmop.GetName(),
					UID:                o.vmop.GetUID(),
					BlockOwnerDeletion: ptr.To(true),
					Controller:         ptr.To(true),
				},
			},
		},
		Spec: virtv1.VirtualMachineInstanceMigrationSpec{
			VMIName: o.vmop.Spec.VirtualMachine,
		},
	})
}

func (o MigrateOperation) getMigration(ctx context.Context) (*virtv1.VirtualMachineInstanceMigration, error) {
	return object.FetchObject(ctx, types.NamespacedName{
		Name:      KubevirtMigrationName(o.vmop),
		Namespace: o.vmop.GetNamespace(),
	}, o.client, &virtv1.VirtualMachineInstanceMigration{})
}

const vmopPrefix = "vmop-"

func KubevirtMigrationName(vmop *virtv2.VirtualMachineOperation) string {
	return fmt.Sprintf("%s%s", vmopPrefix, vmop.GetName())
}

var mapMigrationPhaseToReason = map[virtv1.VirtualMachineInstanceMigrationPhase]vmopcondition.ReasonCompleted{
	virtv1.MigrationPhaseUnset:  vmopcondition.ReasonMigrationPending,
	virtv1.MigrationPending:     vmopcondition.ReasonMigrationPending,
	virtv1.MigrationScheduling:  vmopcondition.ReasonMigrationPrepareTarget,
	virtv1.MigrationScheduled:   vmopcondition.ReasonMigrationPrepareTarget,
	virtv1.MigrationTargetReady: vmopcondition.ReasonMigrationTargetReady,
	virtv1.MigrationRunning:     vmopcondition.ReasonMigrationRunning,
	virtv1.MigrationSucceeded:   vmopcondition.ReasonOperationCompleted,
	virtv1.MigrationFailed:      vmopcondition.ReasonOperationFailed,
}

func IsKubeVirtMigrationRejectedDueToQuota(ctx context.Context, client client.Client, vmop *virtv2.VirtualMachineOperation) (bool, error) {
	if !commonvmop.IsMigration(vmop) {
		return false, nil
	}

	kubevirtMigrationName := KubevirtMigrationName(vmop)
	kubevirtMigration, err := object.FetchObject(ctx, types.NamespacedName{
		Namespace: vmop.GetNamespace(),
		Name:      kubevirtMigrationName,
	}, client, &virtv1.VirtualMachineInstanceMigration{})
	if err != nil {
		return false, err
	}

	if kubevirtMigration == nil {
		return false, nil
	}

	_, ok := conditions.GetKVVMIMCondition(conditions.KubevirtMigrationRejectedByResourceQuotaType, kubevirtMigration.Status.Conditions)
	if ok {
		return true, nil
	}

	return false, nil
}
