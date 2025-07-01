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

package vm

import (
	"time"

	virtv1 "kubevirt.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/manager"

	"github.com/deckhouse/deckhouse/pkg/log"

	"github.com/deckhouse/virtualization-controller/pkg/config/apis/componentconfig"
	"github.com/deckhouse/virtualization-controller/pkg/controller/gc"
)

const GCVMMigrationControllerName = "vmi-migration-gc-controller"

func SetupGC(
	mgr manager.Manager,
	log *log.Logger,
	gcSettings componentconfig.BaseGCSettings,
) error {
	ttl := 24 * time.Hour
	if gcSettings.TTL.Duration > 0 {
		ttl = gcSettings.TTL.Duration
	}
	return gc.SetupGcController(GCVMMigrationControllerName,
		mgr,
		log,
		gc.NewCronSource(mgr.GetClient(),
			gcSettings.Schedule,
			&virtv1.VirtualMachineInstanceMigrationList{},
			gc.NewDefaultCronSourceOption(&virtv1.VirtualMachineInstanceMigrationList{}, ttl, log),
			log.With("resource", "vmi-migration"),
		),
		func() client.Object {
			return &virtv1.VirtualMachineInstanceMigration{}
		},
		func(obj client.Object) bool {
			migration, ok := obj.(*virtv1.VirtualMachineInstanceMigration)
			if !ok {
				return false
			}
			return vmiMigrationIsFinal(migration)
		},
	)
}

func vmiMigrationIsFinal(migration *virtv1.VirtualMachineInstanceMigration) bool {
	return migration.Status.Phase == virtv1.MigrationFailed || migration.Status.Phase == virtv1.MigrationSucceeded
}
