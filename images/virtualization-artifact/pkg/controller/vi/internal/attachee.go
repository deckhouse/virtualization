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

package internal

import (
	"context"
	"fmt"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	commonvm "github.com/deckhouse/virtualization-controller/pkg/common/vm"
	"github.com/deckhouse/virtualization-controller/pkg/controller/conditions"
	"github.com/deckhouse/virtualization-controller/pkg/controller/service"
	"github.com/deckhouse/virtualization-controller/pkg/logger"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/vicondition"
)

type AttacheeHandler struct {
	client client.Client
}

func NewAttacheeHandler(client client.Client) *AttacheeHandler {
	return &AttacheeHandler{
		client: client,
	}
}

func (h AttacheeHandler) Handle(ctx context.Context, vi *v1alpha2.VirtualImage) (reconcile.Result, error) {
	log := logger.FromContext(ctx).With(logger.SlogHandler("attachee"))

	attachedVMs, err := commonvm.MountedVirtualMachineNames(ctx, h.client, v1alpha2.ImageDevice, vi.GetName(), vi.GetNamespace(), false)
	if err != nil {
		return reconcile.Result{}, fmt.Errorf("error getting virtual machines: %w", err)
	}

	h.setInUseCondition(vi, attachedVMs)

	switch {
	case len(attachedVMs) == 0:
		log.Debug("Allow virtual image deletion")
		controllerutil.RemoveFinalizer(vi, v1alpha2.FinalizerVIProtection)
	case vi.DeletionTimestamp == nil:
		log.Debug("Protect virtual image from deletion")
		controllerutil.AddFinalizer(vi, v1alpha2.FinalizerVIProtection)
	default:
		log.Debug("Virtual image deletion is delayed: it's protected by virtual machines")
	}

	return reconcile.Result{}, nil
}

func (h AttacheeHandler) Name() string {
	return "AttacheeHandler"
}

// setInUseCondition reports the VirtualMachines that mount the image. An image in use is
// protected from deletion by the protection finalizer, so this condition is also the answer
// to "why does the image stay in Terminating".
func (h AttacheeHandler) setInUseCondition(vi *v1alpha2.VirtualImage, attachedVMs []string) {
	cb := conditions.NewConditionBuilder(vicondition.InUseType).Generation(vi.Generation)

	if len(attachedVMs) > 0 {
		cb.
			Status(metav1.ConditionTrue).
			Reason(vicondition.AttachedToVirtualMachine).
			Message(service.InUseByVirtualMachinesMessage("VirtualImage", attachedVMs))
	} else {
		cb.
			Status(metav1.ConditionFalse).
			Reason(vicondition.NotInUse).
			Message("")
	}

	conditions.SetCondition(cb, &vi.Status.Conditions)
}
