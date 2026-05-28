/*
Copyright 2026 Flant JSC

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

package validator

import (
	"context"
	"reflect"

	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	commonvd "github.com/deckhouse/virtualization-controller/pkg/common/vd"
	"github.com/deckhouse/virtualization-controller/pkg/controller/conditions"
	intsvc "github.com/deckhouse/virtualization-controller/pkg/controller/vd/internal/service"
	"github.com/deckhouse/virtualization-controller/pkg/controller/vd/internal/source"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/vdcondition"
)

type VirtualImagePVCStorageClassValidator struct {
	client    client.Client
	scService *intsvc.VirtualDiskStorageClassService
}

func NewVirtualImagePVCStorageClassValidator(client client.Client, scService *intsvc.VirtualDiskStorageClassService) *VirtualImagePVCStorageClassValidator {
	return &VirtualImagePVCStorageClassValidator{
		client:    client,
		scService: scService,
	}
}

func (v *VirtualImagePVCStorageClassValidator) ValidateCreate(ctx context.Context, vd *v1alpha2.VirtualDisk) (admission.Warnings, error) {
	scName, err := commonvd.ResolveStorageClassName(ctx, vd, v.scService)
	if err != nil {
		return nil, err
	}

	vdWithStatusStorageClassName := vd.DeepCopy()
	vdWithStatusStorageClassName.Status.StorageClassName = scName

	return nil, commonvd.ValidateVirtualImageStorageClassProvisionerCompatibility(ctx, vdWithStatusStorageClassName, v.client)
}

func (v *VirtualImagePVCStorageClassValidator) ValidateUpdate(ctx context.Context, oldVD, newVD *v1alpha2.VirtualDisk) (admission.Warnings, error) {
	if reflect.DeepEqual(oldVD.Spec.DataSource, newVD.Spec.DataSource) {
		return nil, nil
	}

	ready, _ := conditions.GetCondition(vdcondition.ReadyType, newVD.Status.Conditions)
	if source.IsDiskProvisioningFinished(ready) {
		return nil, nil
	}

	return nil, commonvd.ValidateVirtualImageStorageClassProvisionerCompatibility(ctx, newVD, v.client)
}
