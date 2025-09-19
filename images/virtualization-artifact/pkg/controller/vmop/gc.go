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
	"context"
	"time"

	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/manager"

	"github.com/deckhouse/deckhouse/pkg/log"

	commonvmop "github.com/deckhouse/virtualization-controller/pkg/common/vmop"
	"github.com/deckhouse/virtualization-controller/pkg/config"
	"github.com/deckhouse/virtualization-controller/pkg/controller/gc"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
)

const gcControllerName = "vmop-gc-controller"

func SetupGC(mgr manager.Manager, log *log.Logger, gcSettings config.BaseGcSettings) error {
	vmopGCMgr := newVMOPGCManager(mgr.GetClient(), gcSettings.TTL.Duration, 10)
	source, err := gc.NewCronSource(gcSettings.Schedule, vmopGCMgr, log.With("resource", "vmop"))
	if err != nil {
		return err
	}

	return gc.SetupGcController(gcControllerName,
		mgr,
		log,
		source,
		vmopGCMgr,
	)
}

func newVMOPGCManager(client client.Client, ttl time.Duration, max int) *vmopGCManager {
	if ttl == 0 {
		ttl = 24 * time.Hour
	}
	if max == 0 {
		max = 10
	}
	return &vmopGCManager{
		client: client,
		ttl:    ttl,
		max:    max,
	}
}

var (
	_ gc.ReconcileGCManager = &vmopGCManager{}
	_ gc.SourceGCManager    = &vmopGCManager{}
)

type vmopGCManager struct {
	client client.Client
	ttl    time.Duration
	max    int
}

func (m *vmopGCManager) New() client.Object {
	return &v1alpha2.VirtualMachineOperation{}
}

func (m *vmopGCManager) ShouldBeDeleted(obj client.Object) bool {
	vmop, ok := obj.(*v1alpha2.VirtualMachineOperation)
	if !ok {
		return false
	}
	return commonvmop.IsFinished(vmop)
}

func (m *vmopGCManager) ListForDelete(ctx context.Context, now time.Time) ([]client.Object, error) {
	vmopList := &v1alpha2.VirtualMachineOperationList{}
	err := m.client.List(ctx, vmopList)
	if err != nil {
		return nil, err
	}

	objs := make([]client.Object, 0, len(vmopList.Items))
	for _, vmop := range vmopList.Items {
		objs = append(objs, &vmop)
	}

	result := gc.DefaultFilter(objs, m.ShouldBeDeleted, m.ttl, m.getIndex, m.max, now)

	return result, nil
}

func (m *vmopGCManager) getIndex(obj client.Object) string {
	vmop, ok := obj.(*v1alpha2.VirtualMachineOperation)
	if !ok {
		return ""
	}
	return types.NamespacedName{Namespace: vmop.GetNamespace(), Name: vmop.Spec.VirtualMachine}.String()
}
