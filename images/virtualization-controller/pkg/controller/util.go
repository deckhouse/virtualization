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

type DataSourcesFromDVCR struct {
	CVMI *virtv2.ClusterVirtualMachineImage
	VMI  *virtv2.VirtualMachineImage
}

func NewDVCRDataSource(ctx context.Context, ds virtv2.DataSource, obj metav1.Object, client client.Client) (*DataSourcesFromDVCR, error) {
	ns := obj.GetNamespace()
	dsDVCR := &DataSourcesFromDVCR{}
	switch ds.Type {
	case virtv2.DataSourceTypeVirtualMachineImage:
		vmiName := ds.VirtualMachineImage.Name
		vmiNS := ds.VirtualMachineImage.Namespace
		if ns != "" {
			vmiNS = ns
		}
		if vmiName != "" && vmiNS != "" {
			vmi, err := helper.FetchObject[*virtv2.VirtualMachineImage](ctx, types.NamespacedName{Name: vmiName, Namespace: vmiNS}, client, &virtv2.VirtualMachineImage{})
			if err != nil {
				return dsDVCR, err
			}
			dsDVCR.VMI = vmi
		}
	case virtv2.DataSourceTypeClusterVirtualMachineImage:
		cvmiName := ds.ClusterVirtualMachineImage.Name
		if cvmiName != "" {
			cvmi, err := helper.FetchObject[*virtv2.ClusterVirtualMachineImage](ctx, types.NamespacedName{Name: cvmiName}, client, &virtv2.ClusterVirtualMachineImage{})
			if err != nil {
				return dsDVCR, err
			}
			dsDVCR.CVMI = cvmi
		}
	}
	return dsDVCR, nil
}

func VerifyDVCRDataSources(ds virtv2.DataSource, dsDVCR *DataSourcesFromDVCR) error {
	switch ds.Type {
	case virtv2.DataSourceTypeClusterVirtualMachineImage:
		if dsDVCR == nil || dsDVCR.CVMI == nil {
			return fmt.Errorf("cvmi %s not found", ds.ClusterVirtualMachineImage.Name)
		}
	case virtv2.DataSourceTypeVirtualMachineImage:
		if dsDVCR == nil || dsDVCR.VMI == nil {
			return fmt.Errorf("vmi %s/%s not found", ds.VirtualMachineImage.Namespace, ds.VirtualMachineImage.Name)
		}
	}
	return nil
}
