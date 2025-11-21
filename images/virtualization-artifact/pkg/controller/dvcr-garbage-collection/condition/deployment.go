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

package condition

import (
	"fmt"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	dvcrdeploymentcondition "github.com/deckhouse/virtualization/api/core/v1alpha2/dvcr-deployment-condition"
)

func NewGarbageCollectionCondition(reason dvcrdeploymentcondition.GarbageCollectionReason, msgf string, args ...any) appsv1.DeploymentCondition {
	status := "Unknown"
	switch reason {
	case dvcrdeploymentcondition.Completed,
		dvcrdeploymentcondition.Error:
		status = "False"
	case dvcrdeploymentcondition.InProgress:
		status = "True"
	}

	return appsv1.DeploymentCondition{
		Type:           dvcrdeploymentcondition.GarbageCollectionType,
		Status:         corev1.ConditionStatus(status),
		LastUpdateTime: metav1.Now(),
		Reason:         string(reason),
		Message:        fmt.Sprintf(msgf, args...),
	}
}

// UpdateGarbageCollectionCondition replaces or removes GarbageCollection condition from deployment status.
func UpdateGarbageCollectionCondition(deploy *appsv1.Deployment, reason dvcrdeploymentcondition.GarbageCollectionReason, fmtStr string, args ...any) {
	if deploy == nil {
		return
	}

	condition := NewGarbageCollectionCondition(reason, fmtStr, args...)

	// Add or update existing condition.
	filteredConditions := make([]appsv1.DeploymentCondition, 0, len(deploy.Status.Conditions))
	existing := false
	for _, cond := range deploy.Status.Conditions {
		if cond.Type == dvcrdeploymentcondition.GarbageCollectionType {
			if cond.Reason != condition.Reason || cond.Message != condition.Message {
				condition.LastTransitionTime = metav1.Now()
			}
			cond = condition
			existing = true
		}
		filteredConditions = append(filteredConditions, cond)
	}
	if !existing {
		filteredConditions = append(filteredConditions, condition)
	}
	deploy.Status.Conditions = filteredConditions
}
