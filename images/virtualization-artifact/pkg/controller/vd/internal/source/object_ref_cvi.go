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
	"errors"
	"fmt"

	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/deckhouse/virtualization-controller/pkg/common/object"
	"github.com/deckhouse/virtualization-controller/pkg/common/steptaker"
	"github.com/deckhouse/virtualization-controller/pkg/controller/conditions"
	"github.com/deckhouse/virtualization-controller/pkg/controller/vd/internal/source/step"
	vdsupplements "github.com/deckhouse/virtualization-controller/pkg/controller/vd/internal/supplements"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/vdcondition"
)

type ObjectRefClusterVirtualImage struct {
	diskService ObjectRefClusterVirtualImageDiskService
	client      client.Client
}

func NewObjectRefClusterVirtualImage(
	diskService ObjectRefClusterVirtualImageDiskService,
	client client.Client,
) *ObjectRefClusterVirtualImage {
	return &ObjectRefClusterVirtualImage{
		diskService: diskService,
		client:      client,
	}
}

func (ds ObjectRefClusterVirtualImage) Sync(ctx context.Context, vd *v1alpha2.VirtualDisk) (reconcile.Result, error) {
	if vd.Spec.DataSource == nil || vd.Spec.DataSource.ObjectRef == nil {
		return reconcile.Result{}, errors.New("object ref missed for data source")
	}

	supgen := vdsupplements.NewGenerator(vd)

	cb := conditions.NewConditionBuilder(vdcondition.ReadyType).Generation(vd.Generation)
	defer func() { conditions.SetCondition(cb, &vd.Status.Conditions) }()

	pvc, err := ds.diskService.GetPersistentVolumeClaim(ctx, supgen.Generator)
	if err != nil {
		return reconcile.Result{}, fmt.Errorf("fetch pvc: %w", err)
	}

	dv, err := ds.diskService.GetDataVolume(ctx, supgen.Generator)
	if err != nil {
		return reconcile.Result{}, fmt.Errorf("fetch dv: %w", err)
	}

	return steptaker.NewStepTakers[*v1alpha2.VirtualDisk](
		step.NewReadyStep(ds.diskService, pvc, cb),
		step.NewTerminatingStep(pvc),
		step.NewCreateDataVolumeFromClusterVirtualImageStep(pvc, dv, ds.diskService, ds.client, cb),
		step.NewEnsureNodePlacementStep(pvc, dv, ds.diskService, ds.client, cb),
		step.NewWaitForDVStep(pvc, dv, ds.diskService, ds.client, cb),
		step.NewWaitForPVCStep(pvc, ds.client, cb),
	).Run(ctx, vd)
}

func (ds ObjectRefClusterVirtualImage) Validate(ctx context.Context, vd *v1alpha2.VirtualDisk) error {
	if vd.Spec.DataSource == nil || vd.Spec.DataSource.ObjectRef == nil {
		return errors.New("object ref missed for data source")
	}

	cviRefKey := types.NamespacedName{Name: vd.Spec.DataSource.ObjectRef.Name}
	cviRef, err := object.FetchObject(ctx, cviRefKey, ds.client, &v1alpha2.ClusterVirtualImage{})
	if err != nil {
		return fmt.Errorf("fetch vi %q: %w", cviRefKey, err)
	}

	if cviRef == nil {
		return NewClusterImageNotFoundError(vd.Spec.DataSource.ObjectRef.Name)
	}

	if cviRef.Status.Phase != v1alpha2.ImageReady || cviRef.Status.Target.RegistryURL == "" {
		return NewClusterImageNotReadyError(vd.Spec.DataSource.ObjectRef.Name)
	}

	return nil
}
