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

package vmop

import (
	"time"

	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/manager"

	"github.com/deckhouse/deckhouse/pkg/log"
	commonvmop "github.com/deckhouse/virtualization-controller/pkg/common/vmop"
	"github.com/deckhouse/virtualization-controller/pkg/config"
	"github.com/deckhouse/virtualization-controller/pkg/controller/gc"
	virtv2 "github.com/deckhouse/virtualization/api/core/v1alpha2"
)

const gcControllerName = "vmop-gc-controller"

func SetupGC(
	mgr manager.Manager,
	log *log.Logger,
	gcSettings config.BaseGcSettings,
) error {
	ttl := 24 * time.Hour
	if gcSettings.TTL.Duration > 0 {
		ttl = gcSettings.TTL.Duration
	}
	return gc.SetupGcController(gcControllerName,
		mgr,
		log,
		gc.NewCronSource(mgr.GetClient(),
			gcSettings.Schedule,
			&virtv2.VirtualMachineOperationList{},
			gc.NewDefaultCronSourceOption(&virtv2.VirtualMachineOperationList{}, ttl, log),
			log.With("resource", "vmop"),
		),
		func() client.Object {
			return &virtv2.VirtualMachineOperation{}
		},
		func(obj client.Object) bool {
			vmop, ok := obj.(*virtv2.VirtualMachineOperation)
			if !ok {
				return false
			}
			return commonvmop.IsFinished(vmop)
		},
	)
}
