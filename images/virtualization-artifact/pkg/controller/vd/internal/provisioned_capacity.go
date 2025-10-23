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

package internal

import (
	"context"

	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	virtv1 "kubevirt.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	commonvd "github.com/deckhouse/virtualization-controller/pkg/common/vd"
	"github.com/deckhouse/virtualization-controller/pkg/controller/conditions"
	"github.com/deckhouse/virtualization-controller/pkg/controller/kvbuilder"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/vdcondition"
)

const provisionedCapacityHandlerName = "ProvisionedCapacityHandler"

type ProvisionedCapacityHandler struct {
	client client.Client
}

func NewProvisionedCapacityHandler(client client.Client) *ProvisionedCapacityHandler {
	return &ProvisionedCapacityHandler{
		client: client,
	}
}

func (h ProvisionedCapacityHandler) Handle(ctx context.Context, vd *v1alpha2.VirtualDisk) (reconcile.Result, error) {
	ready, _ := conditions.GetCondition(vdcondition.ReadyType, vd.Status.Conditions)
	if ready.Status != metav1.ConditionTrue {
		return reconcile.Result{}, nil
	}

	vmName := commonvd.GetCurrentlyMountedVMName(vd)
	if vmName == "" {
		return reconcile.Result{}, nil
	}

	kvvmi := &virtv1.VirtualMachineInstance{}
	err := h.client.Get(ctx, client.ObjectKey{Namespace: vd.Namespace, Name: vmName}, kvvmi)
	if err != nil {
		return reconcile.Result{}, client.IgnoreNotFound(err)
	}

	volumeName := kvbuilder.GenerateVDDiskName(vd.Name)
	for _, volumeStatus := range kvvmi.Status.VolumeStatus {
		if volumeStatus.Name == volumeName {
			vd.Status.ProvisionedCapacity = resource.NewQuantity(volumeStatus.Size, resource.BinarySI)
			break
		}
	}

	return reconcile.Result{}, nil
}

func (h ProvisionedCapacityHandler) Name() string {
	return provisionedCapacityHandlerName
}
