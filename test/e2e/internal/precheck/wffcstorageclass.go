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
// verifies that a StorageClass annotated with WFFCStorageClassAnnotation=true exists and
// uses the WaitForFirstConsumer volume binding mode.
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

	wffcSC := config.FindStorageClassByAnnotation(&scList, config.WFFCStorageClassAnnotation)
	if wffcSC == nil {
		return fmt.Errorf(
			"%s=no to disable this precheck: WFFC StorageClass not found. Annotate a StorageClass that uses the "+
				"WaitForFirstConsumer volume binding mode for the e2e tests:\n"+
				"  kubectl annotate storageclass/<wffc-sc-name> %s=true --overwrite",
			wffcStorageClassPrecheckEnvName, config.WFFCStorageClassAnnotation,
		)
	}

	if !isWFFCBinding(wffcSC) {
		return fmt.Errorf(
			"%s=no to disable this precheck: WFFC StorageClass %q annotated with %s=true must use the WaitForFirstConsumer "+
				"volume binding mode, but it is %q.\n"+
				"Annotate a StorageClass that uses the WaitForFirstConsumer volume binding mode:\n"+
				"  kubectl annotate storageclass/<wffc-sc-name> %s=true --overwrite",
			wffcStorageClassPrecheckEnvName,
			wffcSC.Name, config.WFFCStorageClassAnnotation, volumeBindingMode(wffcSC),
			config.WFFCStorageClassAnnotation,
		)
	}

	_, _ = fmt.Fprintf(GinkgoWriter,
		"WFFC StorageClass precheck passed: the tests will use WFFC StorageClass %q (CSI driver %q).\n",
		wffcSC.Name, wffcSC.Provisioner,
	)

	return nil
}

// isWFFCBinding reports whether sc uses the WaitForFirstConsumer volume binding mode.
func isWFFCBinding(sc *storagev1.StorageClass) bool {
	return sc.VolumeBindingMode != nil && *sc.VolumeBindingMode == storagev1.VolumeBindingWaitForFirstConsumer
}

// Register WFFCStorageClass precheck (not common - requires explicit label).
func init() {
	RegisterPrecheck(&wffcStorageClassPrecheck{}, false)
}
