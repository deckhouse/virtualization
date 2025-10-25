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

	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/deckhouse/virtualization-controller/pkg/common/annotations"
	"github.com/deckhouse/virtualization-controller/pkg/common/steptaker"
	"github.com/deckhouse/virtualization-controller/pkg/controller/conditions"
	"github.com/deckhouse/virtualization-controller/pkg/controller/supplements"
	"github.com/deckhouse/virtualization-controller/pkg/controller/vi/internal/source/step"
	"github.com/deckhouse/virtualization-controller/pkg/dvcr"
	"github.com/deckhouse/virtualization-controller/pkg/eventrecord"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/vicondition"
)

type ObjectRefVirtualDiskSnapshotPVC struct {
	importer     Importer
	stat         Stat
	bounder      Bounder
	client       client.Client
	dvcrSettings *dvcr.Settings
	diskService  Disk
	recorder     eventrecord.EventRecorderLogger
}

func NewObjectRefVirtualDiskSnapshotPVC(
	importer Importer,
	stat Stat,
	bounder Bounder,
	diskService Disk,
	client client.Client,
	dvcrSettings *dvcr.Settings,
	recorder eventrecord.EventRecorderLogger,
) *ObjectRefVirtualDiskSnapshotPVC {
	return &ObjectRefVirtualDiskSnapshotPVC{
		importer:     importer,
		stat:         stat,
		bounder:      bounder,
		client:       client,
		dvcrSettings: dvcrSettings,
		diskService:  diskService,
		recorder:     recorder,
	}
}

func (ds ObjectRefVirtualDiskSnapshotPVC) Sync(ctx context.Context, vi *v1alpha2.VirtualImage) (reconcile.Result, error) {
	if vi.Spec.DataSource.ObjectRef == nil || vi.Spec.DataSource.ObjectRef.Kind != v1alpha2.VirtualImageObjectRefKindVirtualDiskSnapshot {
		return reconcile.Result{}, errors.New("object ref missed for data source")
	}

	supgen := supplements.NewGenerator(annotations.VIShortName, vi.Name, vi.Namespace, vi.UID)

	cb := conditions.NewConditionBuilder(vicondition.ReadyType).Generation(vi.Generation)
	defer func() { conditions.SetCondition(cb, &vi.Status.Conditions) }()

	pvc, err := ds.diskService.GetPersistentVolumeClaim(ctx, supgen)
	if err != nil {
		return reconcile.Result{}, fmt.Errorf("fetch pvc: %w", err)
	}

	return steptaker.NewStepTakers[*v1alpha2.VirtualImage](
		step.NewReadyPersistentVolumeClaimStep(pvc, ds.bounder, ds.recorder, cb),
		step.NewTerminatingStep(pvc),
		step.NewCreatePersistentVolumeClaimStep(pvc, ds.recorder, ds.client, cb),
		step.NewCreateBounderPodStep(pvc, ds.bounder, ds.client, ds.recorder, cb),
		step.NewWaitForPVCStep(pvc, cb),
	).Run(ctx, vi)
}

func (ds ObjectRefVirtualDiskSnapshotPVC) Validate(ctx context.Context, vi *v1alpha2.VirtualImage) error {
	return validateVirtualDiskSnapshot(ctx, vi, ds.client)
}
