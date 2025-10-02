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

package watcher

import (
	"fmt"

	vsv1 "github.com/kubernetes-csi/external-snapshotter/client/v6/apis/volumesnapshot/v1"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/source"

	"github.com/deckhouse/virtualization/api/core/v1alpha2"
)

type VolumeSnapshotWatcher struct{}

func NewVolumeSnapshotWatcher() *VolumeSnapshotWatcher {
	return &VolumeSnapshotWatcher{}
}

func (w VolumeSnapshotWatcher) Watch(mgr manager.Manager, ctr controller.Controller) error {
	if err := ctr.Watch(
		source.Kind(mgr.GetCache(), &vsv1.VolumeSnapshot{},
			handler.TypedEnqueueRequestForOwner[*vsv1.VolumeSnapshot](
				mgr.GetScheme(),
				mgr.GetRESTMapper(),
				&v1alpha2.VirtualDiskSnapshot{},
			),
		),
	); err != nil {
		return fmt.Errorf("error setting watch on VolumeSnapshot: %w", err)
	}
	return nil
}
