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

	"github.com/deckhouse/virtualization-controller/pkg/common/imageformat"
	"github.com/deckhouse/virtualization-controller/pkg/common/object"
	virtv2 "github.com/deckhouse/virtualization/api/core/v1alpha2"
)

type DVCRDataSource struct {
	size    virtv2.ImageStatusSize
	meta    metav1.Object
	uid     types.UID
	format  string
	target  string
	isReady bool
}

func NewDVCRDataSourcesForCVMI(ctx context.Context, ds virtv2.ClusterVirtualImageDataSource, client client.Client) (DVCRDataSource, error) {
	if ds.ObjectRef == nil {
		return DVCRDataSource{}, nil
	}

	var dsDVCR DVCRDataSource

	switch ds.ObjectRef.Kind {
	case virtv2.ClusterVirtualImageObjectRefKindVirtualImage:
		vmiName := ds.ObjectRef.Name
		vmiNS := ds.ObjectRef.Namespace
		if vmiName != "" && vmiNS != "" {
			vmi, err := object.FetchObject(ctx, types.NamespacedName{Name: vmiName, Namespace: vmiNS}, client, &virtv2.VirtualImage{})
			if err != nil {
				return DVCRDataSource{}, err
			}

			if vmi != nil {
				dsDVCR.uid = vmi.UID
				dsDVCR.size = vmi.Status.Size
				dsDVCR.format = vmi.Status.Format
				dsDVCR.meta = vmi.GetObjectMeta()
				dsDVCR.isReady = vmi.Status.Phase == virtv2.ImageReady
				dsDVCR.target = vmi.Status.Target.RegistryURL
			}
		}
	case virtv2.ClusterVirtualImageObjectRefKindClusterVirtualImage:
		cvmiName := ds.ObjectRef.Name
		if cvmiName != "" {
			cvmi, err := object.FetchObject(ctx, types.NamespacedName{Name: cvmiName}, client, &virtv2.ClusterVirtualImage{})
			if err != nil {
				return DVCRDataSource{}, err
			}

			if cvmi != nil {
				dsDVCR.uid = cvmi.UID
				dsDVCR.size = cvmi.Status.Size
				dsDVCR.meta = cvmi.GetObjectMeta()
				dsDVCR.format = cvmi.Status.Format
				dsDVCR.isReady = cvmi.Status.Phase == virtv2.ImageReady
				dsDVCR.target = cvmi.Status.Target.RegistryURL
			}
		}
	}

	return dsDVCR, nil
}

func NewDVCRDataSourcesForVMI(ctx context.Context, ds virtv2.VirtualImageDataSource, obj metav1.Object, client client.Client) (DVCRDataSource, error) {
	if ds.ObjectRef == nil {
		return DVCRDataSource{}, nil
	}

	var dsDVCR DVCRDataSource

	switch ds.ObjectRef.Kind {
	case virtv2.VirtualImageObjectRefKindVirtualImage:
		vmiName := ds.ObjectRef.Name
		vmiNS := obj.GetNamespace()
		if vmiName != "" && vmiNS != "" {
			vmi, err := object.FetchObject(ctx, types.NamespacedName{Name: vmiName, Namespace: vmiNS}, client, &virtv2.VirtualImage{})
			if err != nil {
				return DVCRDataSource{}, err
			}

			if vmi != nil {
				if vmi.Spec.Storage == virtv2.StorageKubernetes || vmi.Spec.Storage == virtv2.StoragePersistentVolumeClaim {
					return DVCRDataSource{}, fmt.Errorf("the DVCR not used for virtual images with storage type '%s'", vmi.Spec.Storage)
				}

				dsDVCR.uid = vmi.UID
				dsDVCR.size = vmi.Status.Size
				dsDVCR.format = vmi.Status.Format
				dsDVCR.meta = vmi.GetObjectMeta()
				dsDVCR.isReady = vmi.Status.Phase == virtv2.ImageReady
				dsDVCR.target = vmi.Status.Target.RegistryURL
			}
		}
	case virtv2.VirtualImageObjectRefKindClusterVirtualImage:
		cvmiName := ds.ObjectRef.Name
		if cvmiName != "" {
			cvmi, err := object.FetchObject(ctx, types.NamespacedName{Name: cvmiName}, client, &virtv2.ClusterVirtualImage{})
			if err != nil {
				return DVCRDataSource{}, err
			}

			if cvmi != nil {
				dsDVCR.uid = cvmi.UID
				dsDVCR.size = cvmi.Status.Size
				dsDVCR.meta = cvmi.GetObjectMeta()
				dsDVCR.format = cvmi.Status.Format
				dsDVCR.isReady = cvmi.Status.Phase == virtv2.ImageReady
				dsDVCR.target = cvmi.Status.Target.RegistryURL
			}
		}
	}

	return dsDVCR, nil
}

func NewDVCRDataSourcesForVMD(ctx context.Context, ds *virtv2.VirtualDiskDataSource, obj metav1.Object, client client.Client) (DVCRDataSource, error) {
	if ds == nil || ds.ObjectRef == nil {
		return DVCRDataSource{}, nil
	}

	var dsDVCR DVCRDataSource

	switch ds.ObjectRef.Kind {
	case virtv2.VirtualDiskObjectRefKindVirtualImage:
		vmiName := ds.ObjectRef.Name
		vmiNS := obj.GetNamespace()
		if vmiName != "" && vmiNS != "" {
			vmi, err := object.FetchObject(ctx, types.NamespacedName{Name: vmiName, Namespace: vmiNS}, client, &virtv2.VirtualImage{})
			if err != nil {
				return DVCRDataSource{}, err
			}

			if vmi != nil {
				dsDVCR.uid = vmi.UID
				dsDVCR.size = vmi.Status.Size
				dsDVCR.format = vmi.Status.Format
				dsDVCR.meta = vmi.GetObjectMeta()
				dsDVCR.isReady = vmi.Status.Phase == virtv2.ImageReady
				dsDVCR.target = vmi.Status.Target.RegistryURL
			}
		}
	case virtv2.VirtualDiskObjectRefKindClusterVirtualImage:
		cvmiName := ds.ObjectRef.Name
		if cvmiName != "" {
			cvmi, err := object.FetchObject(ctx, types.NamespacedName{Name: cvmiName}, client, &virtv2.ClusterVirtualImage{})
			if err != nil {
				return DVCRDataSource{}, err
			}

			if cvmi != nil {
				dsDVCR.uid = cvmi.UID
				dsDVCR.size = cvmi.Status.Size
				dsDVCR.meta = cvmi.GetObjectMeta()
				dsDVCR.format = cvmi.Status.Format
				dsDVCR.isReady = cvmi.Status.Phase == virtv2.ImageReady
				dsDVCR.target = cvmi.Status.Target.RegistryURL
			}
		}
	}

	return dsDVCR, nil
}

func (ds *DVCRDataSource) Validate() error {
	if ds.meta == nil {
		return fmt.Errorf("dvcr data source not found")
	}

	return nil
}

func (ds *DVCRDataSource) GetUID() types.UID {
	return ds.uid
}

func (ds *DVCRDataSource) GetSize() virtv2.ImageStatusSize {
	return ds.size
}

func (ds *DVCRDataSource) IsCDROM() bool {
	return imageformat.IsISO(ds.format)
}

func (ds *DVCRDataSource) GetFormat() string {
	return ds.format
}

func (ds *DVCRDataSource) IsReady() bool {
	return ds.isReady
}

func (ds *DVCRDataSource) GetTarget() string {
	return ds.target
}
