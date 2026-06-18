/*
Copyright 2026 Flant JSC

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

package precheck

import (
	"context"
	"fmt"

	storagev1 "k8s.io/api/storage/v1"

	"github.com/deckhouse/virtualization/test/e2e/internal/config"
	"github.com/deckhouse/virtualization/test/e2e/internal/framework"
)

const (
	immediateStorageClassPrecheckEnvName = "IMMEDIATE_STORAGE_CLASS_PRECHECK"
)

// immediateStorageClassPrecheck implements Precheck interface for the immediate StorageClass.
// This precheck verifies that a StorageClass annotated with ImmediateStorageClassAnnotation=true
// exists and uses the Immediate volume binding mode. This is required for tests that work with
// snapshots, as PVs need to be immediately bound.
type immediateStorageClassPrecheck struct{}

func (c *immediateStorageClassPrecheck) Label() string {
	return PrecheckImmediateStorageClass
}

func (c *immediateStorageClassPrecheck) Run(ctx context.Context, f *framework.Framework) error {
	if !isCheckEnabled(immediateStorageClassPrecheckEnvName) {
		return nil
	}

	k8sClient := f.GenericClient()
	var scList storagev1.StorageClassList
	if err := k8sClient.List(ctx, &scList); err != nil {
		return fmt.Errorf("%s=no to disable this precheck: list StorageClasses: %w", immediateStorageClassPrecheckEnvName, err)
	}

	immediateSC := config.FindStorageClassByAnnotation(&scList, config.ImmediateStorageClassAnnotation)
	if immediateSC == nil {
		return fmt.Errorf(
			"%s=no to disable this precheck: immediate StorageClass not found. Annotate an Immediate-binding StorageClass for the e2e tests:\n"+
				"  kubectl annotate storageclass/<immediate-sc-name> %s=true --overwrite",
			immediateStorageClassPrecheckEnvName, config.ImmediateStorageClassAnnotation)
	}

	if !isImmediateBinding(immediateSC) {
		return fmt.Errorf(
			"%s=no to disable this precheck: StorageClass %q annotated with %s=true must use the Immediate volume binding mode, but it is %q",
			immediateStorageClassPrecheckEnvName, immediateSC.Name, config.ImmediateStorageClassAnnotation,
			volumeBindingMode(immediateSC))
	}

	return nil
}

// isImmediateBinding reports whether sc uses the Immediate volume binding mode. A nil
// VolumeBindingMode defaults to Immediate per the Kubernetes API.
func isImmediateBinding(sc *storagev1.StorageClass) bool {
	return sc.VolumeBindingMode == nil || *sc.VolumeBindingMode == storagev1.VolumeBindingImmediate
}

// volumeBindingMode returns sc's volume binding mode for diagnostics, rendering a nil
// mode as its Immediate default.
func volumeBindingMode(sc *storagev1.StorageClass) storagev1.VolumeBindingMode {
	if sc.VolumeBindingMode == nil {
		return storagev1.VolumeBindingImmediate
	}
	return *sc.VolumeBindingMode
}

// Register ImmediateStorageClass precheck (not common - requires explicit label).
func init() {
	RegisterPrecheck(&immediateStorageClassPrecheck{}, false)
}
