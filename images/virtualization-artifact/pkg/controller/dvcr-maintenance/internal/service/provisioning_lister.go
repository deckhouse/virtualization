package service

import (
	"context"
	"fmt"

	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/deckhouse/virtualization-controller/pkg/controller/conditions"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/cvicondition"
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
	result := make([]client.Object, 0)

	clusterImages, err := p.ListClusterVirtualImagesInProvisioning(ctx)
	if err != nil {
		return nil, err
	}
	for i := range clusterImages {
		result = append(result, &clusterImages[i])
	}

	images, err := p.ListVirtualImagesInProvisioning(ctx)
	if err != nil {
		return nil, err
	}
	for i := range images {
		result = append(result, &images[i])
	}

	disks, err := p.ListVirtualDisksInProvisioning(ctx)
	if err != nil {
		return nil, err
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

	provisioning := make([]v1alpha2.ClusterVirtualImage, 0)

	for _, cvi := range cviList.Items {
		cond, ok := conditions.GetCondition(cvicondition.Provisioning, cvi.Status.Conditions)
		if !ok {
			continue
		}
		if cond.Status == "True" {
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

	provisioning := make([]v1alpha2.VirtualImage, 0)

	for _, vi := range viList.Items {
		cond, ok := conditions.GetCondition(cvicondition.Provisioning, vi.Status.Conditions)
		if !ok {
			continue
		}
		if cond.Status == "True" {
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

	provisioning := make([]v1alpha2.VirtualDisk, 0)

	for _, vd := range vdList.Items {
		// Ignore disks without "import to dvcr first" stage.
		if !vdHasDVCRStage(&vd) {
			continue
		}
		cond, ok := conditions.GetCondition(cvicondition.Provisioning, vd.Status.Conditions)
		if !ok {
			continue
		}
		if cond.Status == "True" {
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
