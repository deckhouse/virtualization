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
	virtualizationModuleName         = "virtualization"
	virtualizationModuleCheckEnvName = "VIRTUALIZATION_PRECHECK"
)

// virtualizationPrecheck implements Precheck interface for virtualization module.
type virtualizationPrecheck struct{}

func (v *virtualizationPrecheck) Label() string {
	return PrecheckVirtualization
}

func (v *virtualizationPrecheck) Run(ctx context.Context, f *framework.Framework) error {
	if !isCheckEnabled(virtualizationModuleCheckEnvName) {
		_, _ = GinkgoWriter.Write([]byte("virtualization module check is disabled.\n"))
		return nil
	}

	if !IsModuleEnabled(ctx, f, virtualizationModuleName) {
		return fmt.Errorf("%s=no to disable this precheck: virtualization module should be enabled", virtualizationModuleCheckEnvName)
	}

	// Check virtualization module status
	virtualizationModule := &dv1alpha1.Module{}
	err := f.GenericClient().Get(ctx, client.ObjectKey{Name: virtualizationModuleName}, virtualizationModule)
	if err != nil {
		return fmt.Errorf("%s=no to disable this precheck: failed to check virtualization module status: %w", virtualizationModuleCheckEnvName, err)
	}
	if virtualizationModule.Status.Phase != modulePhaseReady {
		return fmt.Errorf("%s=no to disable this precheck: virtualization module should be ready; current status: %s", virtualizationModuleCheckEnvName, virtualizationModule.Status.Phase)
	}

	return nil
}

// Register virtualization precheck as common (runs for all tests).
func init() {
	RegisterPrecheck(&virtualizationPrecheck{}, true)
}
