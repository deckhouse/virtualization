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
	verticalPodAutoscalerModuleName     = "vertical-pod-autoscaler"
	verticalPodAutoscalerCheckEnvName   = "VERTICAL_POD_AUTOSCALER_PRECHECK"
	verticalPodAutoscalerRequiredModule = "vertical-pod-autoscaler module should be enabled and ready to run coreFraction autoscaling"
)

// verticalPodAutoscalerPrecheck requires the vertical-pod-autoscaler module: its
// recommender is the engine behind coreFraction: "auto".
type verticalPodAutoscalerPrecheck struct{}

func (p *verticalPodAutoscalerPrecheck) Label() string {
	return PrecheckVerticalPodAutoscaler
}

func (p *verticalPodAutoscalerPrecheck) Run(ctx context.Context, f *framework.Framework) error {
	if !isCheckEnabled(verticalPodAutoscalerCheckEnvName) {
		_, _ = GinkgoWriter.Write([]byte("vertical-pod-autoscaler module check is disabled.\n"))
		return nil
	}

	module := &dv1alpha1.Module{}
	if err := f.GenericClient().Get(ctx, client.ObjectKey{Name: verticalPodAutoscalerModuleName}, module); err != nil {
		return fmt.Errorf("%s=no to disable this precheck: failed to check vertical-pod-autoscaler module status: %w", verticalPodAutoscalerCheckEnvName, err)
	}

	if !IsModuleEnabled(module) {
		return fmt.Errorf("%s=no to disable this precheck: %s", verticalPodAutoscalerCheckEnvName, verticalPodAutoscalerRequiredModule)
	}

	if module.Status.Phase != modulePhaseReady {
		return fmt.Errorf("%s=no to disable this precheck: %s; current status: %s", verticalPodAutoscalerCheckEnvName, verticalPodAutoscalerRequiredModule, module.Status.Phase)
	}

	return nil
}

func init() {
	RegisterPrecheck(&verticalPodAutoscalerPrecheck{}, false)
}
