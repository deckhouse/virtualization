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

package vd

import (
	"log/slog"

	"k8s.io/component-base/featuregate"

	"github.com/deckhouse/virtualization-controller/pkg/featuregates"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
)

func GetCurrentlyMountedVMName(vd *v1alpha2.VirtualDisk) string {
	for _, attachedVM := range vd.Status.AttachedToVirtualMachines {
		if attachedVM.Mounted {
			return attachedVM.Name
		}
	}
	return ""
}

func IsMigrating(vd *v1alpha2.VirtualDisk) bool {
	return vd != nil && !vd.Status.MigrationState.StartTimestamp.IsZero() && vd.Status.MigrationState.EndTimestamp.IsZero()
}

// VolumeMigrationEnabled returns true if volume migration is enabled or if the volume is currently migrating
// If the volume migrating but the feature gate was turned off, we should complete the migration
func VolumeMigrationEnabled(gate featuregate.FeatureGate, vd *v1alpha2.VirtualDisk) bool {
	if gate.Enabled(featuregates.VolumeMigration) {
		return true
	}
	if IsMigrating(vd) {
		slog.Info("VolumeMigration is disabled, but the volume is already migrating. Complete the migration.", slog.String("vd.name", vd.Name), slog.String("vd.namespace", vd.Namespace))
		return true
	}
	return false
}

func StorageClassChanged(vd *v1alpha2.VirtualDisk) bool {
	if vd == nil {
		return false
	}

	specSc := vd.Spec.PersistentVolumeClaim.StorageClass
	if specSc == nil {
		return false
	}

	statusSc := vd.Status.StorageClassName
	if *specSc == statusSc {
		return false
	}

	return *specSc != "" && statusSc != ""
}
