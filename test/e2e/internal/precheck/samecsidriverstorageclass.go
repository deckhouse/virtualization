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

	. "github.com/onsi/ginkgo/v2"
	storagev1 "k8s.io/api/storage/v1"

	"github.com/deckhouse/virtualization/test/e2e/internal/config"
	"github.com/deckhouse/virtualization/test/e2e/internal/framework"
)

const (
	sameCSIDriverStorageClassPrecheckEnvName = "SAME_CSI_DRIVER_STORAGE_CLASS_PRECHECK"
)

// sameCSIDriverStorageClassPrecheck implements the Precheck interface for tests that clone
// objects between the WFFC and the immediate StorageClasses (e.g. a source on the immediate
// StorageClass and the produced object on the WFFC one). It verifies that both StorageClasses
// are backed by the same CSI driver (provisioner); their presence and volume binding modes are
// enforced by the dedicated WFFC and immediate StorageClass prechecks.
type sameCSIDriverStorageClassPrecheck struct{}

func (c *sameCSIDriverStorageClassPrecheck) Label() string {
	return PrecheckSameCSIDriverStorageClass
}

func (c *sameCSIDriverStorageClassPrecheck) Run(ctx context.Context, f *framework.Framework) error {
	if !isCheckEnabled(sameCSIDriverStorageClassPrecheckEnvName) {
		_, _ = GinkgoWriter.Write([]byte("same CSI driver StorageClass precheck is disabled.\n"))
		return nil
	}

	k8sClient := f.GenericClient()
	var scList storagev1.StorageClassList
	if err := k8sClient.List(ctx, &scList); err != nil {
		return fmt.Errorf("%s=no to disable this precheck: list StorageClasses: %w", sameCSIDriverStorageClassPrecheckEnvName, err)
	}

	wffcSC, err := config.ResolveWFFCStorageClass(&scList)
	if err != nil {
		return fmt.Errorf("%s=no to disable this precheck: resolve WFFC StorageClass: %w", sameCSIDriverStorageClassPrecheckEnvName, err)
	}
	immediateSC, err := config.ResolveImmediateStorageClass(&scList)
	if err != nil {
		return fmt.Errorf("%s=no to disable this precheck: resolve immediate StorageClass: %w", sameCSIDriverStorageClassPrecheckEnvName, err)
	}
	if wffcSC == nil || immediateSC == nil {
		return fmt.Errorf(
			"%s=no to disable this precheck: both the WFFC and the immediate StorageClasses must be resolvable "+
				"from the main StorageClass (%s or the cluster default); the immediate one can be overridden with %s",
			sameCSIDriverStorageClassPrecheckEnvName, config.StorageClassNameEnv, config.ImmediateStorageClassEnv,
		)
	}

	if wffcSC.Provisioner != immediateSC.Provisioner {
		return fmt.Errorf(
			"%s=no to disable this precheck: WFFC StorageClass %q (CSI driver %q) and immediate StorageClass %q (CSI driver %q) "+
				"must be backed by the same CSI driver",
			sameCSIDriverStorageClassPrecheckEnvName,
			wffcSC.Name, wffcSC.Provisioner, immediateSC.Name, immediateSC.Provisioner,
		)
	}

	_, _ = fmt.Fprintf(GinkgoWriter,
		"same CSI driver StorageClass precheck passed: WFFC StorageClass %q and immediate StorageClass %q share CSI driver %q.\n",
		wffcSC.Name, immediateSC.Name, wffcSC.Provisioner,
	)

	return nil
}

// Register SameCSIDriverStorageClass precheck (not common - requires explicit label).
func init() {
	RegisterPrecheck(&sameCSIDriverStorageClassPrecheck{}, false)
}
