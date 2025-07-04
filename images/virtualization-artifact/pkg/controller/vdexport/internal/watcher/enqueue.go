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

package watcher

import (
	"context"

	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/deckhouse/virtualization/api/core/v1alpha2"
)

func GetEnqueueForKind(c client.Client, kind v1alpha2.VirtualDataExportTargetKind) handler.MapFunc {
	return func(ctx context.Context, obj client.Object) []reconcile.Request {
		switch kind {
		case v1alpha2.VirtualDataExportTargetVirtualDisk:
			_, ok := obj.(*v1alpha2.VirtualDisk)
			if !ok {
				return nil
			}
		case v1alpha2.VirtualDataExportTargetVirtualDiskSnapshot:
			_, ok := obj.(*v1alpha2.VirtualDiskSnapshot)
			if !ok {
				return nil
			}
		case v1alpha2.VirtualDataExportTargetVirtualImage:
			_, ok := obj.(*v1alpha2.VirtualImage)
			if !ok {
				return nil
			}
		case v1alpha2.VirtualDataExportTargetClusterVirtualImage:
			_, ok := obj.(*v1alpha2.ClusterVirtualImage)
			if !ok {
				return nil
			}
		default:
			return nil
		}

		vdexports := &v1alpha2.VirtualDataExportList{}
		err := c.List(ctx, vdexports)
		if err != nil {
			return nil
		}

		var requests []reconcile.Request
		for _, vdexport := range vdexports.Items {
			if vdexport.Spec.TargetRef.Kind == kind && vdexport.Spec.TargetRef.Name == obj.GetName() {
				requests = append(requests, reconcile.Request{
					NamespacedName: types.NamespacedName{
						Name:      vdexport.Name,
						Namespace: vdexport.Namespace,
					},
				})
			}
		}
		return requests

	}
}
