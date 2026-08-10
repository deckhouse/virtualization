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

package moduleconfig

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"

	k8sversion "k8s.io/apimachinery/pkg/util/version"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	mcapi "github.com/deckhouse/virtualization-controller/pkg/controller/moduleconfig/api"
	"github.com/deckhouse/virtualization-controller/pkg/featuregates"
	"github.com/deckhouse/virtualization-controller/pkg/version"
)

const inPlaceResizeFeatureGate = string(featuregates.HotplugCPUAndMemoryWithInPlaceResize)

// inPlaceResizeMinKubeVersion is the first Kubernetes release carrying the PodResizePending and
// PodResizeInProgress pod conditions (KEP-1287); before it kubelet reports the resize progress in
// the deprecated pod.status.resize field. KubeVirt tracks the in-place resize by those conditions
// only, so on older clusters CPU and memory hotplug never completes: the virt-launcher pod is
// resized, but the guest keeps the old topology and no migration is started either.
var inPlaceResizeMinKubeVersion = k8sversion.MustParseGeneric("1.33")

// errKubernetesVersionUnknown is returned when the cluster version did not reach the validator:
// the requirement cannot be checked, so the gate is not let through. A sentinel, so that the
// wording of the message stays free to change without breaking the test that covers this case.
var errKubernetesVersionUnknown = errors.New("can't validate it with absent Kubernetes version: check Deckhouse health and try to enable this feature later")

type featureGatesValidator struct {
	edition           string
	lockedToDisabled  func(name string) bool
	kubernetesVersion *k8sversion.Version
}

func newFeatureGatesValidator(kubernetesVersion *k8sversion.Version) *featureGatesValidator {
	return &featureGatesValidator{
		edition:           version.GetEdition(),
		lockedToDisabled:  featuregates.LockedToDisabled,
		kubernetesVersion: kubernetesVersion,
	}
}

func (v featureGatesValidator) ValidateUpdate(_ context.Context, oldMC, newMC *mcapi.ModuleConfig) (admission.Warnings, error) {
	oldSettings, err := mcapi.LoadModuleSettings(oldMC.Spec.Settings)
	if err != nil {
		return nil, fmt.Errorf("spec.settings: %w", err)
	}

	newSettings, err := mcapi.LoadModuleSettings(newMC.Spec.Settings)
	if err != nil {
		return nil, fmt.Errorf("spec.settings: %w", err)
	}

	// Validate the transition, not the state: already enabled feature gates are not re-checked, so
	// editing unrelated settings keeps working even on a cluster that cannot support them.
	return nil, v.validate(addedFeatureGates(oldSettings.FeatureGates, newSettings.FeatureGates))
}

func (v featureGatesValidator) validate(gates []string) error {
	if len(gates) == 0 {
		return nil
	}

	if err := v.validateAvailableInEdition(gates); err != nil {
		return err
	}

	if slices.Contains(gates, inPlaceResizeFeatureGate) {
		return v.validateInPlaceResizeSupported()
	}

	return nil
}

// validateAvailableInEdition rejects gates the controller would refuse to enable anyway. Without
// this check the controller exits on the next start, and since it also serves this very webhook,
// the offending ModuleConfig can no longer be fixed back.
func (v featureGatesValidator) validateAvailableInEdition(gates []string) error {
	var unavailable []string
	for _, gate := range gates {
		if v.lockedToDisabled(gate) {
			unavailable = append(unavailable, gate)
		}
	}

	if len(unavailable) == 0 {
		return nil
	}

	return fmt.Errorf(
		"spec.settings.featureGates: feature gates not available in the %s edition: %s",
		v.edition, strings.Join(unavailable, ", "),
	)
}

func (v featureGatesValidator) validateInPlaceResizeSupported() error {
	if v.kubernetesVersion == nil {
		return fmt.Errorf("spec.settings.featureGates: %s: %w", inPlaceResizeFeatureGate, errKubernetesVersionUnknown)
	}

	if v.kubernetesVersion.LessThan(inPlaceResizeMinKubeVersion) {
		return fmt.Errorf(
			"spec.settings.featureGates: %s requires Kubernetes %s or newer on the control plane and on every node running virtual machines, but the cluster runs %s; use %s and %s instead",
			inPlaceResizeFeatureGate, inPlaceResizeMinKubeVersion.String(), v.kubernetesVersion.String(),
			featuregates.HotplugCPUWithLiveMigration, featuregates.HotplugMemoryWithLiveMigration,
		)
	}

	return nil
}

func addedFeatureGates(current, desired []string) []string {
	var added []string
	for _, gate := range desired {
		if !slices.Contains(current, gate) {
			added = append(added, gate)
		}
	}

	return added
}
