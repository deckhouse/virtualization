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

package step

import (
	"context"
	"fmt"

	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/ptr"
	cdiv1 "kubevirt.io/containerized-data-importer-api/pkg/apis/core/v1beta1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/deckhouse/virtualization-controller/pkg/common/object"
	"github.com/deckhouse/virtualization-controller/pkg/controller/conditions"
	"github.com/deckhouse/virtualization-controller/pkg/eventrecord"
	virtv2 "github.com/deckhouse/virtualization/api/core/v1alpha2"
)

type CreateDataVolumeFromVirtualDiskStep struct {
	dv       *cdiv1.DataVolume
	recorder eventrecord.EventRecorderLogger
	disk     CreateDataVolumeStepDisk
	client   client.Client
	cb       *conditions.ConditionBuilder
}

func NewCreateDataVolumeFromVirtualDiskStep(
	dv *cdiv1.DataVolume,
	recorder eventrecord.EventRecorderLogger,
	disk CreateDataVolumeStepDisk,
	client client.Client,
	cb *conditions.ConditionBuilder,
) *CreateDataVolumeFromVirtualDiskStep {
	return &CreateDataVolumeFromVirtualDiskStep{
		dv:       dv,
		recorder: recorder,
		disk:     disk,
		client:   client,
		cb:       cb,
	}
}

func (s CreateDataVolumeFromVirtualDiskStep) Take(ctx context.Context, vi *virtv2.VirtualImage) (*reconcile.Result, error) {
	if s.dv != nil {
		return nil, nil
	}

	vdRefKey := types.NamespacedName{Name: vi.Spec.DataSource.ObjectRef.Name, Namespace: vi.Namespace}
	vdRef, err := object.FetchObject(ctx, vdRefKey, s.client, &virtv2.VirtualDisk{})
	if err != nil {
		return nil, fmt.Errorf("fetch vd %q: %w", vdRefKey, err)
	}

	if vdRef == nil {
		return nil, fmt.Errorf("vd object ref %q is nil", vdRefKey)
	}

	vi.Status.SourceUID = ptr.To(vdRef.UID)

	source := &cdiv1.DataVolumeSource{
		PVC: &cdiv1.DataVolumeSourcePVC{
			Name:      vdRef.Status.Target.PersistentVolumeClaim,
			Namespace: vdRef.Namespace,
		},
	}

	size, err := resource.ParseQuantity(vdRef.Status.Capacity)
	if err != nil {
		return nil, fmt.Errorf("parse quantity: %w", err)
	}

	return NewCreateDataVolumeStep(s.dv, s.recorder, s.disk, source, size, s.cb).Take(ctx, vi)
}
