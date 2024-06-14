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

package controller

import (
	"context"
	"fmt"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/deckhouse/virtualization-controller/pkg/sdk/framework/helper"
	virtv2 "github.com/deckhouse/virtualization/api/core/v1alpha2"
)

const FailureReasonCannotBeProcessed = "The resource cannot be processed."

type DVCRDataSource struct {
	size    virtv2.ImageStatusSize
	meta    metav1.Object
	format  string
	isReady bool
}

func NewDVCRDataSourcesForCVMI(ctx context.Context, ds virtv2.ClusterVirtualImageDataSource, client client.Client) (*DVCRDataSource, error) {
	if ds.ObjectRef == nil {
		return nil, nil
	}

	var dsDVCR DVCRDataSource

	switch ds.ObjectRef.Kind {
	case virtv2.ClusterVirtualImageObjectRefKindVirtualImage:
		vmiName := ds.ObjectRef.Name
		vmiNS := ds.ObjectRef.Namespace
		if vmiName != "" && vmiNS != "" {
			vmi, err := helper.FetchObject(ctx, types.NamespacedName{Name: vmiName, Namespace: vmiNS}, client, &virtv2.VirtualImage{})
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
	case virtv2.ClusterVirtualImageObjectRefKindClusterVirtualImage:
		cvmiName := ds.ObjectRef.Name
		if cvmiName != "" {
			cvmi, err := helper.FetchObject(ctx, types.NamespacedName{Name: cvmiName}, client, &virtv2.ClusterVirtualImage{})
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

func NewDVCRDataSourcesForVMI(ctx context.Context, ds virtv2.VirtualImageDataSource, obj metav1.Object, client client.Client) (*DVCRDataSource, error) {
	if ds.ObjectRef == nil {
		return nil, nil
	}

	var dsDVCR DVCRDataSource

	switch ds.ObjectRef.Kind {
	case virtv2.VirtualImageObjectRefKindVirtualImage:
		vmiName := ds.ObjectRef.Name
		vmiNS := obj.GetNamespace()
		if vmiName != "" && vmiNS != "" {
			vmi, err := helper.FetchObject(ctx, types.NamespacedName{Name: vmiName, Namespace: vmiNS}, client, &virtv2.VirtualImage{})
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
	case virtv2.VirtualImageObjectRefKindClusterVirtualImage:
		cvmiName := ds.ObjectRef.Name
		if cvmiName != "" {
			cvmi, err := helper.FetchObject(ctx, types.NamespacedName{Name: cvmiName}, client, &virtv2.ClusterVirtualImage{})
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

func NewDVCRDataSourcesForVMD(ctx context.Context, ds *virtv2.VirtualDiskDataSource, obj metav1.Object, client client.Client) (*DVCRDataSource, error) {
	if ds == nil || ds.ObjectRef == nil {
		return nil, nil
	}

	var dsDVCR DVCRDataSource

	switch ds.ObjectRef.Kind {
	case virtv2.VirtualDiskObjectRefKindVirtualImage:
		vmiName := ds.ObjectRef.Name
		vmiNS := obj.GetNamespace()
		if vmiName != "" && vmiNS != "" {
			vmi, err := helper.FetchObject(ctx, types.NamespacedName{Name: vmiName, Namespace: vmiNS}, client, &virtv2.VirtualImage{})
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
	case virtv2.VirtualDiskObjectRefKindClusterVirtualImage:
		cvmiName := ds.ObjectRef.Name
		if cvmiName != "" {
			cvmi, err := helper.FetchObject(ctx, types.NamespacedName{Name: cvmiName}, client, &virtv2.ClusterVirtualImage{})
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
