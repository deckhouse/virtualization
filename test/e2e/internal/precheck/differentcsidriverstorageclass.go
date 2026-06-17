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
	differentCSIDriverStorageClassPrecheckEnvName = "DIFFERENT_CSI_DRIVER_STORAGE_CLASS_PRECHECK"
)

// differentCSIDriverStorageClassPrecheck implements the Precheck interface for the
// cross-CSI-driver block-device tests. It verifies that:
//  1. a StorageClass annotated with MainStorageClassAnnotation=true exists;
//  2. the cluster has at least one StorageClass whose CSI driver (provisioner) differs
//     from the main one.
//
// The "different CSI driver" StorageClass is discovered automatically; no annotation is
// required for it.
type differentCSIDriverStorageClassPrecheck struct{}

func (c *differentCSIDriverStorageClassPrecheck) Label() string {
	return PrecheckDifferentCSIDriverStorageClass
}

func (c *differentCSIDriverStorageClassPrecheck) Run(ctx context.Context, f *framework.Framework) error {
	if !isCheckEnabled(differentCSIDriverStorageClassPrecheckEnvName) {
		_, _ = GinkgoWriter.Write([]byte("different CSI driver StorageClass precheck is disabled.\n"))
		return nil
	}

	k8sClient := f.GenericClient()
	var scList storagev1.StorageClassList
	if err := k8sClient.List(ctx, &scList); err != nil {
		return fmt.Errorf("%s=no to disable this precheck: list StorageClasses: %w", differentCSIDriverStorageClassPrecheckEnvName, err)
	}

	mainSC := config.FindStorageClassByAnnotation(&scList, config.MainStorageClassAnnotation)
	if mainSC == nil {
		return fmt.Errorf(
			"%s=no to disable this precheck: main StorageClass not found. Annotate one for the e2e tests:\n"+
				"  kubectl annotate storageclass/<main-sc-name> %s=true --overwrite",
			differentCSIDriverStorageClassPrecheckEnvName, config.MainStorageClassAnnotation,
		)
	}

	differentSC := config.FindStorageClassWithDifferentProvisioner(&scList, mainSC.Provisioner)
	if differentSC == nil {
		return fmt.Errorf(
			"%s=no to disable this precheck: no StorageClass with a CSI driver different from the main StorageClass %q (CSI driver %q) was found in the cluster. "+
				"The cross-CSI block-device tests require a second CSI driver to be installed.",
			differentCSIDriverStorageClassPrecheckEnvName, mainSC.Name, mainSC.Provisioner,
		)
	}

	_, _ = fmt.Fprintf(GinkgoWriter,
		"different CSI driver StorageClass precheck passed: the tests will use main StorageClass %q (CSI driver %q) and StorageClass %q (CSI driver %q).\n",
		mainSC.Name, mainSC.Provisioner, differentSC.Name, differentSC.Provisioner,
	)

	return nil
}

// Register DifferentCSIDriverStorageClass precheck (not common - requires explicit label).
func init() {
	RegisterPrecheck(&differentCSIDriverStorageClassPrecheck{}, false)
}
