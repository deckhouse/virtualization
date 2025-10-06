/*
Copyright 2025 Flant JSC

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

package migration

import (
	"context"
	"fmt"
	"log/slog"
	"strconv"
	"strings"

	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/deckhouse/deckhouse/pkg/log"
	"github.com/deckhouse/virtualization-controller/pkg/common/patch"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
)

const (
	coreFractionsFormatMigrationName = "core-fractions-format"
)

func newCoreFractionsFormat(client client.Client, logger *log.Logger) (Migration, error) {
	return &coreFractionsFormat{
		client: client,
		logger: logger,
	}, nil
}

type coreFractionsFormat struct {
	client client.Client
	logger *log.Logger
}

func (m *coreFractionsFormat) Name() string {
	return coreFractionsFormatMigrationName
}

func (m *coreFractionsFormat) Migrate(ctx context.Context) error {
	vmcList := &v1alpha2.VirtualMachineClassList{}
	if err := m.client.List(ctx, vmcList); err != nil {
		return fmt.Errorf("failed to list VirtualMachineClasses: %w", err)
	}

	for i := range vmcList.Items {
		vmc := &vmcList.Items[i]

		needUpdate, genPatch, err := m.genPatch(vmc)
		if err != nil {
			return fmt.Errorf("failed to generate patch for VMClass %s: %w", vmc.Name, err)
		}
		if !needUpdate {
			continue
		}

		m.logger.Info("Migrating VMClass coreFractions format",
			slog.String("name", vmc.Name),
		)

		if m.logger.GetLevel() <= log.LevelDebug {
			if data, err := genPatch.Data(vmc); err == nil {
				m.logger.Debug("Patch VMClass",
					slog.String("name", vmc.Name),
					slog.String("data", string(data)),
				)
			}
		}

		if err := m.client.Patch(ctx, vmc, genPatch); err != nil {
			return fmt.Errorf("failed to patch VMClass %s: %w", vmc.Name, err)
		}
	}

	return nil
}

func (m *coreFractionsFormat) genPatch(vmc *v1alpha2.VirtualMachineClass) (bool, client.Patch, error) {
	var ops []patch.JSONPatchOperation

	for policyIdx, policy := range vmc.Spec.SizingPolicies {
		if len(policy.CoreFractions) == 0 {
			continue
		}

		for fractionIdx, fraction := range policy.CoreFractions {
			newValue, changed := normalizeCoreFraction(string(fraction))
			if !changed {
				continue
			}

			ops = append(ops, patch.NewJSONPatchOperation(
				patch.PatchReplaceOp,
				fmt.Sprintf("/spec/sizingPolicies/%d/coreFractions/%d", policyIdx, fractionIdx),
				newValue,
			))
		}
	}

	if len(ops) == 0 {
		return false, nil, nil
	}

	bytes, err := patch.NewJSONPatch(ops...).Bytes()
	if err != nil {
		return false, nil, fmt.Errorf("failed to create JSON patch: %w", err)
	}

	return true, client.RawPatch(types.JSONPatchType, bytes), nil
}

func normalizeCoreFraction(value string) (string, bool) {
	trimmed := strings.TrimSpace(value)

	if strings.HasSuffix(trimmed, "%") {
		return trimmed, false
	}

	if num, err := strconv.Atoi(trimmed); err == nil && num >= 1 && num <= 100 {
		return fmt.Sprintf("%d%%", num), true
	}

	return value, false
}
