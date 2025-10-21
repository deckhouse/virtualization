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

	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/deckhouse/virtualization-controller/pkg/controller/supplements"
	"github.com/deckhouse/virtualization-controller/pkg/controller/vd/internal/source/step"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
)

//go:generate go tool moq -rm -out mock.go . Handler BlankDataSourceDiskService ObjectRefVirtualImageDiskService ObjectRefClusterVirtualImageDiskService ObjectRefVirtualDiskSnapshotDiskService

type Handler interface {
	Name() string
	Sync(ctx context.Context, vd *v1alpha2.VirtualDisk) (reconcile.Result, error)
	CleanUp(ctx context.Context, vd *v1alpha2.VirtualDisk) (bool, error)
	Validate(ctx context.Context, vd *v1alpha2.VirtualDisk) error
}

type BlankDataSourceDiskService interface {
	step.VolumeAndAccessModesGetter
	step.ReadyStepDiskService

	CleanUp(ctx context.Context, sup supplements.Generator) (bool, error)
}

type ObjectRefVirtualImageDiskService interface {
	step.ReadyStepDiskService
	step.WaitForDVStepDiskService
	step.CreateDataVolumeStepDiskService
	step.EnsureNodePlacementStepDiskService
}

type ObjectRefClusterVirtualImageDiskService interface {
	step.ReadyStepDiskService
	step.WaitForDVStepDiskService
	step.CreateDataVolumeStepDiskService
	step.EnsureNodePlacementStepDiskService
}

type ObjectRefVirtualDiskSnapshotDiskService interface {
	step.ReadyStepDiskService
}
