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

package source

import (
	"context"
	"fmt"

	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/deckhouse/virtualization-controller/pkg/controller/service"
	vdsupplements "github.com/deckhouse/virtualization-controller/pkg/controller/vd/internal/supplements"
	"github.com/deckhouse/virtualization-controller/pkg/eventrecord"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
)

const objectRefDataSource = "objectref"

type ObjectRefDataSource struct {
	diskService      *service.DiskService
	vdSnapshotSyncer *ObjectRefVirtualDiskSnapshot
	viSyncer         *ObjectRefVirtualImage
	cviSyncer        *ObjectRefClusterVirtualImage
}

func NewObjectRefDataSource(
	recorder eventrecord.EventRecorderLogger,
	diskService *service.DiskService,
	client client.Client,
) *ObjectRefDataSource {
	return &ObjectRefDataSource{
		diskService:      diskService,
		vdSnapshotSyncer: NewObjectRefVirtualDiskSnapshot(recorder, diskService, client),
		viSyncer:         NewObjectRefVirtualImage(diskService, client),
		cviSyncer:        NewObjectRefClusterVirtualImage(diskService, client),
	}
}

func (ds ObjectRefDataSource) Sync(ctx context.Context, vd *v1alpha2.VirtualDisk) (reconcile.Result, error) {
	if vd.Spec.DataSource == nil || vd.Spec.DataSource.ObjectRef == nil {
		return reconcile.Result{}, fmt.Errorf("not object ref data source, please report a bug")
	}

	switch vd.Spec.DataSource.ObjectRef.Kind {
	case v1alpha2.VirtualDiskObjectRefKindVirtualDiskSnapshot:
		return ds.vdSnapshotSyncer.Sync(ctx, vd)
	case v1alpha2.VirtualDiskObjectRefKindClusterVirtualImage:
		return ds.cviSyncer.Sync(ctx, vd)
	case v1alpha2.VirtualImageKind:
		return ds.viSyncer.Sync(ctx, vd)
	}

	return reconcile.Result{}, fmt.Errorf("unexpected object ref kind %s, please report a bug", vd.Spec.DataSource.ObjectRef.Kind)
}

func (ds ObjectRefDataSource) CleanUp(ctx context.Context, vd *v1alpha2.VirtualDisk) (bool, error) {
	supgen := vdsupplements.NewGenerator(vd)

	requeue, err := ds.diskService.CleanUp(ctx, supgen)
	if err != nil {
		return false, err
	}

	return requeue, nil
}

func (ds ObjectRefDataSource) Validate(ctx context.Context, vd *v1alpha2.VirtualDisk) error {
	if vd.Spec.DataSource == nil || vd.Spec.DataSource.ObjectRef == nil {
		return fmt.Errorf("not object ref data source, please report a bug")
	}

	switch vd.Spec.DataSource.ObjectRef.Kind {
	case v1alpha2.VirtualDiskObjectRefKindVirtualDiskSnapshot:
		return ds.vdSnapshotSyncer.Validate(ctx, vd)
	case v1alpha2.VirtualDiskObjectRefKindClusterVirtualImage:
		return ds.cviSyncer.Validate(ctx, vd)
	case v1alpha2.VirtualImageKind:
		return ds.viSyncer.Validate(ctx, vd)
	}

	return fmt.Errorf("unexpected object ref kind %s, please report a bug", vd.Spec.DataSource.ObjectRef.Kind)
}

func (ds ObjectRefDataSource) Name() string {
	return objectRefDataSource
}
