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

// upgradeStorageClassesPrecheck requires both StorageClasses of the module
// upgrade suite to be named explicitly and to exist: STORAGE_CLASS_NAME for the
// root and most data disks (a migratable, replicated class is expected) and
// LOCAL_STORAGE_CLASS_NAME for the node-local data disks. The cluster default
// StorageClass is deliberately not used as a fallback: the suite must know
// which class plays which role.
type upgradeStorageClassesPrecheck struct{}

func (p *upgradeStorageClassesPrecheck) Label() string {
	return PrecheckUpgradeStorageClasses
}

func (p *upgradeStorageClassesPrecheck) Run(ctx context.Context, f *framework.Framework) error {
	replicated := config.StorageClassNameOverride()
	local := config.LocalStorageClassNameOverride()
	if replicated == "" || local == "" {
		return fmt.Errorf("the module upgrade suite needs both %s and %s to be set: "+
			"%s=<replicated StorageClass> %s=<node-local StorageClass> task run:upgrade",
			config.StorageClassNameEnv, config.LocalStorageClassNameEnv,
			config.StorageClassNameEnv, config.LocalStorageClassNameEnv)
	}
	if replicated == local {
		return fmt.Errorf("%s and %s must name different StorageClasses, both are %q",
			config.StorageClassNameEnv, config.LocalStorageClassNameEnv, replicated)
	}

	var scList storagev1.StorageClassList
	if err := f.GenericClient().List(ctx, &scList); err != nil {
		return fmt.Errorf("list StorageClasses: %w", err)
	}
	if _, err := config.ResolveDefaultStorageClass(&scList); err != nil {
		return err
	}
	if _, err := config.ResolveLocalStorageClass(&scList); err != nil {
		return err
	}

	return nil
}

func init() {
	RegisterPrecheck(&upgradeStorageClassesPrecheck{}, false)
}
