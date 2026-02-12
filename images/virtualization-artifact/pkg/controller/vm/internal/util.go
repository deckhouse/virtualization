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
	"slices"
	"strings"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	virtv1 "kubevirt.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/deckhouse/virtualization-controller/pkg/common/annotations"
	"github.com/deckhouse/virtualization-controller/pkg/controller/conditions"
	"github.com/deckhouse/virtualization-controller/pkg/controller/service"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/vmcondition"
)

func isDeletion(vm *v1alpha2.VirtualMachine) bool {
	return vm == nil || !vm.GetDeletionTimestamp().IsZero()
}

type updaterProtection func(p *service.ProtectionService) func(ctx context.Context, objs ...client.Object) error

func addAllUnknown(vm *v1alpha2.VirtualMachine, conds ...vmcondition.Type) (update bool) {
	for _, cond := range conds {
		if conditions.HasCondition(cond, vm.Status.Conditions) {
			continue
		}
		cb := conditions.NewConditionBuilder(cond).
			Generation(vm.GetGeneration()).
			Reason(conditions.ReasonUnknown).
			Status(metav1.ConditionUnknown)
		conditions.SetCondition(cb, &vm.Status.Conditions)
		update = true
	}
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

func isVMPending(kvvm *virtv1.VirtualMachine) bool {
	return getPhase(nil, kvvm) == v1alpha2.MachinePending
}

func isVMStopped(kvvm *virtv1.VirtualMachine) bool {
	return getPhase(nil, kvvm) == v1alpha2.MachineStopped
}

func isKVVMICreated(kvvm *virtv1.VirtualMachine) bool {
	return kvvm != nil && kvvm.Status.Created
}

func getPhase(vm *v1alpha2.VirtualMachine, kvvm *virtv1.VirtualMachine) v1alpha2.MachinePhase {
	if kvvm == nil {
		return v1alpha2.MachinePending
	}

	if handler, exists := mapPhases[kvvm.Status.PrintableStatus]; exists {
		return handler(vm, kvvm)
	}

	return v1alpha2.MachinePending
}

type PhaseGetter func(vm *v1alpha2.VirtualMachine, kvvm *virtv1.VirtualMachine) v1alpha2.MachinePhase

var mapPhases = map[virtv1.VirtualMachinePrintableStatus]PhaseGetter{
	// VirtualMachineStatusStopped indicates that the virtual machine is currently stopped and isn't expected to start.
	virtv1.VirtualMachineStatusStopped: func(vm *v1alpha2.VirtualMachine, kvvm *virtv1.VirtualMachine) v1alpha2.MachinePhase {
		if vm != nil && kvvm != nil {
			if !checkVirtualMachineConfiguration(vm) &&
				kvvm != nil && kvvm.Annotations[annotations.AnnVMStartRequested] == "true" {
				return v1alpha2.MachinePending
			}
		}

		if vm != nil && vm.Status.Phase == v1alpha2.MachinePending &&
			(vm.Spec.RunPolicy == v1alpha2.AlwaysOnPolicy || vm.Spec.RunPolicy == v1alpha2.AlwaysOnUnlessStoppedManually) {
			return v1alpha2.MachinePending
		}

		return v1alpha2.MachineStopped
	},
	// VirtualMachineStatusProvisioning indicates that cluster resources associated with the virtual machine
	// (e.g., DataVolumes) are being provisioned and prepared.
	virtv1.VirtualMachineStatusProvisioning: func(_ *v1alpha2.VirtualMachine, _ *virtv1.VirtualMachine) v1alpha2.MachinePhase {
		return v1alpha2.MachineStarting
	},
	// VirtualMachineStatusStarting indicates that the virtual machine is being prepared for running.
	virtv1.VirtualMachineStatusStarting: func(_ *v1alpha2.VirtualMachine, kvvm *virtv1.VirtualMachine) v1alpha2.MachinePhase {
		synchronizedCondition, _ := conditions.GetKVVMCondition(conditions.VirtualMachineSynchronized, kvvm.Status.Conditions)

		if synchronizedCondition.Reason == failedCreatePodReason {
			return v1alpha2.MachinePending
		}

		return v1alpha2.MachineStarting
	},
	// VirtualMachineStatusRunning indicates that the virtual machine is running.
	virtv1.VirtualMachineStatusRunning: func(_ *v1alpha2.VirtualMachine, _ *virtv1.VirtualMachine) v1alpha2.MachinePhase {
		return v1alpha2.MachineRunning
	},
	// VirtualMachineStatusPaused indicates that the virtual machine is paused.
	virtv1.VirtualMachineStatusPaused: func(_ *v1alpha2.VirtualMachine, _ *virtv1.VirtualMachine) v1alpha2.MachinePhase {
		return v1alpha2.MachinePause
	},
	// VirtualMachineStatusStopping indicates that the virtual machine is in the process of being stopped.
	virtv1.VirtualMachineStatusStopping: func(_ *v1alpha2.VirtualMachine, _ *virtv1.VirtualMachine) v1alpha2.MachinePhase {
		return v1alpha2.MachineStopping
	},
	// VirtualMachineStatusTerminating indicates that the virtual machine is in the process of deletion,
	// as well as its associated resources (VirtualMachineInstance, DataVolumes, …).
	virtv1.VirtualMachineStatusTerminating: func(_ *v1alpha2.VirtualMachine, _ *virtv1.VirtualMachine) v1alpha2.MachinePhase {
		return v1alpha2.MachineTerminating
	},
	// VirtualMachineStatusCrashLoopBackOff indicates that the virtual machine is currently in a crash loop waiting to be retried.
	virtv1.VirtualMachineStatusCrashLoopBackOff: func(_ *v1alpha2.VirtualMachine, _ *virtv1.VirtualMachine) v1alpha2.MachinePhase {
		return v1alpha2.MachinePending
	},
	// VirtualMachineStatusMigrating indicates that the virtual machine is in the process of being migrated
	// to another host.
	virtv1.VirtualMachineStatusMigrating: func(_ *v1alpha2.VirtualMachine, _ *virtv1.VirtualMachine) v1alpha2.MachinePhase {
		return v1alpha2.MachineMigrating
	},
	// VirtualMachineStatusUnknown indicates that the state of the virtual machine could not be obtained,
	// typically due to an error in communicating with the host on which it's running.
	virtv1.VirtualMachineStatusUnknown: func(_ *v1alpha2.VirtualMachine, _ *virtv1.VirtualMachine) v1alpha2.MachinePhase {
		return v1alpha2.MachinePending
	},
	// VirtualMachineStatusUnschedulable indicates that an error has occurred while scheduling the virtual machine,
	// e.g. due to unsatisfiable resource requests or unsatisfiable scheduling constraints.
	virtv1.VirtualMachineStatusUnschedulable: func(_ *v1alpha2.VirtualMachine, _ *virtv1.VirtualMachine) v1alpha2.MachinePhase {
		return v1alpha2.MachinePending
	},
	// VirtualMachineStatusErrImagePull indicates that an error has occurred while pulling an image for
	// a containerDisk VM volume.
	virtv1.VirtualMachineStatusErrImagePull: func(_ *v1alpha2.VirtualMachine, _ *virtv1.VirtualMachine) v1alpha2.MachinePhase {
		return v1alpha2.MachinePending
	},
	// VirtualMachineStatusImagePullBackOff indicates that an error has occurred while pulling an image for
	// a containerDisk VM volume, and that kubelet is backing off before retrying.
	virtv1.VirtualMachineStatusImagePullBackOff: func(_ *v1alpha2.VirtualMachine, _ *virtv1.VirtualMachine) v1alpha2.MachinePhase {
		return v1alpha2.MachinePending
	},
	// VirtualMachineStatusPvcNotFound indicates that the virtual machine references a PVC volume which doesn't exist.
	virtv1.VirtualMachineStatusPvcNotFound: func(_ *v1alpha2.VirtualMachine, _ *virtv1.VirtualMachine) v1alpha2.MachinePhase {
		return v1alpha2.MachinePending
	},
	// VirtualMachineStatusDataVolumeError indicates that an error has been reported by one of the DataVolumes
	// referenced by the virtual machines.
	virtv1.VirtualMachineStatusDataVolumeError: func(_ *v1alpha2.VirtualMachine, _ *virtv1.VirtualMachine) v1alpha2.MachinePhase {
		return v1alpha2.MachinePending
	},
	// VirtualMachineStatusWaitingForVolumeBinding indicates that some PersistentVolumeClaims backing
	// the virtual machine volume are still not bound.
	virtv1.VirtualMachineStatusWaitingForVolumeBinding: func(_ *v1alpha2.VirtualMachine, _ *virtv1.VirtualMachine) v1alpha2.MachinePhase {
		return v1alpha2.MachinePending
	},

	kvvmEmptyPhase: func(_ *v1alpha2.VirtualMachine, _ *virtv1.VirtualMachine) v1alpha2.MachinePhase {
		return v1alpha2.MachinePending
	},
}

const (
	kvvmEmptyPhase        virtv1.VirtualMachinePrintableStatus = ""
	failedCreatePodReason string                               = "FailedCreate"
)

func getKVMIReadyReason(kvmiReason string) conditions.Stringer {
	if r, ok := mapReasons[kvmiReason]; ok {
		return r
	}

	if kvmiReason == "" {
		return conditions.ReasonUnknown
	}

	return conditions.CommonReason(kvmiReason)
}

var mapReasons = map[string]vmcondition.RunningReason{
	// PodTerminatingReason indicates on the Ready condition on the VMI if the underlying pod is terminating
	virtv1.PodTerminatingReason: vmcondition.ReasonPodTerminating,
	// PodNotExistsReason indicates on the Ready condition on the VMI if the underlying pod does not exist
	virtv1.PodNotExistsReason: vmcondition.ReasonPodNotFound,
	// PodConditionMissingReason indicates on the Ready condition on the VMI if the underlying pod does not report a Ready condition
	virtv1.PodConditionMissingReason: vmcondition.ReasonPodConditionMissing,
	// GuestNotRunningReason indicates on the Ready condition on the VMI if the underlying guest VM is not running
	virtv1.GuestNotRunningReason: vmcondition.ReasonGuestNotRunning,
}

func isPodStartedError(vm *virtv1.VirtualMachine) bool {
	synchronized := service.GetKVVMCondition(string(virtv1.VirtualMachineInstanceSynchronized), vm.Status.Conditions)
	if synchronized != nil &&
		synchronized.Status == corev1.ConditionFalse &&
		synchronized.Reason == failedCreatePodReason {
		return true
	}

	return slices.Contains([]virtv1.VirtualMachinePrintableStatus{
		virtv1.VirtualMachineStatusErrImagePull,
		virtv1.VirtualMachineStatusImagePullBackOff,
		virtv1.VirtualMachineStatusCrashLoopBackOff,
		virtv1.VirtualMachineStatusUnschedulable,
		virtv1.VirtualMachineStatusDataVolumeError,
		virtv1.VirtualMachineStatusPvcNotFound,
	}, vm.Status.PrintableStatus)
}

func isInternalVirtualMachineError(phase virtv1.VirtualMachinePrintableStatus) bool {
	return slices.Contains([]virtv1.VirtualMachinePrintableStatus{
		virtv1.VirtualMachineStatusErrImagePull,
		virtv1.VirtualMachineStatusImagePullBackOff,
		virtv1.VirtualMachineStatusDataVolumeError,
		virtv1.VirtualMachineStatusPvcNotFound,
		virtv1.VirtualMachineStatusCrashLoopBackOff,
		virtv1.VirtualMachineStatusUnknown,
	}, phase)
}

func podFinal(pod corev1.Pod) bool {
	return pod.Status.Phase == corev1.PodSucceeded || pod.Status.Phase == corev1.PodFailed
}

func checkVirtualMachineConfiguration(vm *v1alpha2.VirtualMachine) bool {
	for _, c := range vm.Status.Conditions {
		switch vmcondition.Type(c.Type) {
		case vmcondition.TypeBlockDevicesReady:
			if c.Status != metav1.ConditionTrue && c.Reason != vmcondition.ReasonWaitingForWaitForFirstConsumerBlockDevicesToBeReady.String() {
				return false
			}

		case vmcondition.TypeSnapshotting:
			if c.Status == metav1.ConditionTrue && c.Reason == vmcondition.ReasonSnapshottingInProgress.String() {
				return false
			}

		case vmcondition.TypeIPAddressReady:
			if c.Status != metav1.ConditionTrue && c.Reason != vmcondition.ReasonIPAddressNotAssigned.String() {
				return false
			}

		case vmcondition.TypeProvisioningReady,
			vmcondition.TypeClassReady:
			if c.Status != metav1.ConditionTrue {
				return false
			}

		case vmcondition.TypeSizingPolicyMatched:
			if c.Status != metav1.ConditionTrue {
				return false
			}

		case vmcondition.TypeMACAddressReady:
			if c.Status != metav1.ConditionTrue {
				return false
			}

		case vmcondition.TypeNetworkReady:
			if c.Status == metav1.ConditionFalse && c.Reason == vmcondition.ReasonSDNModuleDisabled.String() {
				return false
			}
		}
	}
	return true
}
