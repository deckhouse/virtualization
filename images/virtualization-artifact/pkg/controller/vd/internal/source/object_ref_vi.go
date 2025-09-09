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

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	cdiv1 "kubevirt.io/containerized-data-importer-api/pkg/apis/core/v1beta1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/deckhouse/virtualization-controller/pkg/common/annotations"
	"github.com/deckhouse/virtualization-controller/pkg/common/object"
	"github.com/deckhouse/virtualization-controller/pkg/common/steptaker"
	"github.com/deckhouse/virtualization-controller/pkg/controller/conditions"
	"github.com/deckhouse/virtualization-controller/pkg/controller/supplements"
	"github.com/deckhouse/virtualization-controller/pkg/controller/vd/internal/source/step"
	virtv2 "github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/vdcondition"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/vicondition"
)

type ObjectRefVirtualImage struct {
	diskService ObjectRefVirtualImageDiskService
	client      client.Client
}

func NewObjectRefVirtualImage(
	diskService ObjectRefVirtualImageDiskService,
	client client.Client,
) *ObjectRefVirtualImage {
	return &ObjectRefVirtualImage{
		diskService: diskService,
		client:      client,
	}
}

func (ds ObjectRefVirtualImage) Sync(ctx context.Context, vd *virtv2.VirtualDisk) (reconcile.Result, error) {
	if vd.Spec.DataSource == nil || vd.Spec.DataSource.ObjectRef == nil {
		return reconcile.Result{}, errors.New("object ref missed for data source")
	}

	supgen := supplements.NewGenerator(annotations.VDShortName, vd.Name, vd.Namespace, vd.UID)

	cb := conditions.NewConditionBuilder(vdcondition.ReadyType).Generation(vd.Generation)
	defer func() { conditions.SetCondition(cb, &vd.Status.Conditions) }()

	pvc, err := object.FetchObject(ctx, supgen.PersistentVolumeClaim(), ds.client, &corev1.PersistentVolumeClaim{})
	if err != nil {
		return reconcile.Result{}, fmt.Errorf("fetch pvc: %w", err)
	}

	dv, err := object.FetchObject(ctx, supgen.DataVolume(), ds.client, &cdiv1.DataVolume{})
	if err != nil {
		return reconcile.Result{}, fmt.Errorf("fetch dv: %w", err)
	}

	return steptaker.NewStepTakers[*virtv2.VirtualDisk](
		step.NewReadyStep(ds.diskService, pvc, cb),
		step.NewTerminatingStep(pvc),
		step.NewCreateDataVolumeFromVirtualImageStep(pvc, dv, ds.diskService, ds.client, cb),
		step.NewEnsureNodePlacementStep(pvc, dv, ds.diskService, ds.client, cb),
		step.NewWaitForDVStep(pvc, dv, ds.diskService, ds.client, cb),
		step.NewWaitForPVCStep(pvc, ds.client, cb),
	).Run(ctx, vd)
}

func (ds ObjectRefVirtualImage) Validate(ctx context.Context, vd *virtv2.VirtualDisk) error {
	if vd.Spec.DataSource == nil || vd.Spec.DataSource.ObjectRef == nil {
		return errors.New("object ref missed for data source")
	}

	viRefKey := types.NamespacedName{Name: vd.Spec.DataSource.ObjectRef.Name, Namespace: vd.Namespace}
	viRef, err := object.FetchObject(ctx, viRefKey, ds.client, &virtv2.VirtualImage{})
	if err != nil {
		return fmt.Errorf("fetch vi %q: %w", viRefKey, err)
	}

	if viRef == nil {
		return NewImageNotReadyError(vd.Spec.DataSource.ObjectRef.Name)
	}

	viRefReady, _ := conditions.GetCondition(vicondition.ReadyType, viRef.Status.Conditions)
	if viRefReady.Status != metav1.ConditionTrue {
		return NewImageNotReadyError(vd.Spec.DataSource.ObjectRef.Name)
	}

	switch viRef.Spec.Storage {
	case virtv2.StoragePersistentVolumeClaim, virtv2.StorageKubernetes:
		if viRef.Status.Target.PersistentVolumeClaim == "" {
			return NewImageNotReadyError(vd.Spec.DataSource.ObjectRef.Name)
		}
	case virtv2.StorageContainerRegistry, "":
		if viRef.Status.Target.RegistryURL == "" {
			return NewImageNotReadyError(vd.Spec.DataSource.ObjectRef.Name)
		}
	default:
		return fmt.Errorf("unexpected virtual image storage %s, please report a bug", viRef.Spec.Storage)
	}

	return nil
}
