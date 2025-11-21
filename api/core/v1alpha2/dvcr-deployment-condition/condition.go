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

import appsv1 "k8s.io/api/apps/v1"

const (
	// GarbageCollectionType indicates whether the deployment/dvcr is in garbage collection mode.
	GarbageCollectionType appsv1.DeploymentConditionType = "GarbageCollection"
)

type (
	// GarbageCollectionReason represents the various reasons for the GarbageCollection condition type.
	GarbageCollectionReason string
)

func (s GarbageCollectionReason) String() string {
	return string(s)
}

const (
	// InProgress indicates that the garbage collection is in progress. (status "True")
	InProgress GarbageCollectionReason = "InProgress"

	// Completed indicates that the garbage collection is done and result is in the message. (status "False")
	Completed GarbageCollectionReason = "Completed"

	// Error indicates that the garbage collection was unsuccessful and error is in the message. (status "False")
	Error GarbageCollectionReason = "Error"
)
