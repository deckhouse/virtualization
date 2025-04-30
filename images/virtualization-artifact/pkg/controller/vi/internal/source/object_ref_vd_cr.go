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

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/deckhouse/virtualization-controller/pkg/common/annotations"
	"github.com/deckhouse/virtualization-controller/pkg/common/blockdevice"
	"github.com/deckhouse/virtualization-controller/pkg/common/object"
	"github.com/deckhouse/virtualization-controller/pkg/controller/conditions"
	"github.com/deckhouse/virtualization-controller/pkg/controller/importer"
	"github.com/deckhouse/virtualization-controller/pkg/controller/supplements"
	"github.com/deckhouse/virtualization-controller/pkg/controller/vi/internal/source/step"
	"github.com/deckhouse/virtualization-controller/pkg/dvcr"
	"github.com/deckhouse/virtualization-controller/pkg/eventrecord"
	virtv2 "github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/vdcondition"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/vicondition"
)

type ObjectRefVirtualDiskCR struct {
	client       client.Client
	importer     Importer
	disk         Disk
	stat         Stat
	dvcrSettings *dvcr.Settings
	recorder     eventrecord.EventRecorderLogger
}

func NewObjectRefVirtualDiskCR(
	client client.Client,
	importer Importer,
	diskService Disk,
	stat Stat,
	dvcrSettings *dvcr.Settings,
	recorder eventrecord.EventRecorderLogger,
) *ObjectRefVirtualDiskCR {
	return &ObjectRefVirtualDiskCR{
		client:       client,
		importer:     importer,
		disk:         diskService,
		stat:         stat,
		dvcrSettings: dvcrSettings,
		recorder:     recorder,
	}
}

func (ds ObjectRefVirtualDiskCR) Sync(ctx context.Context, vi *virtv2.VirtualImage) (reconcile.Result, error) {
	if vi.Spec.DataSource.ObjectRef == nil || vi.Spec.DataSource.ObjectRef.Kind != virtv2.VirtualImageObjectRefKindVirtualDisk {
		return reconcile.Result{}, errors.New("object ref missed for data source")
	}

	supgen := supplements.NewGenerator(annotations.VIShortName, vi.Name, vi.Namespace, vi.UID)

	cb := conditions.NewConditionBuilder(vicondition.ReadyType).Generation(vi.Generation)
	defer func() { conditions.SetCondition(cb, &vi.Status.Conditions) }()

	pod, err := importer.FindPod(ctx, ds.client, supgen)
	if err != nil {
		return reconcile.Result{}, fmt.Errorf("fetch pod: %w", err)
	}

	return blockdevice.NewStepTakers[*virtv2.VirtualImage](
		step.NewReadyContainerRegistryStep(pod, ds.importer, ds.disk, ds.stat, ds.recorder, cb),
		step.NewTerminatingStep(pod),
		step.NewSetSourceUIDStep(ds.GetSourceUID),
		step.NewCreatePodStep(pod, ds.dvcrSettings, ds.recorder, ds.importer, ds.stat, ds.GetPodSettings, cb),
		step.NewWaitForPodStep(pod, ds.stat, cb),
	).Run(ctx, vi)
}

func (ds ObjectRefVirtualDiskCR) GetSourceUID(ctx context.Context, vi *virtv2.VirtualImage) (*types.UID, error) {
	vdRefKey := types.NamespacedName{Name: vi.Spec.DataSource.ObjectRef.Name, Namespace: vi.Namespace}
	vdRef, err := object.FetchObject(ctx, vdRefKey, ds.client, &virtv2.VirtualDisk{})
	if err != nil {
		return nil, fmt.Errorf("fetch vd %q: %w", vdRefKey, err)
	}

	if vdRef == nil {
		return nil, fmt.Errorf("vd object ref %q is nil", vdRefKey)
	}

	return ptr.To(vdRef.UID), nil
}

func (ds ObjectRefVirtualDiskCR) GetPodSettings(ctx context.Context, vi *virtv2.VirtualImage) (*importer.PodSettings, error) {
	vdRefKey := types.NamespacedName{Name: vi.Spec.DataSource.ObjectRef.Name, Namespace: vi.Namespace}
	vdRef, err := object.FetchObject(ctx, vdRefKey, ds.client, &virtv2.VirtualDisk{})
	if err != nil {
		return nil, fmt.Errorf("fetch vd %q: %w", vdRefKey, err)
	}

	if vdRef == nil {
		return nil, fmt.Errorf("vd object ref %q is nil", vdRefKey)
	}

	supgen := supplements.NewGenerator(annotations.VIShortName, vi.Name, vi.Namespace, vi.UID)
	ownerRef := metav1.NewControllerRef(vi, vi.GroupVersionKind())
	return ds.importer.GetPodSettingsWithPVC(ownerRef, supgen, vdRef.Status.Target.PersistentVolumeClaim, vdRef.Namespace), nil
}

func (ds ObjectRefVirtualDiskCR) Validate(ctx context.Context, vi *virtv2.VirtualImage) error {
	return validateVirtualDisk(ctx, vi, ds.client)
}

func validateVirtualDisk(ctx context.Context, vi *virtv2.VirtualImage, client client.Client) error {
	if vi.Spec.DataSource.ObjectRef == nil || vi.Spec.DataSource.ObjectRef.Kind != virtv2.VirtualImageObjectRefKindVirtualDisk {
		return errors.New("object ref missed for data source")
	}

	vd, err := object.FetchObject(ctx, types.NamespacedName{Name: vi.Spec.DataSource.ObjectRef.Name, Namespace: vi.Namespace}, client, &virtv2.VirtualDisk{})
	if err != nil {
		return fmt.Errorf("fetch virtual disk: %w", err)
	}

	if vd == nil || vd.Status.Phase != virtv2.DiskReady {
		return NewVirtualDiskNotReadyError(vi.Spec.DataSource.ObjectRef.Name)
	}

	inUseCondition, _ := conditions.GetCondition(vdcondition.InUseType, vd.Status.Conditions)
	if inUseCondition.Status != metav1.ConditionTrue || inUseCondition.ObservedGeneration != vd.Generation {
		return NewVirtualDiskNotReadyForUseError(vd.Name)
	}

	switch inUseCondition.Reason {
	case vdcondition.UsedForImageCreation.String():
		return nil
	case vdcondition.AttachedToVirtualMachine.String():
		return NewVirtualDiskAttachedToVirtualMachineError(vd.Name)
	default:
		return NewVirtualDiskNotReadyForUseError(vd.Name)
	}
}
