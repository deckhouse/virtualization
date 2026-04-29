/*
Copyright 2026 Flant JSC

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

package migration

import (
	"context"
	"log/slog"

	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	virtv1 "kubevirt.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/deckhouse/deckhouse/pkg/log"
	"github.com/deckhouse/virtualization-controller/pkg/controller/conditions"
	migrationstore "github.com/deckhouse/virtualization-controller/pkg/migration/store"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/vmcondition"
)

const vmRunningTransitionTimeMigrationName = "vm-running-transition-time"

func newVMRunningTransitionTime(client client.Client, logger *log.Logger) (Migration, error) {
	return &vmRunningTransitionTime{
		client: client,
		logger: logger,
		store:  migrationstore.NewConfigMapStore(client),
	}, nil
}

type vmRunningTransitionTime struct {
	client client.Client
	logger *log.Logger
	store  migrationstore.Store
}

func (m *vmRunningTransitionTime) Name() string {
	return vmRunningTransitionTimeMigrationName
}

func (m *vmRunningTransitionTime) Migrate(ctx context.Context) error {
	completed, err := m.store.IsCompleted(ctx, m.Name())
	if err != nil {
		return err
	}
	if completed {
		return nil
	}

	if err = m.migrate(ctx); err != nil {
		return err
	}

	return m.store.MarkCompleted(ctx, m.Name())
}

func (m *vmRunningTransitionTime) migrate(ctx context.Context) error {
	vmList := &v1alpha2.VirtualMachineList{}
	if err := m.client.List(ctx, vmList); err != nil {
		return err
	}

	for i := range vmList.Items {
		vm := &vmList.Items[i]
		cond := conditions.FindStatusCondition(vm.Status.Conditions, vmcondition.TypeRunning.String())
		if cond == nil || cond.Status != metav1.ConditionTrue || cond.Reason != vmcondition.ReasonVirtualMachineRunning.String() {
			continue
		}

		kvvmi := &virtv1.VirtualMachineInstance{}
		err := m.client.Get(ctx, types.NamespacedName{Name: vm.Name, Namespace: vm.Namespace}, kvvmi)
		if k8serrors.IsNotFound(err) {
			continue
		}
		if err != nil {
			return err
		}
		if kvvmi.CreationTimestamp.IsZero() || cond.LastTransitionTime.Equal(&kvvmi.CreationTimestamp) {
			continue
		}

		m.logger.Info("Update virtual machine Running condition transition time",
			slog.String("name", vm.Name),
			slog.String("namespace", vm.Namespace),
			slog.Time("lastTransitionTime", cond.LastTransitionTime.Time),
			slog.Time("creationTimestamp", kvvmi.CreationTimestamp.Time),
		)

		vm.Status.Conditions = replaceConditionLastTransitionTime(vm.Status.Conditions, vmcondition.TypeRunning.String(), kvvmi.CreationTimestamp)
		if err = m.client.Status().Update(ctx, vm); err != nil {
			return err
		}
	}

	return nil
}

func replaceConditionLastTransitionTime(conditions []metav1.Condition, conditionType string, lastTransitionTime metav1.Time) []metav1.Condition {
	res := make([]metav1.Condition, len(conditions))
	copy(res, conditions)
	for i := range res {
		if res[i].Type == conditionType {
			res[i].LastTransitionTime = lastTransitionTime
			break
		}
	}
	return res
}
