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

package migration

import (
	"context"

	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/deckhouse/virtualization/api/core/v1alpha2"
)

type diskCache struct {
	CVINameUID map[string]types.UID
	VINameUID  map[types.NamespacedName]types.UID
	VDNameUID  map[types.NamespacedName]types.UID
}

func newDiskCache(ctx context.Context, c client.Client) (diskCache, error) {
	cviList := &v1alpha2.ClusterVirtualImageList{}
	if err := c.List(ctx, cviList, &client.ListOptions{}); err != nil {
		return diskCache{}, err
	}
	cviNameUIDMap := make(map[string]types.UID, len(cviList.Items))
	for i := range cviList.Items {
		cviNameUIDMap[cviList.Items[i].Name] = cviList.Items[i].UID
	}

	viList := &v1alpha2.VirtualImageList{}
	if err := c.List(ctx, viList, &client.ListOptions{}); err != nil {
		return diskCache{}, err
	}
	viNameUIDMap := make(map[types.NamespacedName]types.UID, len(viList.Items))
	for i := range viList.Items {
		viNameUIDMap[types.NamespacedName{
			Namespace: viList.Items[i].Namespace,
			Name:      viList.Items[i].Name,
		}] = viList.Items[i].UID
	}

	vdList := &v1alpha2.VirtualDiskList{}
	if err := c.List(ctx, vdList, &client.ListOptions{}); err != nil {
		return diskCache{}, err
	}
	vdNameUIDMap := make(map[types.NamespacedName]types.UID, len(vdList.Items))
	for i := range vdList.Items {
		vdNameUIDMap[types.NamespacedName{
			Namespace: vdList.Items[i].Namespace,
			Name:      vdList.Items[i].Name,
		}] = vdList.Items[i].UID
	}

	return diskCache{
		CVINameUID: cviNameUIDMap,
		VINameUID:  viNameUIDMap,
		VDNameUID:  vdNameUIDMap,
	}, nil
}
