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
	"strings"

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
//  2. a StorageClass annotated with DifferentCSIDriverStorageClassAnnotation=true exists;
//  3. the two StorageClasses are backed by different CSI drivers (provisioners).
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
	differentSC := config.FindStorageClassByAnnotation(&scList, config.DifferentCSIDriverStorageClassAnnotation)

	var missing []string
	if mainSC == nil {
		missing = append(missing, fmt.Sprintf("main StorageClass (annotation %s=true)", config.MainStorageClassAnnotation))
	}
	if differentSC == nil {
		missing = append(missing, fmt.Sprintf("different-CSI-driver StorageClass (annotation %s=true)", config.DifferentCSIDriverStorageClassAnnotation))
	}
	if len(missing) > 0 {
		return fmt.Errorf(
			"%s=no to disable this precheck: the cross-CSI block-device tests require a main StorageClass and a StorageClass backed by a different CSI driver, but %s not found.\n"+
				"Pick two StorageClasses backed by DIFFERENT CSI drivers and annotate them for the e2e tests:\n"+
				"  kubectl annotate storageclass/<main-sc-name> %s=true --overwrite\n"+
				"  kubectl annotate storageclass/<other-csi-sc-name> %s=true --overwrite",
			differentCSIDriverStorageClassPrecheckEnvName,
			strings.Join(missing, " and "),
			config.MainStorageClassAnnotation,
			config.DifferentCSIDriverStorageClassAnnotation,
		)
	}

	if mainSC.Provisioner == differentSC.Provisioner {
		return fmt.Errorf(
			"%s=no to disable this precheck: main StorageClass %q and StorageClass %q both use the CSI driver %q, "+
				"but this test requires two StorageClasses backed by DIFFERENT CSI drivers.\n"+
				"Annotate a StorageClass that uses a different provisioner than %q:\n"+
				"  kubectl annotate storageclass/<other-csi-sc-name> %s=true --overwrite",
			differentCSIDriverStorageClassPrecheckEnvName,
			mainSC.Name, differentSC.Name, mainSC.Provisioner, mainSC.Provisioner,
			config.DifferentCSIDriverStorageClassAnnotation,
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
