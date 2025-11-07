package condition

import (
	"fmt"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	dvcr_deployment_condition "github.com/deckhouse/virtualization/api/core/v1alpha2/dvcr-deployment-condition"
)

func NewMaintenanceCondition(reason dvcr_deployment_condition.MaintenanceReason, msgf string, args ...any) appsv1.DeploymentCondition {
	status := "Unknown"
	switch reason {
	case dvcr_deployment_condition.LastResult:
		status = "False"
	case dvcr_deployment_condition.InProgress:
		status = "True"
	}

	return appsv1.DeploymentCondition{
		Type:           dvcr_deployment_condition.MaintenanceType,
		Status:         corev1.ConditionStatus(status),
		LastUpdateTime: metav1.Now(),
		Reason:         string(reason),
		Message:        fmt.Sprintf(msgf, args...),
	}
}

// UpdateMaintenanceCondition replaces or removes Maintenance condition from deployment status.
// Return true if status was changed.
func UpdateMaintenanceCondition(deploy *appsv1.Deployment, reason dvcr_deployment_condition.MaintenanceReason, msgf string, args ...any) {
	if deploy == nil {
		return
	}

	condition := NewMaintenanceCondition(reason, msgf, args...)

	// Condition is nil, so remove maintenance condition.
	if len(deploy.Status.Conditions) > 0 {
		filteredConditions := make([]appsv1.DeploymentCondition, 0)
		for _, cond := range deploy.Status.Conditions {
			if cond.Type == dvcr_deployment_condition.MaintenanceType {
				if cond.Reason != condition.Reason || cond.Message != condition.Message {
					condition.LastTransitionTime = metav1.Now()
				}
				cond = condition
			}
			// Copy non-maintenance conditions.
			filteredConditions = append(filteredConditions, cond)
		}
		deploy.Status.Conditions = filteredConditions
	}

	// Deploy has no conditions, create new slice.
	deploy.Status.Conditions = []appsv1.DeploymentCondition{condition}
}

// DeleteMaintenanceCondition removes Maintenance condition from deployment status.
func DeleteMaintenanceCondition(deploy *appsv1.Deployment) {
	if deploy == nil || len(deploy.Status.Conditions) == 0 {
		return
	}

	// Filter conditions to remove maintenance condition.
	filteredConditions := make([]appsv1.DeploymentCondition, 0)
	for _, cond := range deploy.Status.Conditions {
		if cond.Type != dvcr_deployment_condition.MaintenanceType {
			// Copy only non-maintenance conditions.
			filteredConditions = append(filteredConditions, cond)
		}
	}
	deploy.Status.Conditions = filteredConditions
}
