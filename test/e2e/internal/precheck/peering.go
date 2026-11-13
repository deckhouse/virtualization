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

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/deckhouse/virtualization/test/e2e/internal/framework"
)

type peeringPrecheck struct{}

func (*peeringPrecheck) Label() string { return PrecheckPeering }

func (*peeringPrecheck) Run(ctx context.Context, f *framework.Framework) error {
	if !isCheckEnabled(peeringCheckEnvName) {
		return nil
	}

	deployment, err := f.KubeClient().AppsV1().Deployments(framework.ModuleNamespace).Get(ctx, "virt-synchronization-controller", metav1.GetOptions{})
	if err != nil {
		return fmt.Errorf("peering migration requires a commercial virtualization build with a synchronization controller: %w", err)
	}

	if deployment.Status.AvailableReplicas == 0 {
		return fmt.Errorf("wait for deployment %s/%s to become available before testing peering migration", deployment.Namespace, deployment.Name)
	}

	return nil
}
