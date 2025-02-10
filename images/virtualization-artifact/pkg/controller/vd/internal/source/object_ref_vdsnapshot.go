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

	vsv1 "github.com/kubernetes-csi/external-snapshotter/client/v6/apis/volumesnapshot/v1"
	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/deckhouse/virtualization-controller/pkg/common/annotations"
	"github.com/deckhouse/virtualization-controller/pkg/common/object"
	"github.com/deckhouse/virtualization-controller/pkg/controller/conditions"
	"github.com/deckhouse/virtualization-controller/pkg/controller/supplements"
	"github.com/deckhouse/virtualization-controller/pkg/controller/vd/internal/source/step"
	"github.com/deckhouse/virtualization-controller/pkg/eventrecord"
	virtv2 "github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/vdcondition"
)

//go:generate moq -rm -out mock.go . ObjectRefVirtualDiskSnapshotDiskService

type ObjectRefVirtualDiskSnapshotDiskService interface {
	step.ReadyStepDiskService

	GetVirtualDiskSnapshot(ctx context.Context, name, namespace string) (*virtv2.VirtualDiskSnapshot, error)
	GetVolumeSnapshot(ctx context.Context, name, namespace string) (*vsv1.VolumeSnapshot, error)
}

type ObjectRefVirtualDiskSnapshot struct {
	diskService ObjectRefVirtualDiskSnapshotDiskService
	recorder    eventrecord.EventRecorderLogger
	client      client.Client
}

func NewObjectRefVirtualDiskSnapshot(recorder eventrecord.EventRecorderLogger, diskService ObjectRefVirtualDiskSnapshotDiskService, client client.Client) *ObjectRefVirtualDiskSnapshot {
	return &ObjectRefVirtualDiskSnapshot{
		diskService: diskService,
		recorder:    recorder,
		client:      client,
	}
}

func (ds ObjectRefVirtualDiskSnapshot) Sync(ctx context.Context, vd *virtv2.VirtualDisk) (reconcile.Result, error) {
	if vd.Spec.DataSource == nil || vd.Spec.DataSource.ObjectRef == nil {
		return reconcile.Result{}, errors.New("object ref missed for data source")
	}

	supgen := supplements.NewGenerator(annotations.VDShortName, vd.Name, vd.Namespace, vd.UID)

	cb := conditions.NewConditionBuilder(vdcondition.ReadyType).Generation(vd.Generation)
	defer func() { conditions.SetCondition(cb, &vd.Status.Conditions) }()

	pvc, err := object.FetchObject(ctx, supgen.PersistentVolumeClaim(), ds.client, &corev1.PersistentVolumeClaim{})
	if err != nil {
		return reconcile.Result{}, err
	}

	return step.NewTakers[*virtv2.VirtualDisk](
		step.NewReadyStep(ds.diskService, pvc, cb),
		step.NewTerminatingStep(pvc),
		step.NewWaitForPVCStep(pvc, ds.client, cb),
		step.NewCreatePVCStep(pvc, ds.recorder, ds.client, cb),
	).Run(ctx, vd)
}

func (ds ObjectRefVirtualDiskSnapshot) Validate(ctx context.Context, vd *virtv2.VirtualDisk) error {
	if vd.Spec.DataSource == nil || vd.Spec.DataSource.ObjectRef == nil {
		return errors.New("object ref missed for data source")
	}

	vdSnapshot, err := ds.diskService.GetVirtualDiskSnapshot(ctx, vd.Spec.DataSource.ObjectRef.Name, vd.Namespace)
	if err != nil {
		return err
	}

	if vdSnapshot == nil || vdSnapshot.Status.Phase != virtv2.VirtualDiskSnapshotPhaseReady {
		return NewVirtualDiskSnapshotNotReadyError(vd.Spec.DataSource.ObjectRef.Name)
	}

	vs, err := ds.diskService.GetVolumeSnapshot(ctx, vdSnapshot.Status.VolumeSnapshotName, vdSnapshot.Namespace)
	if err != nil {
		return err
	}

	if vs == nil || vs.Status.ReadyToUse == nil || !*vs.Status.ReadyToUse {
		return NewVirtualDiskSnapshotNotReadyError(vd.Spec.DataSource.ObjectRef.Name)
	}

	return nil
}
