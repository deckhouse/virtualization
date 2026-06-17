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
	mainStandbyStorageClassPrecheckEnvName = "MAIN_STANDBY_STORAGE_CLASS_PRECHECK"
)

// mainStandbyStorageClassPrecheck implements the Precheck interface for the main and
// standby StorageClasses used by block-device creation tests. It verifies that:
//  1. a StorageClass annotated with MainStorageClassAnnotation=true exists;
//  2. a StorageClass annotated with StandbyStorageClassAnnotation=true exists;
//  3. both StorageClasses are backed by the same CSI driver (provisioner).
type mainStandbyStorageClassPrecheck struct{}

func (c *mainStandbyStorageClassPrecheck) Label() string {
	return PrecheckMainStandbyStorageClass
}

func (c *mainStandbyStorageClassPrecheck) Run(ctx context.Context, f *framework.Framework) error {
	if !isCheckEnabled(mainStandbyStorageClassPrecheckEnvName) {
		_, _ = GinkgoWriter.Write([]byte("main/standby StorageClass precheck is disabled.\n"))
		return nil
	}

	k8sClient := f.GenericClient()
	var scList storagev1.StorageClassList
	if err := k8sClient.List(ctx, &scList); err != nil {
		return fmt.Errorf("%s=no to disable this precheck: list StorageClasses: %w", mainStandbyStorageClassPrecheckEnvName, err)
	}

	mainSC := config.FindStorageClassByAnnotation(&scList, config.MainStorageClassAnnotation)
	standbySC := config.FindStorageClassByAnnotation(&scList, config.StandbyStorageClassAnnotation)

	var missing []string
	if mainSC == nil {
		missing = append(missing, fmt.Sprintf("main StorageClass (annotation %s=true)", config.MainStorageClassAnnotation))
	}
	if standbySC == nil {
		missing = append(missing, fmt.Sprintf("standby StorageClass (annotation %s=true)", config.StandbyStorageClassAnnotation))
	}
	if len(missing) > 0 {
		return fmt.Errorf(
			"%s=no to disable this precheck: the e2e block-device tests require a main and a standby StorageClass, but %s not found.\n"+
				"Pick two StorageClasses backed by the SAME CSI driver and annotate them for the e2e tests:\n"+
				"  kubectl annotate storageclass/<main-sc-name> %s=true --overwrite\n"+
				"  kubectl annotate storageclass/<standby-sc-name> %s=true --overwrite",
			mainStandbyStorageClassPrecheckEnvName,
			strings.Join(missing, " and "),
			config.MainStorageClassAnnotation,
			config.StandbyStorageClassAnnotation,
		)
	}

	if mainSC.Provisioner != standbySC.Provisioner {
		return fmt.Errorf(
			"%s=no to disable this precheck: main StorageClass %q (CSI driver %q) and standby StorageClass %q (CSI driver %q) "+
				"must be backed by the same CSI driver.\n"+
				"Annotate two StorageClasses that share the same provisioner:\n"+
				"  kubectl annotate storageclass/<main-sc-name> %s=true --overwrite\n"+
				"  kubectl annotate storageclass/<standby-sc-name> %s=true --overwrite",
			mainStandbyStorageClassPrecheckEnvName,
			mainSC.Name, mainSC.Provisioner, standbySC.Name, standbySC.Provisioner,
			config.MainStorageClassAnnotation, config.StandbyStorageClassAnnotation,
		)
	}

	_, _ = fmt.Fprintf(GinkgoWriter,
		"main/standby StorageClass precheck passed: the tests will use main StorageClass %q and standby StorageClass %q (CSI driver %q).\n",
		mainSC.Name, standbySC.Name, mainSC.Provisioner,
	)

	return nil
}

// Register MainStandbyStorageClass precheck (not common - requires explicit label).
func init() {
	RegisterPrecheck(&mainStandbyStorageClassPrecheck{}, false)
}
