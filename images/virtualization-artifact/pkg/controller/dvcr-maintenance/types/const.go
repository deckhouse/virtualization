package types

import "k8s.io/apimachinery/pkg/types"

const (
	ModuleNamespace = "d8-virtualization"

	DVCRDeploymentName         = "dvcr"
	DVCRMaintenanceSecretName  = "dvcr-maintenance"
	CronSourceNamespace        = "__cron_source__"
	CronSourceRunGC            = "run-gc"
	CronSourceProvisioningPoll = "provisioning-poll"

	// ProvisioningPollSchedule is a cron schedule to poll provisioning status: every 20 seconds.
	ProvisioningPollSchedule = "*/20 * * * * *"
)

func DVCRDeploymentKey() types.NamespacedName {
	return types.NamespacedName{
		Namespace: ModuleNamespace,
		Name:      DVCRDeploymentName,
	}
}

func DVCRMaintenanceSecretKey() types.NamespacedName {
	return types.NamespacedName{
		Namespace: ModuleNamespace,
		Name:      DVCRMaintenanceSecretName,
	}
}
