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
	"slices"

	. "github.com/onsi/ginkgo/v2"
	"sigs.k8s.io/controller-runtime/pkg/client"

	dv1alpha1 "github.com/deckhouse/virtualization/test/e2e/internal/api/deckhouse/v1alpha1"
	"github.com/deckhouse/virtualization/test/e2e/internal/framework"
)

type featureGatePrecheck struct {
	label string
	env   string
	gates []string
}

func (p featureGatePrecheck) Label() string {
	return p.label
}

func (p featureGatePrecheck) Run(ctx context.Context, f *framework.Framework) error {
	if !isCheckEnabled(p.env) {
		_, _ = fmt.Fprintf(GinkgoWriter, "%s check is disabled", p.label)
		return nil
	}

	virtualizationModuleConfig := &dv1alpha1.ModuleConfig{}
	err := f.GenericClient().Get(ctx, client.ObjectKey{Name: virtualizationModuleName}, virtualizationModuleConfig)
	if err != nil {
		return fmt.Errorf("%s=no to disable this precheck: failed to get virtualization module config spec: %w", p.env, err)
	}

	gates, err := getFeatureGates(virtualizationModuleConfig)
	if err != nil {
		return fmt.Errorf("%s=no to disable this precheck: %w", p.env, err)
	}

	for _, gate := range p.gates {
		if !slices.Contains(gates, gate) {
			return fmt.Errorf("%s=no to disable this precheck: feature gate %s is not enabled in virtualization module config spec", p.env, gate)
		}
	}

	return nil
}

const (
	hotplugCPUWithLiveMigrationCheckEnvName    = "HOTPLUG_CPU_LIVE_MIGRATION_PRECHECK"
	hotplugMemoryWithLiveMigrationCheckEnvName = "HOTPLUG_MEMORY_LIVE_MIGRATION_PRECHECK"
	hotplugInPlaceCheckEnvName                 = "HOTPLUG_IN_PLACE_PRECHECK"
)

func init() {
	RegisterPrecheck(&featureGatePrecheck{
		label: HotplugCPUWithLiveMigrationPrecheck,
		env:   hotplugCPUWithLiveMigrationCheckEnvName,
		gates: []string{"HotplugCPUWithLiveMigration"},
	}, false)

	RegisterPrecheck(&featureGatePrecheck{
		label: HotplugMemoryWithLiveMigrationPrecheck,
		env:   hotplugMemoryWithLiveMigrationCheckEnvName,
		gates: []string{"HotplugMemoryWithLiveMigration"},
	}, false)

	RegisterPrecheck(&featureGatePrecheck{
		label: HotplugInPlaceResizePrecheck,
		env:   hotplugInPlaceCheckEnvName,
		gates: []string{"HotplugCPUAndMemoryWithInPlaceResize"},
	}, false)
}

func getFeatureGates(mc *dv1alpha1.ModuleConfig) ([]string, error) {
	gates, ok := mc.Spec.Settings["featureGates"].([]interface{})
	if !ok {
		return nil, fmt.Errorf("failed to get feature gates from virtualization module config spec: %T", gates)
	}
	var result []string
	for _, g := range gates {
		if s, ok := g.(string); ok {
			result = append(result, s)
		} else {
			return nil, fmt.Errorf("failed to get feature gates from virtualization module config spec: %T", gates)
		}
	}

	return result, nil
}
