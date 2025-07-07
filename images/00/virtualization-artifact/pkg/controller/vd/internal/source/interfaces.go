/*
Copyright 2025 Flant JSC

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

package source

import (
	"context"

	vsv1 "github.com/kubernetes-csi/external-snapshotter/client/v6/apis/volumesnapshot/v1"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/deckhouse/virtualization-controller/pkg/controller/supplements"
	"github.com/deckhouse/virtualization-controller/pkg/controller/vd/internal/source/step"
	virtv2 "github.com/deckhouse/virtualization/api/core/v1alpha2"
)

//go:generate moq -rm -out mock.go . Handler BlankDataSourceDiskService ObjectRefVirtualDiskSnapshotDiskService

type Handler interface {
	Name() string
	Sync(ctx context.Context, vd *virtv2.VirtualDisk) (reconcile.Result, error)
	CleanUp(ctx context.Context, vd *virtv2.VirtualDisk) (bool, error)
	Validate(ctx context.Context, vd *virtv2.VirtualDisk) error
}

type BlankDataSourceDiskService interface {
	step.VolumeAndAccessModesGetter
	step.ReadyStepDiskService

	CleanUp(ctx context.Context, sup *supplements.Generator) (bool, error)
}

type ObjectRefVirtualDiskSnapshotDiskService interface {
	step.ReadyStepDiskService

	GetVirtualDiskSnapshot(ctx context.Context, name, namespace string) (*virtv2.VirtualDiskSnapshot, error)
	GetVolumeSnapshot(ctx context.Context, name, namespace string) (*vsv1.VolumeSnapshot, error)
}
