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

package livemigration

import (
	"context"
	"log/slog"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"sigs.k8s.io/controller-runtime/pkg/client"

	mcapi "github.com/deckhouse/virtualization-controller/pkg/controller/moduleconfig/api"
	"github.com/deckhouse/virtualization-controller/pkg/logger"
)

const (
	moduleConfigName                = "virtualization"
	systemMigrationPolicyAnnotation = "virtualization.deckhouse.io/system-migration-policy"
)

func GetSystemMigrationPolicyAnnotation(ctx context.Context, kubeClient client.Client) string {
	moduleConfig := &mcapi.ModuleConfig{}
	err := kubeClient.Get(ctx, client.ObjectKey{Name: moduleConfigName}, moduleConfig)
	if err != nil {
		if !apierrors.IsNotFound(err) {
			slog.Default().Error("failed to get ModuleConfig virtualization", logger.SlogErr(err))
		}
		return ""
	}

	return moduleConfig.GetAnnotations()[systemMigrationPolicyAnnotation]
}
