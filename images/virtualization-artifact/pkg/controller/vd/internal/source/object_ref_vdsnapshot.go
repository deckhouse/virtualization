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
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/deckhouse/virtualization-controller/pkg/common/object"
	"github.com/deckhouse/virtualization-controller/pkg/common/steptaker"
	"github.com/deckhouse/virtualization-controller/pkg/controller/conditions"
	"github.com/deckhouse/virtualization-controller/pkg/controller/vd/internal/source/step"
	vdsupplements "github.com/deckhouse/virtualization-controller/pkg/controller/vd/internal/supplements"
	"github.com/deckhouse/virtualization-controller/pkg/eventrecord"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/vdcondition"
)

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

func (ds ObjectRefVirtualDiskSnapshot) Sync(ctx context.Context, vd *v1alpha2.VirtualDisk) (reconcile.Result, error) {
	if vd.Spec.DataSource == nil || vd.Spec.DataSource.ObjectRef == nil {
		return reconcile.Result{}, errors.New("object ref missed for data source")
	}

	sup := vdsupplements.NewGenerator(vd)

	cb := conditions.NewConditionBuilder(vdcondition.ReadyType).Generation(vd.Generation)
	defer func() { conditions.SetCondition(cb, &vd.Status.Conditions) }()

	// pvc will be nil if the name empty or object is not found
	pvc, err := object.FetchObject(ctx, sup.PersistentVolumeClaim(), ds.client, &corev1.PersistentVolumeClaim{})
	if err != nil {
		return reconcile.Result{}, err
	}

	return steptaker.NewStepTakers[*v1alpha2.VirtualDisk](
		step.NewReadyStep(ds.diskService, pvc, cb),
		step.NewTerminatingStep(pvc),
		step.NewCreatePVCFromVDSnapshotStep(pvc, ds.recorder, ds.client, cb),
		step.NewWaitForPVCStep(pvc, ds.client, cb),
	).Run(ctx, vd)
}

func (ds ObjectRefVirtualDiskSnapshot) Validate(ctx context.Context, vd *v1alpha2.VirtualDisk) error {
	if vd.Spec.DataSource == nil || vd.Spec.DataSource.ObjectRef == nil {
		return errors.New("object ref missed for data source")
	}

	vdSnapshot, err := object.FetchObject(ctx, types.NamespacedName{
		Name:      vd.Spec.DataSource.ObjectRef.Name,
		Namespace: vd.Namespace,
	}, ds.client, &v1alpha2.VirtualDiskSnapshot{})
	if err != nil {
		return err
	}

	if vdSnapshot == nil || vdSnapshot.Status.Phase != v1alpha2.VirtualDiskSnapshotPhaseReady {
		return NewVirtualDiskSnapshotNotReadyError(vd.Spec.DataSource.ObjectRef.Name)
	}

	vs, err := object.FetchObject(ctx, types.NamespacedName{
		Name:      vdSnapshot.Status.VolumeSnapshotName,
		Namespace: vdSnapshot.Namespace,
	}, ds.client, &vsv1.VolumeSnapshot{})
	if err != nil {
		return err
	}

	if vs == nil || vs.Status.ReadyToUse == nil || !*vs.Status.ReadyToUse {
		return NewVirtualDiskSnapshotNotReadyError(vd.Spec.DataSource.ObjectRef.Name)
	}

	return nil
}
