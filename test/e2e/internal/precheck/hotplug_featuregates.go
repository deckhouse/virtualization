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
	"k8s.io/component-base/featuregate"

	"github.com/deckhouse/virtualization-controller/pkg/featuregates"
	"github.com/deckhouse/virtualization/test/e2e/internal/framework"
)

const (
	hotplugCPUPrecheckEnvName    = "HOTPLUG_CPU_PRECHECK"
	hotplugMemoryPrecheckEnvName = "HOTPLUG_MEMORY_PRECHECK"
)

type moduleConfigFeatureGatePrecheck struct {
	label       string
	envName     string
	featureGate featuregate.Feature
}

func (p *moduleConfigFeatureGatePrecheck) Label() string {
	return p.label
}

func (p *moduleConfigFeatureGatePrecheck) Run(ctx context.Context, f *framework.Framework) error {
	if !isCheckEnabled(p.envName) {
		_, _ = GinkgoWriter.Write([]byte(fmt.Sprintf("%s check is disabled.\n", p.featureGate)))
		return nil
	}

	moduleConfig, err := f.GetVirtualizationModuleConfig(ctx)
	if err != nil {
		return fmt.Errorf("%s=no to disable this precheck: failed to get ModuleConfig/%s: %w", p.envName, virtualizationModuleName, err)
	}

	featureGates := moduleConfig.Spec.Settings.FeatureGates
	hasFeatureGate := false
	for _, fg := range featureGates {
		if fg == string(p.featureGate) {
			hasFeatureGate = true
			break
		}
	}

	if !hasFeatureGate {
		return fmt.Errorf("%s=no to disable this precheck: ModuleConfig/%s does not contain %q in .spec.settings.featureGates",
			p.envName, p.featureGate, virtualizationModuleName)
	}

	return nil
}

func init() {
	RegisterPrecheck(&moduleConfigFeatureGatePrecheck{
		label:       PrecheckHotplugCPU,
		envName:     hotplugCPUPrecheckEnvName,
		featureGate: featuregates.HotplugCPUWithLiveMigration,
	}, false)

	RegisterPrecheck(&moduleConfigFeatureGatePrecheck{
		label:       PrecheckHotplugMemory,
		envName:     hotplugMemoryPrecheckEnvName,
		featureGate: featuregates.HotplugMemoryWithLiveMigration,
	}, false)
}
