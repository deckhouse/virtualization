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
	defaultStorageClassPrecheckEnvName = "DEFAULT_STORAGE_CLASS_PRECHECK"
)

// defaultStorageClassPrecheck implements Precheck interface for default StorageClass.
// This is a common precheck that runs for all tests.
type defaultStorageClassPrecheck struct{}

func (c *defaultStorageClassPrecheck) Label() string {
	return PrecheckDefaultStorageClass
}

func (c *defaultStorageClassPrecheck) Run(ctx context.Context, f *framework.Framework) error {
	if !isCheckEnabled(defaultStorageClassPrecheckEnvName) {
		return nil
	}

	k8sClient := f.GenericClient()
	var scList storagev1.StorageClassList
	err := k8sClient.List(ctx, &scList)
	if err != nil {
		return fmt.Errorf("%s=no to disable this precheck: list StorageClasses: %w", defaultStorageClassPrecheckEnvName, err)
	}

	if config.FindDefaultStorageClass(&scList) == nil {
		return fmt.Errorf("%s=no to disable this precheck: cluster has no default StorageClass. "+
			"Please set a default StorageClass with: kubectl annotate storageclass/<name> storageclass.kubernetes.io/is-default-class=true",
			defaultStorageClassPrecheckEnvName)
	}

	return nil
}

// Register defaultStorageClassPrecheck as common (runs for all tests).
func init() {
	RegisterPrecheck(&defaultStorageClassPrecheck{}, true)
}
