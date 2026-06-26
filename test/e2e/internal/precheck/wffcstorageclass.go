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
	wffcStorageClassPrecheckEnvName = "WFFC_STORAGE_CLASS_PRECHECK"
)

// wffcStorageClassPrecheck implements the Precheck interface for the WaitForFirstConsumer
// StorageClass used by block-device tests to provision the scenario's main resources. It
// verifies that a WFFC StorageClass can be resolved and uses the WaitForFirstConsumer
// volume binding mode.
type wffcStorageClassPrecheck struct{}

func (c *wffcStorageClassPrecheck) Label() string {
	return PrecheckWFFCStorageClass
}

func (c *wffcStorageClassPrecheck) Run(ctx context.Context, f *framework.Framework) error {
	if !isCheckEnabled(wffcStorageClassPrecheckEnvName) {
		_, _ = GinkgoWriter.Write([]byte("WFFC StorageClass precheck is disabled.\n"))
		return nil
	}

	k8sClient := f.GenericClient()
	var scList storagev1.StorageClassList
	if err := k8sClient.List(ctx, &scList); err != nil {
		return fmt.Errorf("%s=no to disable this precheck: list StorageClasses: %w", wffcStorageClassPrecheckEnvName, err)
	}

	wffcSC, err := config.ResolveWFFCStorageClass(&scList)
	if err != nil {
		return fmt.Errorf("%s=no to disable this precheck: %w", wffcStorageClassPrecheckEnvName, err)
	}
	if wffcSC == nil {
		return fmt.Errorf(
			"%s=no to disable this precheck: WFFC StorageClass not found. "+
				"Set %s explicitly or configure a default StorageClass with WaitForFirstConsumer binding, "+
				"or with Immediate binding and another WaitForFirstConsumer StorageClass on the same CSI driver",
			wffcStorageClassPrecheckEnvName, config.WFFCStorageClassEnv,
		)
	}

	if !config.IsWFFCBinding(wffcSC) {
		return fmt.Errorf(
			"%s=no to disable this precheck: WFFC StorageClass %q must use the WaitForFirstConsumer "+
				"volume binding mode, but it is %q",
			wffcStorageClassPrecheckEnvName, wffcSC.Name, config.VolumeBindingMode(wffcSC),
		)
	}

	_, _ = fmt.Fprintf(GinkgoWriter,
		"WFFC StorageClass precheck passed: the tests will use WFFC StorageClass %q (CSI driver %q).\n",
		wffcSC.Name, wffcSC.Provisioner,
	)

	return nil
}

// Register WFFCStorageClass precheck (not common - requires explicit label).
func init() {
	RegisterPrecheck(&wffcStorageClassPrecheck{}, false)
}
