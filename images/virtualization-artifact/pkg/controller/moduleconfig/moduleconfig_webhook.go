/*
Copyright 2024 Flant JSC

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
	"log/slog"

	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/manager"

	"github.com/deckhouse/deckhouse/pkg/log"
	appconfig "github.com/deckhouse/virtualization-controller/pkg/config"
	mcapi "github.com/deckhouse/virtualization-controller/pkg/controller/moduleconfig/api"
	"github.com/deckhouse/virtualization-controller/pkg/controller/validator"
)

const moduleConfigName = "virtualization"

func SetupWebhookWithManager(mgr manager.Manager, clusterSubnets *appconfig.ClusterSubnets) error {
	moduleConfigValidator := NewModuleConfigValidator(mgr.GetClient(), clusterSubnets)
	if err := builder.WebhookManagedBy(mgr).
		For(&mcapi.ModuleConfig{}).
		WithValidator(moduleConfigValidator).
		Complete(); err != nil {
		return err
	}
	return nil
}

func NewModuleConfigValidator(client client.Client, clusterSubnets *appconfig.ClusterSubnets) *validator.Validator[*mcapi.ModuleConfig] {
	logger := log.Default().With(slog.String("validator", "moduleconfig"))

	cidrs := newCIDRsValidator(client, clusterSubnets)
	reduceCIDRs := newRemoveCIDRsValidator(client)
	viStorageClasses := newViStorageClassValidator(client)
	dvcrValidator := newDvcrValidator(client)

	return validator.NewValidator[*mcapi.ModuleConfig](logger).
		WithPredicate(&validator.Predicate[*mcapi.ModuleConfig]{
			Update: func(oldMC, newMC *mcapi.ModuleConfig) bool {
				return newMC.GetName() == moduleConfigName &&
					oldMC.GetGeneration() != newMC.GetGeneration()
			},
		}).
		WithUpdateValidators(cidrs, reduceCIDRs, viStorageClasses, dvcrValidator)
}
