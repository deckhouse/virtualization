package readiness

import (
	"context"
	"fmt"

	"hooks/pkg/settings"

	"github.com/deckhouse/module-sdk/pkg"
	"github.com/deckhouse/module-sdk/pkg/app"
)

var ReadinessConfig = app.ReadinessConfig{
	ProbeFunc: checkModuleReadiness,
}

func checkModuleReadiness(ctx context.Context, input *pkg.HookInput) error {
	readinessObj := input.Values.Get(settings.InternalValuesReadinessPath)
	if !readinessObj.IsObject() {
		return fmt.Errorf("module is not ready yet")
	}
	validationErr := readinessObj.Get("moduleConfigValidationError")
	if validationErr.Exists() {
		return fmt.Errorf(validationErr.String())
	}
	return nil
}
