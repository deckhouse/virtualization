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

// immediateStorageClassPrecheck implements Precheck interface for immediate StorageClass.
// This precheck verifies that:
// 1. Default StorageClass has VolumeBindingMode=Immediate, OR
// 2. There is an immediate StorageClass with the same provisioner as default StorageClass.
// This is required for tests that work with snapshots, as PVs need to be immediately bound.
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

	// Find default StorageClass
	defaultSC := config.FindDefaultStorageClass(&scList)
	if defaultSC == nil {
		return fmt.Errorf("%s=no to disable this precheck: cluster has no default StorageClass",
			immediateStorageClassPrecheckEnvName)
	}

	// Check if immediate StorageClass exists with same provisioner
	immediateSC := config.FindImmediateStorageClass(defaultSC, &scList)
	if immediateSC == nil {
		return fmt.Errorf("%s=no to disable this precheck: default StorageClass %q has WaitForFirstConsumer binding mode, "+
			"and no immediate StorageClass found with the same provisioner %q. "+
			"Create an immediate StorageClass or set an immediate StorageClass as default",
			immediateStorageClassPrecheckEnvName, defaultSC.Name, defaultSC.Provisioner)
	}

	return nil
}

// Register ImmediateStorageClass precheck (not common - requires explicit label).
func init() {
	RegisterPrecheck(&immediateStorageClassPrecheck{}, false)
}
