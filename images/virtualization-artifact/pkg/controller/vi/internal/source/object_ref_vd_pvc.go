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
	"errors"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	cdiv1 "kubevirt.io/containerized-data-importer-api/pkg/apis/core/v1beta1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/deckhouse/virtualization-controller/pkg/common/annotations"
	"github.com/deckhouse/virtualization-controller/pkg/common/blockdevice"
	"github.com/deckhouse/virtualization-controller/pkg/common/object"
	"github.com/deckhouse/virtualization-controller/pkg/controller/conditions"
	"github.com/deckhouse/virtualization-controller/pkg/controller/supplements"
	"github.com/deckhouse/virtualization-controller/pkg/controller/vi/internal/source/step"
	"github.com/deckhouse/virtualization-controller/pkg/eventrecord"
	virtv2 "github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/vicondition"
)

type ObjectRefVirtualDiskPVC struct {
	bounder  Bounder
	client   client.Client
	disk     Disk
	recorder eventrecord.EventRecorderLogger
}

func NewObjectRefVirtualDiskPVC(
	bounder Bounder,
	client client.Client,
	disk Disk,
	recorder eventrecord.EventRecorderLogger,
) *ObjectRefVirtualDiskPVC {
	return &ObjectRefVirtualDiskPVC{
		bounder:  bounder,
		client:   client,
		disk:     disk,
		recorder: recorder,
	}
}

func (ds ObjectRefVirtualDiskPVC) Sync(ctx context.Context, vi *virtv2.VirtualImage) (reconcile.Result, error) {
	if vi.Spec.DataSource.ObjectRef == nil || vi.Spec.DataSource.ObjectRef.Kind != virtv2.VirtualImageObjectRefKindVirtualDisk {
		return reconcile.Result{}, errors.New("object ref missed for data source")
	}

	supgen := supplements.NewGenerator(annotations.VIShortName, vi.Name, vi.Namespace, vi.UID)

	cb := conditions.NewConditionBuilder(vicondition.ReadyType).Generation(vi.Generation)
	defer func() { conditions.SetCondition(cb, &vi.Status.Conditions) }()

	pvc, err := object.FetchObject(ctx, supgen.PersistentVolumeClaim(), ds.client, &corev1.PersistentVolumeClaim{})
	if err != nil {
		return reconcile.Result{}, fmt.Errorf("fetch pvc: %w", err)
	}

	dv, err := object.FetchObject(ctx, supgen.DataVolume(), ds.client, &cdiv1.DataVolume{})
	if err != nil {
		return reconcile.Result{}, fmt.Errorf("fetch dv: %w", err)
	}

	return blockdevice.NewStepTakers[*virtv2.VirtualImage](
		step.NewReadyPersistentVolumeClaimStep(pvc, ds.bounder, ds.recorder, cb),
		step.NewTerminatingStep(pvc, dv),
		step.NewCreateDataVolumeFromVirtualDiskStep(dv, ds.recorder, ds.disk, ds.client, cb),
		step.NewWaitForDVStep(pvc, dv, ds.disk, ds.client, cb),
		step.NewWaitForPVCStep(pvc, cb),
	).Run(ctx, vi)
}

func (ds ObjectRefVirtualDiskPVC) Validate(ctx context.Context, vi *virtv2.VirtualImage) error {
	return validateVirtualDisk(ctx, vi, ds.client)
}
