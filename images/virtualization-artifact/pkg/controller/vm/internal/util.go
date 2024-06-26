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

package internal

import (
	"context"
	"strings"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	virtv1 "kubevirt.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/deckhouse/virtualization-controller/pkg/controller/conditions"
	"github.com/deckhouse/virtualization-controller/pkg/controller/service"
	virtv2 "github.com/deckhouse/virtualization/api/core/v1alpha2"
)

func isDeletion(vm *virtv2.VirtualMachine) bool {
	return vm == nil || !vm.GetDeletionTimestamp().IsZero()
}

type updaterProtection func(p *service.ProtectionService) func(ctx context.Context, objs ...client.Object) error

func addAllUnknown(vm *virtv2.VirtualMachine, conds ...string) (update bool) {
	mgr := conditions.NewManager(vm.Status.Conditions)
	for _, c := range conds {
		if add := mgr.Add(conditions.NewConditionBuilder(c).
			Generation(vm.GetGeneration()).
			Status(metav1.ConditionUnknown).
			Condition()); add {
			update = true
		}
	}
	vm.Status.Conditions = mgr.Generate()
	return
}

func conditionStatus(status string) metav1.ConditionStatus {
	status = strings.ToLower(status)
	switch {
	case strings.Contains(status, "true"):
		return metav1.ConditionTrue
	case strings.Contains(status, "false"):
		return metav1.ConditionFalse
	default:
		return metav1.ConditionUnknown
	}
}

func namespacedName(obj client.Object) types.NamespacedName {
	return types.NamespacedName{Name: obj.GetName(), Namespace: obj.GetNamespace()}
}

func vmIsDegraded(kvvm *virtv1.VirtualMachine) bool {
	return getPhase(kvvm) == virtv2.MachineDegraded
}

func vmIsPending(kvvm *virtv1.VirtualMachine) bool {
	return getPhase(kvvm) == virtv2.MachinePending
}

func vmIsStopped(kvvm *virtv1.VirtualMachine) bool {
	return getPhase(kvvm) == virtv2.MachineStopped
}

func vmIsCreated(kvvm *virtv1.VirtualMachine) bool {
	return kvvm != nil && kvvm.Status.Created
}

func getPhase(kvvm *virtv1.VirtualMachine) virtv2.MachinePhase {
	phase := kvvmEmptyPhase
	if kvvm != nil {
		phase = kvvm.Status.PrintableStatus
	}
	return mapPhases[phase]
}

const kvvmEmptyPhase virtv1.VirtualMachinePrintableStatus = ""

var mapPhases = map[virtv1.VirtualMachinePrintableStatus]virtv2.MachinePhase{
	// VirtualMachineStatusStopped indicates that the virtual machine is currently stopped and isn't expected to start.
	virtv1.VirtualMachineStatusStopped: virtv2.MachineStopped,
	// VirtualMachineStatusProvisioning indicates that cluster resources associated with the virtual machine
	// (e.g., DataVolumes) are being provisioned and prepared.
	virtv1.VirtualMachineStatusProvisioning: virtv2.MachineStarting,
	// VirtualMachineStatusStarting indicates that the virtual machine is being prepared for running.
	virtv1.VirtualMachineStatusStarting: virtv2.MachineStarting,
	// VirtualMachineStatusRunning indicates that the virtual machine is running.
	virtv1.VirtualMachineStatusRunning: virtv2.MachineRunning,
	// VirtualMachineStatusPaused indicates that the virtual machine is paused.
	virtv1.VirtualMachineStatusPaused: virtv2.MachinePause,
	// VirtualMachineStatusStopping indicates that the virtual machine is in the process of being stopped.
	virtv1.VirtualMachineStatusStopping: virtv2.MachineStopping,
	// VirtualMachineStatusTerminating indicates that the virtual machine is in the process of deletion,
	// as well as its associated resources (VirtualMachineInstance, DataVolumes, …).
	virtv1.VirtualMachineStatusTerminating: virtv2.MachineTerminating,
	// VirtualMachineStatusCrashLoopBackOff indicates that the virtual machine is currently in a crash loop waiting to be retried.
	virtv1.VirtualMachineStatusCrashLoopBackOff: virtv2.MachineDegraded,
	// VirtualMachineStatusMigrating indicates that the virtual machine is in the process of being migrated
	// to another host.
	virtv1.VirtualMachineStatusMigrating: virtv2.MachineMigrating,
	// VirtualMachineStatusUnknown indicates that the state of the virtual machine could not be obtained,
	// typically due to an error in communicating with the host on which it's running.
	virtv1.VirtualMachineStatusUnknown: virtv2.MachineDegraded,
	// VirtualMachineStatusUnschedulable indicates that an error has occurred while scheduling the virtual machine,
	// e.g. due to unsatisfiable resource requests or unsatisfiable scheduling constraints.
	virtv1.VirtualMachineStatusUnschedulable: virtv2.MachineDegraded,
	// VirtualMachineStatusErrImagePull indicates that an error has occurred while pulling an image for
	// a containerDisk VM volume.
	virtv1.VirtualMachineStatusErrImagePull: virtv2.MachineDegraded,
	// VirtualMachineStatusImagePullBackOff indicates that an error has occurred while pulling an image for
	// a containerDisk VM volume, and that kubelet is backing off before retrying.
	virtv1.VirtualMachineStatusImagePullBackOff: virtv2.MachineDegraded,
	// VirtualMachineStatusPvcNotFound indicates that the virtual machine references a PVC volume which doesn't exist.
	virtv1.VirtualMachineStatusPvcNotFound: virtv2.MachineDegraded,
	// VirtualMachineStatusDataVolumeError indicates that an error has been reported by one of the DataVolumes
	// referenced by the virtual machines.
	virtv1.VirtualMachineStatusDataVolumeError: virtv2.MachineDegraded,
	// VirtualMachineStatusWaitingForVolumeBinding indicates that some PersistentVolumeClaims backing
	// the virtual machine volume are still not bound.
	virtv1.VirtualMachineStatusWaitingForVolumeBinding: virtv2.MachinePending,
	kvvmEmptyPhase: virtv2.MachinePending,
}
