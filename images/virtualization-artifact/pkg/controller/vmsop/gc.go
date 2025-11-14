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

package vmsop

import (
	"context"
	"time"

	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/manager"

	"github.com/deckhouse/deckhouse/pkg/log"
	commonvmsop "github.com/deckhouse/virtualization-controller/pkg/common/vmsop"
	"github.com/deckhouse/virtualization-controller/pkg/config"
	"github.com/deckhouse/virtualization-controller/pkg/controller/gc"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
)

const gcControllerName = "vmsop-gc-controller"

func SetupGC(mgr manager.Manager, log *log.Logger, gcSettings config.BaseGcSettings) error {
	vmsopGCMgr := newVMSOPGCManager(mgr.GetClient(), gcSettings.TTL.Duration, 10)

	return gc.SetupGcController(gcControllerName,
		mgr,
		log.With("resource", "vmsop"),
		gcSettings.Schedule,
		vmsopGCMgr,
	)
}

func newVMSOPGCManager(client client.Client, ttl time.Duration, max int) *vmsopGCManager {
	if ttl == 0 {
		ttl = 24 * time.Hour
	}
	if max == 0 {
		max = 10
	}
	return &vmsopGCManager{
		client: client,
		ttl:    ttl,
		max:    max,
	}
}

var _ gc.ReconcileGCManager = &vmsopGCManager{}

type vmsopGCManager struct {
	client client.Client
	ttl    time.Duration
	max    int
}

func (m *vmsopGCManager) New() client.Object {
	return &v1alpha2.VirtualMachineSnapshotOperation{}
}

func (m *vmsopGCManager) ShouldBeDeleted(obj client.Object) bool {
	vmsop, ok := obj.(*v1alpha2.VirtualMachineSnapshotOperation)
	if !ok {
		return false
	}
	return commonvmsop.IsFinished(vmsop)
}

func (m *vmsopGCManager) ListForDelete(ctx context.Context, now time.Time) ([]client.Object, error) {
	vmsopList := &v1alpha2.VirtualMachineSnapshotOperationList{}
	err := m.client.List(ctx, vmsopList)
	if err != nil {
		return nil, err
	}

	objs := make([]client.Object, 0, len(vmsopList.Items))
	for _, vmsop := range vmsopList.Items {
		objs = append(objs, &vmsop)
	}

	result := gc.DefaultFilter(objs, m.ShouldBeDeleted, m.ttl, m.getIndex, m.max, now)

	return result, nil
}

func (m *vmsopGCManager) getIndex(obj client.Object) string {
	vmsop, ok := obj.(*v1alpha2.VirtualMachineSnapshotOperation)
	if !ok {
		return ""
	}
	return vmsop.Spec.VirtualMachineSnapshotName
}
