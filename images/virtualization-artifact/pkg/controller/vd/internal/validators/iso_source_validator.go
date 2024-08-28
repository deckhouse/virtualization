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

package validators

import (
	"context"
	"errors"
	"strings"

	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	"github.com/deckhouse/virtualization-controller/pkg/controller"
	"github.com/deckhouse/virtualization-controller/pkg/controller/vd/internal/source"
	"github.com/deckhouse/virtualization-controller/pkg/imageformat"
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

	switch vd.Spec.DataSource.Type {
	case virtv2.DataSourceTypeObjectRef:
		if vd.Spec.DataSource.ObjectRef == nil {
			return nil, errors.New("data source object ref is omitted, but expected")
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
				return nil, source.ErrISOSourceNotSupported
			}
		}
	case virtv2.DataSourceTypeHTTP:
		if vd.Spec.DataSource.HTTP == nil {
			return nil, errors.New("data source http is omitted, but expected")
		}

		if strings.HasSuffix(strings.ToLower(vd.Spec.DataSource.HTTP.URL), imageformat.FormatISO) {
			return nil, source.ErrISOSourceNotSupported
		}
	}

	return nil, nil
}

func (v *ISOSourceValidator) ValidateUpdate(ctx context.Context, _, newVD *virtv2.VirtualDisk) (admission.Warnings, error) {
	if newVD.Spec.DataSource == nil {
		return nil, nil
	}

	switch newVD.Spec.DataSource.Type {
	case virtv2.DataSourceTypeObjectRef:
		if newVD.Spec.DataSource.ObjectRef == nil {
			return nil, errors.New("data source object ref is omitted, but expected")
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
				return nil, source.ErrISOSourceNotSupported
			}
		}

		return nil, nil
	case virtv2.DataSourceTypeHTTP:
		if newVD.Spec.DataSource.HTTP == nil {
			return nil, errors.New("data source http is omitted, but expected")
		}

		if strings.HasSuffix(strings.ToLower(newVD.Spec.DataSource.HTTP.URL), imageformat.FormatISO) {
			return nil, source.ErrISOSourceNotSupported
		}

		return nil, nil
	default:
		return nil, nil
	}
}
