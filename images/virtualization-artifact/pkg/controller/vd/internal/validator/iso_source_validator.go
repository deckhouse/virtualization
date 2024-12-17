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

package validator

import (
	"context"

	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	"github.com/deckhouse/virtualization-controller/pkg/common/imageformat"
	"github.com/deckhouse/virtualization-controller/pkg/controller"
	"github.com/deckhouse/virtualization-controller/pkg/controller/service"
	"github.com/deckhouse/virtualization-controller/pkg/controller/vd/internal/source"
	virtv2 "github.com/deckhouse/virtualization/api/core/v1alpha2"
)

type ISOSourceValidator struct {
	client client.Client
}

func NewISOSourceValidator(client client.Client) *ISOSourceValidator {
	return &ISOSourceValidator{client: client}
}

func (v *ISOSourceValidator) ValidateCreate(ctx context.Context, vd *virtv2.VirtualDisk) (admission.Warnings, error) {
	if vd.Spec.DataSource == nil {
		return nil, nil
	}

	if vd.Spec.DataSource.Type != virtv2.DataSourceTypeObjectRef || vd.Spec.DataSource.ObjectRef == nil {
		return nil, nil
	}

	switch vd.Spec.DataSource.ObjectRef.Kind {
	case virtv2.VirtualDiskObjectRefKindVirtualImage,
		virtv2.VirtualDiskObjectRefKindClusterVirtualImage:
		dvcrDataSource, err := controller.NewDVCRDataSourcesForVMD(ctx, vd.Spec.DataSource, vd, v.client)
		if err != nil {
			return nil, err
		}

		if !dvcrDataSource.IsReady() {
			return nil, nil
		}

		if imageformat.IsISO(dvcrDataSource.GetFormat()) {
			return admission.Warnings{
				service.CapitalizeFirstLetter(source.ErrISOSourceNotSupported.Error()),
			}, nil
		}
	}

	return nil, nil
}

func (v *ISOSourceValidator) ValidateUpdate(ctx context.Context, _, newVD *virtv2.VirtualDisk) (admission.Warnings, error) {
	if newVD.Spec.DataSource == nil {
		return nil, nil
	}

	if newVD.Spec.DataSource.Type != virtv2.DataSourceTypeObjectRef || newVD.Spec.DataSource.ObjectRef == nil {
		return nil, nil
	}

	switch newVD.Spec.DataSource.ObjectRef.Kind {
	case virtv2.VirtualDiskObjectRefKindVirtualImage,
		virtv2.VirtualDiskObjectRefKindClusterVirtualImage:
		dvcrDataSource, err := controller.NewDVCRDataSourcesForVMD(ctx, newVD.Spec.DataSource, newVD, v.client)
		if err != nil {
			return nil, err
		}

		if !dvcrDataSource.IsReady() {
			return nil, nil
		}

		if imageformat.IsISO(dvcrDataSource.GetFormat()) {
			return admission.Warnings{
				service.CapitalizeFirstLetter(source.ErrISOSourceNotSupported.Error()),
			}, nil
		}
	}

	return nil, nil
}
