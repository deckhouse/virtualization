package controller

import (
	"context"
	"fmt"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	virtv2 "github.com/deckhouse/virtualization-controller/api/v2alpha1"
	"github.com/deckhouse/virtualization-controller/pkg/sdk/framework/helper"
)

const FailureReasonCannotBeProcessed = "The resource cannot be processed."

type DVCRDataSource struct {
	format string
	meta   metav1.Object
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

			dsDVCR.meta = vmi
			dsDVCR.format = vmi.Status.Format
		}
	case virtv2.DataSourceTypeClusterVirtualMachineImage:
		cvmiName := ds.ClusterVirtualMachineImage.Name
		if cvmiName != "" {
			cvmi, err := helper.FetchObject(ctx, types.NamespacedName{Name: cvmiName}, client, &virtv2.ClusterVirtualMachineImage{})
			if err != nil {
				return nil, err
			}

			dsDVCR.meta = cvmi
			dsDVCR.format = cvmi.Status.Format
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

			dsDVCR.meta = vmi
			dsDVCR.format = vmi.Status.Format
		}
	case virtv2.DataSourceTypeClusterVirtualMachineImage:
		cvmiName := ds.ClusterVirtualMachineImage.Name
		if cvmiName != "" {
			cvmi, err := helper.FetchObject(ctx, types.NamespacedName{Name: cvmiName}, client, &virtv2.ClusterVirtualMachineImage{})
			if err != nil {
				return nil, err
			}

			dsDVCR.meta = cvmi
			dsDVCR.format = cvmi.Status.Format
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

func (ds *DVCRDataSource) GetFormat() string {
	return ds.format
}
