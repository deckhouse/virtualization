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
	"strings"

	corev1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	virtv1 "kubevirt.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	vmopbuilder "github.com/deckhouse/virtualization-controller/pkg/builder/vmop"
	"github.com/deckhouse/virtualization-controller/pkg/common/annotations"
	"github.com/deckhouse/virtualization-controller/pkg/common/object"
	commonvmop "github.com/deckhouse/virtualization-controller/pkg/common/vmop"
	"github.com/deckhouse/virtualization-controller/pkg/eventrecord"
	"github.com/deckhouse/virtualization-controller/pkg/logger"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
)

// maxMigrationAttempts limits how many failed migrations a single workload-update
// trigger may cause before the update is given up until the next trigger change.
const maxMigrationAttempts = 3

func NewOneShotMigrationService(client client.Client, recorder eventrecord.EventRecorderLogger, prefix string) *OneShotMigrationService {
	return &OneShotMigrationService{
		client:   client,
		recorder: recorder,
		prefix:   prefix,
	}
}

type OneShotMigrationService struct {
	client   client.Client
	recorder eventrecord.EventRecorderLogger
	prefix   string
}

// OnceMigrate ensures a live migration for the given workload-update trigger
// (annotationKey with annotationExpectedValue). A trigger is considered handled
// only when a migration created for it completes successfully or fails
// maxMigrationAttempts times; the annotationExpectedValue is then recorded on
// the KVVMI so the trigger never fires a migration again.
func (s *OneShotMigrationService) OnceMigrate(ctx context.Context, vm *v1alpha2.VirtualMachine, annotationKey, annotationExpectedValue string) (bool, error) {
	kvvmi := &virtv1.VirtualMachineInstance{}
	if err := s.client.Get(ctx, object.NamespacedName(vm), kvvmi); err != nil {
		return false, client.IgnoreNotFound(err)
	}

	desiredValue := kvvmi.GetAnnotations()[annotationKey]

	log := logger.FromContext(ctx)

	if desiredValue == annotationExpectedValue {
		log.Debug("Migration already handled for this trigger. Skipping...",
			slog.String("annotationKey", annotationKey),
			slog.String("annotationValue", annotationExpectedValue))
		return false, nil
	}

	vmops, err := s.listVMOPMigrate(ctx, vm.GetName(), vm.GetNamespace())
	if err != nil {
		return false, err
	}

	var (
		activeExists   bool
		succeeded      bool
		failedAttempts int
	)
	for _, vmop := range vmops {
		if !commonvmop.IsFinished(&vmop) {
			activeExists = true
			continue
		}
		if _, managed := vmop.GetAnnotations()[annotations.AnnVMOPWorkloadUpdate]; !managed {
			continue
		}
		if vmop.GetAnnotations()[annotationKey] != annotationExpectedValue {
			continue
		}
		switch vmop.Status.Phase {
		case v1alpha2.VMOPPhaseCompleted:
			succeeded = true
		case v1alpha2.VMOPPhaseFailed:
			failedAttempts++
		}
	}

	if succeeded {
		return false, s.setAnnoExpectedValueToKVVMI(ctx, kvvmi, annotationKey, annotationExpectedValue)
	}

	if failedAttempts >= maxMigrationAttempts {
		log.Warn("Migration for the workload update failed too many times. Giving up until the trigger changes.",
			slog.String("annotationKey", annotationKey),
			slog.Int("failedAttempts", failedAttempts))
		s.recorder.WithLogging(log).Eventf(vm, corev1.EventTypeWarning, v1alpha2.ReasonWorkloadUpdateFailed,
			"Live migration for the workload update failed %d times, giving up; the VirtualMachine may require a restart to apply the update.", failedAttempts)
		return false, s.setAnnoExpectedValueToKVVMI(ctx, kvvmi, annotationKey, annotationExpectedValue)
	}

	// Any unfinished operation blocks a new one on the vmop webhook side,
	// including a Terminating operation (e.g. an evict whose migration is being
	// cancelled), so trying to create a VMOP while one exists is guaranteed to
	// be denied. Wait for the existing operation to finish instead.
	if activeExists {
		log.Debug("The virtual machine has an unfinished migration operation. Skipping...")
		return false, nil
	}

	log.Info("Create VMOP")
	vmop := newVMOP(s.prefix, vm.GetNamespace(), vm.GetName(), annotationKey, annotationExpectedValue)
	if err = s.client.Create(ctx, vmop); err != nil {
		// The cached VMOP list can lag behind the webhook's live view, so a
		// concurrent operation may still be detected only at admission time.
		// That is a wait-and-retry situation, not a reconcile failure.
		if isDeniedByActiveVMOP(err) {
			log.Debug("VMOP creation denied because another operation is active. Skipping...", logger.SlogErr(err))
			return false, nil
		}
		return false, err
	}

	return true, nil
}

func (s *OneShotMigrationService) listVMOPMigrate(ctx context.Context, vmName, vmNamespace string) ([]v1alpha2.VirtualMachineOperation, error) {
	vmopList := &v1alpha2.VirtualMachineOperationList{}
	if err := s.client.List(ctx, vmopList, client.InNamespace(vmNamespace)); err != nil {
		return nil, fmt.Errorf("failed to list virtual machine operations: %w", err)
	}
	var vmops []v1alpha2.VirtualMachineOperation
	for _, vmop := range vmopList.Items {
		if vmop.Spec.VirtualMachine == vmName && commonvmop.IsMigration(&vmop) {
			vmops = append(vmops, vmop)
		}
	}
	return vmops, nil
}

func (s *OneShotMigrationService) setAnnoExpectedValueToKVVMI(ctx context.Context, kvvmi *virtv1.VirtualMachineInstance, annotationKey, annotationExpectedValue string) error {
	return object.EnsureAnnotation(ctx, s.client, kvvmi, annotationKey, annotationExpectedValue)
}

// isDeniedByActiveVMOP reports whether the creation was rejected by the vmop
// admission webhook because another unfinished operation exists for the same
// virtual machine.
func isDeniedByActiveVMOP(err error) bool {
	return k8serrors.IsForbidden(err) || (err != nil && strings.Contains(err.Error(), "should finish first"))
}

func newVMOP(prefix, namespace, vmName, annotationKey, annotationExpectedValue string) *v1alpha2.VirtualMachineOperation {
	return vmopbuilder.New(
		vmopbuilder.WithGenerateName(prefix),
		vmopbuilder.WithNamespace(namespace),
		vmopbuilder.WithAnnotation(annotations.AnnVMOPWorkloadUpdate, "true"),
		vmopbuilder.WithAnnotation(annotationKey, annotationExpectedValue),
		vmopbuilder.WithType(v1alpha2.VMOPTypeEvict),
		vmopbuilder.WithVirtualMachine(vmName),
	)
}
