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
	"fmt"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"

	"github.com/deckhouse/deckhouse/pkg/log"
	"github.com/deckhouse/virtualization-controller/pkg/controller/supplements"
	vmrestore "github.com/deckhouse/virtualization-controller/pkg/controller/vmrestore/internal"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
)

type VirtualMachineBlockDeviceAttachmentWatcher struct {
	client   client.Client
	restorer vmrestore.Restorer
}

func NewVirtualMachineBlockDeviceAttachmentWatcher(client client.Client, restorer vmrestore.Restorer) *VirtualMachineBlockDeviceAttachmentWatcher {
	return &VirtualMachineBlockDeviceAttachmentWatcher{
		client:   client,
		restorer: restorer,
	}
}

func (w VirtualMachineBlockDeviceAttachmentWatcher) Watch(mgr manager.Manager, ctr controller.Controller) error {
	if err := ctr.Watch(
		source.Kind(mgr.GetCache(), &v1alpha2.VirtualMachineBlockDeviceAttachment{},
			handler.TypedEnqueueRequestsFromMapFunc(w.enqueueRequests),
		),
	); err != nil {
		return fmt.Errorf("error setting watch on VirtualMachineBlockDeviceAttachment: %w", err)
	}
	return nil
}

func (w VirtualMachineBlockDeviceAttachmentWatcher) enqueueRequests(ctx context.Context, vmbda *v1alpha2.VirtualMachineBlockDeviceAttachment) (requests []reconcile.Request) {
	var vmRestores v1alpha2.VirtualMachineRestoreList
	err := w.client.List(ctx, &vmRestores, &client.ListOptions{
		Namespace: vmbda.GetNamespace(),
	})
	if err != nil {
		log.Error(fmt.Sprintf("failed to list vmRestores: %s", err))
		return
	}

	for _, vmRestore := range vmRestores.Items {
		vmSnapshotName := vmRestore.Spec.VirtualMachineSnapshotName
		var vmSnapshot v1alpha2.VirtualMachineSnapshot
		err := w.client.Get(ctx, types.NamespacedName{Name: vmSnapshotName, Namespace: vmbda.GetNamespace()}, &vmSnapshot)
		if err != nil {
			log.Error(fmt.Sprintf("failed to get vmSnapshot: %s", err))
			return
		}

		if vmSnapshot.Status.VirtualMachineSnapshotSecretName == "" {
			continue
		}

		supGen := supplements.NewGenerator("vms", vmSnapshot.Name, vmSnapshot.Namespace, vmSnapshot.UID)
		restorerSecret, err := supplements.FetchSupplement(ctx, w.client, supGen, supplements.SupplementSnapshot, &corev1.Secret{})
		if err != nil {
			log.Error(fmt.Sprintf("failed to get virtualMachineSnapshotSecret: %s", err))
			return
		}

		if restorerSecret == nil {
			log.Error(fmt.Sprintf("virtualMachineSnapshotSecret %q not found", vmSnapshot.Status.VirtualMachineSnapshotSecretName))
			return
		}

		vmbdas, err := w.restorer.RestoreVirtualMachineBlockDeviceAttachments(ctx, restorerSecret)
		if err != nil {
			log.Error(fmt.Sprintf("failed to extract vmbda resources from the virtualMachineSnapshotSecret: %s", err))
			return
		}

		for _, eVmbda := range vmbdas {
			if w.isVmbdaNameMatch(vmbda.Name, eVmbda.Name, vmRestore.Spec.NameReplacements) {
				requests = append(requests, reconcile.Request{
					NamespacedName: types.NamespacedName{
						Name:      vmRestore.Name,
						Namespace: vmRestore.Namespace,
					},
				})
			}
		}
	}

	return
}

func (w VirtualMachineBlockDeviceAttachmentWatcher) isVmbdaNameMatch(vmbdaName, restoredName string, nameReplacements []v1alpha2.NameReplacement) bool {
	var (
		isNameMatch            bool
		isNameReplacementMatch bool
	)

	isNameMatch = vmbdaName == restoredName

	for _, nr := range nameReplacements {
		if nr.From.Kind != v1alpha2.VirtualMachineBlockDeviceAttachmentKind {
			continue
		}

		if nr.From.Name == vmbdaName {
			isNameReplacementMatch = true
			break
		}
	}

	return isNameMatch || isNameReplacementMatch
}
