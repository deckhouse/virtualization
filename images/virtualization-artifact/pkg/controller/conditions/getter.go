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

package conditions

import (
	corev1 "k8s.io/api/core/v1"
	virtv1 "kubevirt.io/api/core/v1"
	cdiv1 "kubevirt.io/containerized-data-importer-api/pkg/apis/core/v1beta1"
)

func GetPodCondition(condType corev1.PodConditionType, conds []corev1.PodCondition) (corev1.PodCondition, bool) {
	for _, cond := range conds {
		if cond.Type == condType {
			return cond, true
		}
	}

	return corev1.PodCondition{}, false
}

const (
	DVRunningConditionType          cdiv1.DataVolumeConditionType = "Running"
	DVRunningConditionPendingReason string                        = "Pending"
	DVQoutaNotExceededConditionType cdiv1.DataVolumeConditionType = "QuotaNotExceeded"
	DVImagePullFailedReason         string                        = "ImagePullFailed"
)

func GetDataVolumeCondition(condType cdiv1.DataVolumeConditionType, conds []cdiv1.DataVolumeCondition) (cdiv1.DataVolumeCondition, bool) {
	for _, cond := range conds {
		if cond.Type == condType {
			return cond, true
		}
	}

	return cdiv1.DataVolumeCondition{}, false
}

func GetKVVMCondition(condType virtv1.VirtualMachineConditionType, conds []virtv1.VirtualMachineCondition) (virtv1.VirtualMachineCondition, bool) {
	for _, cond := range conds {
		if cond.Type == condType {
			return cond, true
		}
	}
	return virtv1.VirtualMachineCondition{}, false
}

func GetKVVMICondition(condType virtv1.VirtualMachineInstanceConditionType, conds []virtv1.VirtualMachineInstanceCondition) (virtv1.VirtualMachineInstanceCondition, bool) {
	for _, cond := range conds {
		if cond.Type == condType {
			return cond, true
		}
	}
	return virtv1.VirtualMachineInstanceCondition{}, false
}

func GetKVVMIMCondition(condType virtv1.VirtualMachineInstanceMigrationConditionType, conditions []virtv1.VirtualMachineInstanceMigrationCondition) (virtv1.VirtualMachineInstanceMigrationCondition, bool) {
	for _, condition := range conditions {
		if condition.Type == condType {
			return condition, true
		}
	}

	return virtv1.VirtualMachineInstanceMigrationCondition{}, false
}

const (
	VirtualMachineInstanceNodePlacementNotMatched virtv1.VirtualMachineInstanceConditionType          = "NodePlacementNotMatched"
	KubevirtMigrationRejectedByResourceQuotaType  virtv1.VirtualMachineInstanceMigrationConditionType = "migrationRejectedByResourceQuota"
	VirtualMachineSynchronized                    virtv1.VirtualMachineConditionType                  = "Synchronized"
)
