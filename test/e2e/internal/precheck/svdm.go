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
	"sigs.k8s.io/controller-runtime/pkg/client"

	dv1alpha1 "github.com/deckhouse/virtualization/test/e2e/internal/api/deckhouse/v1alpha1"
	"github.com/deckhouse/virtualization/test/e2e/internal/framework"
)

const (
	// svdmModuleName is the name of the Storage Volume Data Manager module in Deckhouse.
	svdmModuleName         = "storage-volume-data-manager"
	svdmModuleCheckEnvName = "SVDM_MODULE_PRECHECK"
)

// svdmPrecheck implements Precheck interface for SVDM module.
type svdmPrecheck struct{}

func (s *svdmPrecheck) Label() string {
	return PrecheckSVDM
}

func (s *svdmPrecheck) Run(ctx context.Context, f *framework.Framework) error {
	if !isCheckEnabled(svdmModuleCheckEnvName) {
		_, _ = GinkgoWriter.Write([]byte("Storage Volume Data Manager (SVDM) module check is disabled.\n"))
		return nil
	}

	if !IsModuleEnabled(ctx, f, svdmModuleName) {
		return fmt.Errorf("%s=no to disable this precheck: Storage Volume Data Manager module should be enabled", svdmModuleCheckEnvName)
	}

	svdmModule := &dv1alpha1.Module{}
	err := f.GenericClient().Get(ctx, client.ObjectKey{Name: svdmModuleName}, svdmModule)
	if err != nil {
		return fmt.Errorf("%s=no to disable this precheck: failed to check SVDM module status: %w", svdmModuleCheckEnvName, err)
	}
	if svdmModule.Status.Phase != modulePhaseReady {
		return fmt.Errorf("%s=no to disable this precheck: Storage Volume Data Manager module should be ready; current status: %s", svdmModuleCheckEnvName, svdmModule.Status.Phase)
	}

	return nil
}

// Register SVDM precheck (not common - requires explicit label).
func init() {
	RegisterPrecheck(&svdmPrecheck{}, false)
}
