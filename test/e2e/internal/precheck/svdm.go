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

	"github.com/deckhouse/virtualization/test/e2e/internal/framework"
)

const (
	// svdmModuleName is the deprecated Storage Volume Data Manager module. Data exports
	// moved to storage-foundation (v1.0.0+), which serves the
	// dataexports.storage-foundation.deckhouse.io API that d8 v0.33.8+ uses.
	svdmModuleName         = "storage-volume-data-manager"
	svdmModuleCheckEnvName = "SVDM_MODULE_PRECHECK"

	// dataExportModuleName provides the data-export machinery (DataExport CRD,
	// data-manager controller and populator).
	dataExportModuleName = "storage-foundation"
)

// svdmPrecheck implements Precheck interface for the data-export machinery.
type svdmPrecheck struct{}

func (s *svdmPrecheck) Label() string {
	return PrecheckSVDM
}

func (s *svdmPrecheck) Run(ctx context.Context, f *framework.Framework) error {
	if !isCheckEnabled(svdmModuleCheckEnvName) {
		_, _ = GinkgoWriter.Write([]byte("data-export modules check is disabled.\n"))
		return nil
	}

	if err := RequireModuleDisabled(ctx, f, svdmModuleName); err != nil {
		return fmt.Errorf("%s=no to disable this precheck: %w", svdmModuleCheckEnvName, err)
	}

	if err := RequireModuleReady(ctx, f, dataExportModuleName); err != nil {
		return fmt.Errorf("%s=no to disable this precheck: %w", svdmModuleCheckEnvName, err)
	}

	return nil
}

// Register SVDM precheck (not common - requires explicit label).
func init() {
	RegisterPrecheck(&svdmPrecheck{}, false)
}
