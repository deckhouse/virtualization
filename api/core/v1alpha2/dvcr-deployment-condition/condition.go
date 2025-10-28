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

package dvcr_deployment_condition

// Type represents the various condition types for the `ClusterVirtualImage`.
type Type string

func (s Type) String() string {
	return string(s)
}

const (
	// MaintenanceType indicates whether the deployment/dvcr is in maintenance mode.
	MaintenanceType Type = "Maintenance"
)

type (
	// MaintenanceReason represents the various reasons for the DVCRMaintenance condition type.
	MaintenanceReason string
)

func (s MaintenanceReason) String() string {
	return string(s)
}

const (
	// PrepareAutoCleanup indicates that the maintenance is prepared: create secret, wait for vi/cvi/vd to stop uploading.
	PrepareAutoCleanup MaintenanceReason = "PrepareAutoCleanup"
	// MaintenanceAutoCleanupInProgress indicates that deployment is in the maintenance mode.
	MaintenanceAutoCleanupInProgress MaintenanceReason = "AutoCleanupInProgress"
	// MaintenanceAutoCleanupScheduled indicates that the deployment is in the normal mode, and the maintenance is scheduled for some time in the future
	MaintenanceAutoCleanupScheduled MaintenanceReason = "AutoCleanupScheduled"
)
