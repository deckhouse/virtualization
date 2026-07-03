package moduleconfig

import (
	"context"
	"fmt"

	"sigs.k8s.io/controller-runtime/pkg/client"

	mcapi "github.com/deckhouse/virtualization-controller/pkg/controller/moduleconfig/api"
)

func HasVirtualMachineCIDRs(settings mcapi.SettingsValues) (bool, error) {
	cidrs, err := ParseCIDRs(settings)
	if err != nil {
		return false, err
	}
	return len(cidrs) > 0, nil
}

func ModuleConfigHasVirtualMachineCIDRs(ctx context.Context, c client.Client) (bool, error) {
	if c == nil {
		return false, fmt.Errorf("kubernetes client is nil")
	}

	var moduleConfig mcapi.ModuleConfig
	if err := c.Get(ctx, client.ObjectKey{Name: moduleConfigName}, &moduleConfig); err != nil {
		return false, fmt.Errorf("get ModuleConfig/%s: %w", moduleConfigName, err)
	}

	return HasVirtualMachineCIDRs(moduleConfig.Spec.Settings)
}
