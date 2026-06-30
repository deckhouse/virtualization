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
	"fmt"
	"reflect"

	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	"github.com/deckhouse/virtualization-controller/pkg/common/storageclass"
	commonvd "github.com/deckhouse/virtualization-controller/pkg/common/vd"
	"github.com/deckhouse/virtualization-controller/pkg/controller/conditions"
	intsvc "github.com/deckhouse/virtualization-controller/pkg/controller/vd/internal/service"
	"github.com/deckhouse/virtualization-controller/pkg/controller/vd/internal/source"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/vdcondition"
)

// VirtualDiskSnapshotStorageClassValidator forbids creating a VirtualDisk from a
// VirtualDiskSnapshot whose source PVC is backed by a different CSI driver than
// the target storage class. Cross-CSI-driver provisioning is not supported.
type VirtualDiskSnapshotStorageClassValidator struct {
	client    client.Client
	scService *intsvc.VirtualDiskStorageClassService
}

func NewVirtualDiskSnapshotStorageClassValidator(client client.Client, scService *intsvc.VirtualDiskStorageClassService) *VirtualDiskSnapshotStorageClassValidator {
	return &VirtualDiskSnapshotStorageClassValidator{
		client:    client,
		scService: scService,
	}
}

func (v *VirtualDiskSnapshotStorageClassValidator) ValidateCreate(ctx context.Context, vd *v1alpha2.VirtualDisk) (admission.Warnings, error) {
	return nil, v.validate(ctx, vd)
}

func (v *VirtualDiskSnapshotStorageClassValidator) ValidateUpdate(ctx context.Context, oldVD, newVD *v1alpha2.VirtualDisk) (admission.Warnings, error) {
	if reflect.DeepEqual(oldVD.Spec.DataSource, newVD.Spec.DataSource) {
		return nil, nil
	}

	ready, _ := conditions.GetCondition(vdcondition.ReadyType, newVD.Status.Conditions)
	if source.IsDiskProvisioningFinished(ready) {
		return nil, nil
	}

	return nil, v.validate(ctx, newVD)
}

func (v *VirtualDiskSnapshotStorageClassValidator) validate(ctx context.Context, vd *v1alpha2.VirtualDisk) error {
	if vd.Spec.DataSource == nil || vd.Spec.DataSource.Type != v1alpha2.DataSourceTypeObjectRef {
		return nil
	}
	if vd.Spec.DataSource.ObjectRef == nil || vd.Spec.DataSource.ObjectRef.Kind != v1alpha2.VirtualDiskObjectRefKindVirtualDiskSnapshot {
		return nil
	}

	sourceProvisioner, ok, err := storageclass.ProvisionerOfVirtualDiskSnapshot(ctx, v.client, vd.Namespace, vd.Spec.DataSource.ObjectRef.Name)
	if err != nil {
		return err
	}
	if !ok {
		return nil
	}

	scName, err := commonvd.ResolveStorageClassName(ctx, vd, v.scService)
	if err != nil {
		return err
	}

	targetProvisioner, err := storageclass.ProvisionerOf(ctx, v.client, scName)
	if err != nil {
		return err
	}
	if targetProvisioner == "" {
		return nil
	}

	if targetProvisioner != sourceProvisioner {
		return fmt.Errorf(
			"virtual disk storage class %q provisioner %q does not match the source VirtualDiskSnapshot %q provisioner %q: "+
				"creating a disk from a snapshot on a different CSI driver is not supported",
			scName, targetProvisioner, vd.Spec.DataSource.ObjectRef.Name, sourceProvisioner,
		)
	}

	return nil
}
