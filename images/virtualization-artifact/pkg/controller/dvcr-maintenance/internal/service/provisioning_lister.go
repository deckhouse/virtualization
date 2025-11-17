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

package service

import (
	"context"
	"fmt"

	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/deckhouse/virtualization-controller/pkg/controller/conditions"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/cvicondition"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/vdcondition"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/vicondition"
)

type ProvisioningLister struct {
	client client.Client
}

func NewProvisioningLister(client client.Client) *ProvisioningLister {
	return &ProvisioningLister{
		client: client,
	}
}

func (p *ProvisioningLister) ListAllInProvisioning(ctx context.Context) ([]client.Object, error) {
	clusterImages, err := p.ListClusterVirtualImagesInProvisioning(ctx)
	if err != nil {
		return nil, err
	}

	images, err := p.ListVirtualImagesInProvisioning(ctx)
	if err != nil {
		return nil, err
	}

	disks, err := p.ListVirtualDisksInProvisioning(ctx)
	if err != nil {
		return nil, err
	}

	result := make([]client.Object, 0, len(clusterImages)+len(images)+len(disks))

	for i := range clusterImages {
		result = append(result, &clusterImages[i])
	}
	for i := range images {
		result = append(result, &images[i])
	}
	for i := range disks {
		result = append(result, &disks[i])
	}

	return result, nil
}

func (p *ProvisioningLister) ListClusterVirtualImagesInProvisioning(ctx context.Context) ([]v1alpha2.ClusterVirtualImage, error) {
	var cviList v1alpha2.ClusterVirtualImageList
	err := p.client.List(ctx, &cviList)
	if err != nil {
		return nil, fmt.Errorf("list all ClusterVirtualImages: %w", err)
	}

	var provisioning []v1alpha2.ClusterVirtualImage
	for _, cvi := range cviList.Items {
		cond, exists := conditions.GetCondition(cvicondition.ReadyType, cvi.Status.Conditions)
		if exists && cond.Status == "False" && cond.Reason == cvicondition.Provisioning.String() {
			provisioning = append(provisioning, cvi)
		}
	}
	return provisioning, nil
}

func (p *ProvisioningLister) ListVirtualImagesInProvisioning(ctx context.Context) ([]v1alpha2.VirtualImage, error) {
	var viList v1alpha2.VirtualImageList
	err := p.client.List(ctx, &viList)
	if err != nil {
		return nil, fmt.Errorf("list all VirtualImages: %w", err)
	}

	var provisioning []v1alpha2.VirtualImage
	for _, vi := range viList.Items {
		cond, exists := conditions.GetCondition(vicondition.ReadyType, vi.Status.Conditions)
		if exists && cond.Status == "False" && cond.Reason == vicondition.Provisioning.String() {
			provisioning = append(provisioning, vi)
		}
	}
	return provisioning, nil
}

func (p *ProvisioningLister) ListVirtualDisksInProvisioning(ctx context.Context) ([]v1alpha2.VirtualDisk, error) {
	var vdList v1alpha2.VirtualDiskList
	err := p.client.List(ctx, &vdList)
	if err != nil {
		return nil, fmt.Errorf("list all VirtualDiks: %w", err)
	}

	var provisioning []v1alpha2.VirtualDisk
	for _, vd := range vdList.Items {
		// Ignore disks without "import to dvcr first" stage.
		if !vdHasDVCRStage(&vd) {
			continue
		}
		cond, exists := conditions.GetCondition(vdcondition.ReadyType, vd.Status.Conditions)
		if exists && cond.Status == "False" && cond.Reason == vdcondition.Provisioning.String() {
			provisioning = append(provisioning, vd)
		}
	}
	return provisioning, nil
}

// vdHasDVCRStage returns true if ClusterVirtualImage, VirtualImage or VirtualDosk
// upload images into DVCR.
func vdHasDVCRStage(vd *v1alpha2.VirtualDisk) bool {
	if vd == nil || vd.Spec.DataSource == nil {
		return false
	}
	switch vd.Spec.DataSource.Type {
	case v1alpha2.DataSourceTypeHTTP,
		v1alpha2.DataSourceTypeContainerImage,
		v1alpha2.DataSourceTypeUpload:
		return true
	}
	if vd.Spec.DataSource.ObjectRef == nil {
		return false
	}
	switch vd.Spec.DataSource.ObjectRef.Kind {
	case v1alpha2.VirtualDiskObjectRefKindVirtualImage,
		v1alpha2.VirtualDiskObjectRefKindClusterVirtualImage:
		return true
	}
	return false
}
