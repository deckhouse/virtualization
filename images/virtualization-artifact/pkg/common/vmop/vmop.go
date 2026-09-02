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

package vmop

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/deckhouse/virtualization-controller/pkg/controller/conditions"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/vmopcondition"
)

// Prefixes of the generated names of the operations that the controllers of the module create on
// their own. Every such operation is an eviction, and the annotations that describe a workload
// update are put on the internal virtual machine instance rather than on the operation, so the
// name is the only thing that tells a firmware update apart from a node placement update.
const (
	FirmwareUpdatePrefix      = "firmware-update-"
	NodePlacementUpdatePrefix = "nodeplacement-update-"
	HotplugResourcesPrefix    = "hotplug-resources-"
	VolumeMigrationPrefix     = "volume-migration-"
)

func IsInProgressOrPending(vmop *v1alpha2.VirtualMachineOperation) bool {
	return vmop != nil && (vmop.Status.Phase == "" || vmop.Status.Phase == v1alpha2.VMOPPhasePending || vmop.Status.Phase == v1alpha2.VMOPPhaseInProgress)
}

func IsFinished(vmop *v1alpha2.VirtualMachineOperation) bool {
	return vmop != nil && (vmop.Status.Phase == v1alpha2.VMOPPhaseFailed || vmop.Status.Phase == v1alpha2.VMOPPhaseCompleted || vmop.Status.Phase == v1alpha2.VMOPPhaseSuperseded)
}

func IsTerminating(vmop *v1alpha2.VirtualMachineOperation) bool {
	return vmop != nil && (vmop.Status.Phase == v1alpha2.VMOPPhaseTerminating || !vmop.GetDeletionTimestamp().IsZero())
}

func IsMigration(vmop *v1alpha2.VirtualMachineOperation) bool {
	return vmop != nil && (vmop.Spec.Type == v1alpha2.VMOPTypeMigrate || vmop.Spec.Type == v1alpha2.VMOPTypeEvict)
}

func IsOperationInProgress(vmop *v1alpha2.VirtualMachineOperation) bool {
	sent, _ := conditions.GetCondition(vmopcondition.TypeSignalSent, vmop.Status.Conditions)
	return sent.Status == metav1.ConditionTrue && !IsFinished(vmop)
}
