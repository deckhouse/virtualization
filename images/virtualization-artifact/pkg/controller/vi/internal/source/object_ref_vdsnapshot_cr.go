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

	vsv1 "github.com/kubernetes-csi/external-snapshotter/client/v6/apis/volumesnapshot/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/deckhouse/virtualization-controller/pkg/common/annotations"
	"github.com/deckhouse/virtualization-controller/pkg/common/object"
	"github.com/deckhouse/virtualization-controller/pkg/common/steptaker"
	"github.com/deckhouse/virtualization-controller/pkg/controller/conditions"
	"github.com/deckhouse/virtualization-controller/pkg/controller/supplements"
	"github.com/deckhouse/virtualization-controller/pkg/controller/vi/internal/source/step"
	"github.com/deckhouse/virtualization-controller/pkg/dvcr"
	"github.com/deckhouse/virtualization-controller/pkg/eventrecord"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/vicondition"
)

type ObjectRefVirtualDiskSnapshotCR struct {
	importer     Importer
	stat         Stat
	client       client.Client
	dvcrSettings *dvcr.Settings
	diskService  Disk
	recorder     eventrecord.EventRecorderLogger
}

func NewObjectRefVirtualDiskSnapshotCR(
	importer Importer,
	statService Stat,
	diskService Disk,
	client client.Client,
	dvcrSettings *dvcr.Settings,
	recorder eventrecord.EventRecorderLogger,
) *ObjectRefVirtualDiskSnapshotCR {
	return &ObjectRefVirtualDiskSnapshotCR{
		importer:     importer,
		client:       client,
		recorder:     recorder,
		stat:         statService,
		diskService:  diskService,
		dvcrSettings: dvcrSettings,
	}
}

func (ds ObjectRefVirtualDiskSnapshotCR) Sync(ctx context.Context, vi *v1alpha2.VirtualImage) (reconcile.Result, error) {
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

	pod, err := ds.importer.GetPod(ctx, supgen)
	if err != nil {
		return reconcile.Result{}, fmt.Errorf("fetch pod: %w", err)
	}

	return steptaker.NewStepTakers[*v1alpha2.VirtualImage](
		step.NewReadyContainerRegistryStep(pod, ds.importer, ds.diskService, ds.stat, ds.recorder, cb),
		step.NewTerminatingStep(pvc),
		step.NewCreatePersistentVolumeClaimStep(pvc, ds.recorder, ds.client, cb),
		step.NewCreatePodStep(pod, ds.client, ds.dvcrSettings, ds.recorder, ds.importer, ds.stat, cb),
		step.NewWaitForPodStep(pod, pvc, ds.stat, cb),
	).Run(ctx, vi)
}

func (ds ObjectRefVirtualDiskSnapshotCR) Validate(ctx context.Context, vi *v1alpha2.VirtualImage) error {
	return validateVirtualDiskSnapshot(ctx, vi, ds.client)
}

func validateVirtualDiskSnapshot(ctx context.Context, vi *v1alpha2.VirtualImage, client client.Client) error {
	if vi.Spec.DataSource.ObjectRef == nil || vi.Spec.DataSource.ObjectRef.Kind != v1alpha2.VirtualImageObjectRefKindVirtualDiskSnapshot {
		return errors.New("object ref missed for data source")
	}

	vdSnapshot, err := object.FetchObject(ctx, types.NamespacedName{Name: vi.Spec.DataSource.ObjectRef.Name, Namespace: vi.Namespace}, client, &v1alpha2.VirtualDiskSnapshot{})
	if err != nil {
		return fmt.Errorf("fetch virtual disk snapshot: %w", err)
	}

	if vdSnapshot == nil || vdSnapshot.Status.Phase != v1alpha2.VirtualDiskSnapshotPhaseReady {
		return NewVirtualDiskSnapshotNotReadyError(vi.Spec.DataSource.ObjectRef.Name)
	}

	vs, err := object.FetchObject(ctx, types.NamespacedName{Name: vdSnapshot.Status.VolumeSnapshotName, Namespace: vdSnapshot.Namespace}, client, &vsv1.VolumeSnapshot{})
	if err != nil {
		return fmt.Errorf("fetch volume snapshot: %w", err)
	}

	if vs == nil || vs.Status.ReadyToUse == nil || !*vs.Status.ReadyToUse {
		return NewVirtualDiskSnapshotNotReadyError(vi.Spec.DataSource.ObjectRef.Name)
	}

	return nil
}
