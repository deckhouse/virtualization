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
	"log/slog"
	"sort"
	"strings"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	commonvm "github.com/deckhouse/virtualization-controller/pkg/common/vm"
	"github.com/deckhouse/virtualization-controller/pkg/controller/conditions"
	"github.com/deckhouse/virtualization-controller/pkg/controller/service"
	"github.com/deckhouse/virtualization-controller/pkg/controller/vi/internal/source"
	"github.com/deckhouse/virtualization-controller/pkg/logger"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/vicondition"
)

const deletionHandlerName = "DeletionHandler"

type DeletionHandler struct {
	sources *source.Sources
	client  client.Client
}

func NewDeletionHandler(sources *source.Sources, client client.Client) *DeletionHandler {
	return &DeletionHandler{
		sources: sources,
		client:  client,
	}
}

func (h DeletionHandler) Handle(ctx context.Context, vi *v1alpha2.VirtualImage) (reconcile.Result, error) {
	log := logger.FromContext(ctx).With(logger.SlogHandler(deletionHandlerName))

	if vi.DeletionTimestamp != nil {
		if controllerutil.ContainsFinalizer(vi, v1alpha2.FinalizerVIProtection) {
			attachedVMs, err := h.attachedVirtualMachineNames(ctx, vi)
			if err != nil {
				return reconcile.Result{}, err
			}

			h.setDeletingCondition(
				vi,
				vicondition.DeletionBlockedByProtection,
				deletionBlockedByProtectionMessage(vi, attachedVMs),
			)
			return reconcile.Result{}, nil
		}

		requeue, reason, err := h.sources.CleanUp(ctx, vi)
		if err != nil {
			return reconcile.Result{}, err
		}

		if requeue {
			if reason == "" {
				reason = "Waiting for cleanup to finish"
			}
			h.setDeletingCondition(vi, vicondition.DeletionCleanupPending, reason)
			log.Info("VirtualImage cleanup is pending", slog.String("reason", reason))
			return reconcile.Result{RequeueAfter: time.Second}, nil
		}

		conditions.RemoveCondition(vicondition.DeletingType, &vi.Status.Conditions)
		log.Info("Deletion observed: remove cleanup finalizer from VirtualImage")
		controllerutil.RemoveFinalizer(vi, v1alpha2.FinalizerVICleanup)
		return reconcile.Result{}, nil
	}

	conditions.RemoveCondition(vicondition.DeletingType, &vi.Status.Conditions)
	controllerutil.AddFinalizer(vi, v1alpha2.FinalizerVICleanup)
	return reconcile.Result{}, nil
}

func (h DeletionHandler) setDeletingCondition(vi *v1alpha2.VirtualImage, reason vicondition.DeletingReason, message string) {
	conditions.SetCondition(
		conditions.NewConditionBuilder(vicondition.DeletingType).
			Generation(vi.Generation).
			Status(metav1.ConditionFalse).
			Reason(reason).
			Message(service.CapitalizeFirstLetter(message)+"."),
		&vi.Status.Conditions,
	)
}

func (h DeletionHandler) attachedVirtualMachineNames(ctx context.Context, vi *v1alpha2.VirtualImage) ([]string, error) {
	var vms v1alpha2.VirtualMachineList
	err := h.client.List(ctx, &vms, &client.ListOptions{Namespace: vi.GetNamespace()})
	if err != nil {
		return nil, err
	}

	var attachedVMs []string
	for _, vm := range vms.Items {
		_, mounted, err := commonvm.BlockDeviceUsage(ctx, h.client, vm, v1alpha2.ImageDevice, vi.GetName())
		if err != nil {
			return nil, err
		}

		if mounted {
			attachedVMs = append(attachedVMs, vm.Name)
		}
	}

	sort.Strings(attachedVMs)
	return attachedVMs, nil
}

func deletionBlockedByProtectionMessage(vi *v1alpha2.VirtualImage, attachedVMs []string) string {
	switch len(attachedVMs) {
	case 0:
		return "The VirtualImage is protected from deletion by the protection finalizer"
	case 1:
		return "The VirtualImage is protected from deletion because it is attached to VirtualMachine " + attachedVMs[0]
	default:
		return "The VirtualImage is protected from deletion because it is attached to VirtualMachines: " + strings.Join(attachedVMs, ", ")
	}
}

func (h DeletionHandler) Name() string {
	return "DeletionHandler"
}
