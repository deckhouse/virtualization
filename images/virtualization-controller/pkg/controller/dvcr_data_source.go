package controller

import (
	"context"
	"fmt"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	virtv2 "github.com/deckhouse/virtualization-controller/api/v1alpha2"
	"github.com/deckhouse/virtualization-controller/pkg/sdk/framework/helper"
)

const FailureReasonCannotBeProcessed = "The resource cannot be processed."

type DVCRDataSource struct {
	size    virtv2.ImageStatusSize
	meta    metav1.Object
	format  string
	isReady bool
}

func NewDVCRDataSourcesForCVMI(ctx context.Context, ds virtv2.CVMIDataSource, client client.Client) (*DVCRDataSource, error) {
	var dsDVCR DVCRDataSource

	switch ds.Type {
	case virtv2.DataSourceTypeVirtualMachineImage:
		vmiName := ds.VirtualMachineImage.Name
		vmiNS := ds.VirtualMachineImage.Namespace
		if vmiName != "" && vmiNS != "" {
			vmi, err := helper.FetchObject(ctx, types.NamespacedName{Name: vmiName, Namespace: vmiNS}, client, &virtv2.VirtualMachineImage{})
			if err != nil {
				return nil, err
			}

			if vmi != nil {
				dsDVCR.size = vmi.Status.Size
				dsDVCR.format = vmi.Status.Format
				dsDVCR.meta = vmi.GetObjectMeta()
				dsDVCR.isReady = vmi.Status.Phase == virtv2.ImageReady
			}
		}
	case virtv2.DataSourceTypeClusterVirtualMachineImage:
		cvmiName := ds.ClusterVirtualMachineImage.Name
		if cvmiName != "" {
			cvmi, err := helper.FetchObject(ctx, types.NamespacedName{Name: cvmiName}, client, &virtv2.ClusterVirtualMachineImage{})
			if err != nil {
				return nil, err
			}

			if cvmi != nil {
				dsDVCR.size = cvmi.Status.Size
				dsDVCR.meta = cvmi.GetObjectMeta()
				dsDVCR.format = cvmi.Status.Format
				dsDVCR.isReady = cvmi.Status.Phase == virtv2.ImageReady
			}
		}
	}

	return &dsDVCR, nil
}

func NewDVCRDataSourcesForVMI(ctx context.Context, ds virtv2.VMIDataSource, obj metav1.Object, client client.Client) (*DVCRDataSource, error) {
	var dsDVCR DVCRDataSource

	switch ds.Type {
	case virtv2.DataSourceTypeVirtualMachineImage:
		vmiName := ds.VirtualMachineImage.Name
		vmiNS := obj.GetNamespace()
		if vmiName != "" && vmiNS != "" {
			vmi, err := helper.FetchObject(ctx, types.NamespacedName{Name: vmiName, Namespace: vmiNS}, client, &virtv2.VirtualMachineImage{})
			if err != nil {
				return nil, err
			}

			if vmi != nil {
				dsDVCR.size = vmi.Status.Size
				dsDVCR.format = vmi.Status.Format
				dsDVCR.meta = vmi.GetObjectMeta()
				dsDVCR.isReady = vmi.Status.Phase == virtv2.ImageReady
			}
		}
	case virtv2.DataSourceTypeClusterVirtualMachineImage:
		cvmiName := ds.ClusterVirtualMachineImage.Name
		if cvmiName != "" {
			cvmi, err := helper.FetchObject(ctx, types.NamespacedName{Name: cvmiName}, client, &virtv2.ClusterVirtualMachineImage{})
			if err != nil {
				return nil, err
			}

			if cvmi != nil {
				dsDVCR.size = cvmi.Status.Size
				dsDVCR.meta = cvmi.GetObjectMeta()
				dsDVCR.format = cvmi.Status.Format
				dsDVCR.isReady = cvmi.Status.Phase == virtv2.ImageReady
			}
		}
	}

	return &dsDVCR, nil
}

func NewDVCRDataSourcesForVMD(ctx context.Context, ds *virtv2.VMDDataSource, obj metav1.Object, client client.Client) (*DVCRDataSource, error) {
	if ds == nil {
		return nil, nil
	}

	var dsDVCR DVCRDataSource

	switch ds.Type {
	case virtv2.DataSourceTypeVirtualMachineImage:
		vmiName := ds.VirtualMachineImage.Name
		vmiNS := obj.GetNamespace()
		if vmiName != "" && vmiNS != "" {
			vmi, err := helper.FetchObject(ctx, types.NamespacedName{Name: vmiName, Namespace: vmiNS}, client, &virtv2.VirtualMachineImage{})
			if err != nil {
				return nil, err
			}

			if vmi != nil {
				// TODO Get size from vmi.status.capacity for Kubernetes vmi.
				dsDVCR.size = vmi.Status.Size
				dsDVCR.format = vmi.Status.Format
				dsDVCR.meta = vmi.GetObjectMeta()
				dsDVCR.isReady = vmi.Status.Phase == virtv2.ImageReady
			}
		}
	case virtv2.DataSourceTypeClusterVirtualMachineImage:
		cvmiName := ds.ClusterVirtualMachineImage.Name
		if cvmiName != "" {
			cvmi, err := helper.FetchObject(ctx, types.NamespacedName{Name: cvmiName}, client, &virtv2.ClusterVirtualMachineImage{})
			if err != nil {
				return nil, err
			}

			if cvmi != nil {
				dsDVCR.size = cvmi.Status.Size
				dsDVCR.meta = cvmi.GetObjectMeta()
				dsDVCR.format = cvmi.Status.Format
				dsDVCR.isReady = cvmi.Status.Phase == virtv2.ImageReady
			}
		}
	}

	return &dsDVCR, nil
}

func (ds *DVCRDataSource) Validate() error {
	if ds.meta == nil {
		return fmt.Errorf("dvcr data source not found")
	}

	return nil
}

func (ds *DVCRDataSource) GetSize() virtv2.ImageStatusSize {
	return ds.size
}

func (ds *DVCRDataSource) GetFormat() string {
	return ds.format
}

func (ds *DVCRDataSource) IsReady() bool {
	return ds.isReady
}
