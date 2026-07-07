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
	"github.com/deckhouse/virtualization-controller/pkg/controller/cvi/internal/source"
	"github.com/deckhouse/virtualization-controller/pkg/controller/service"
	"github.com/deckhouse/virtualization-controller/pkg/logger"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/cvicondition"
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

func (h DeletionHandler) Handle(ctx context.Context, cvi *v1alpha2.ClusterVirtualImage) (reconcile.Result, error) {
	log := logger.FromContext(ctx).With(logger.SlogHandler(deletionHandlerName))

	if cvi.DeletionTimestamp != nil {
		if controllerutil.ContainsFinalizer(cvi, v1alpha2.FinalizerCVIProtection) {
			attachedVMs, err := h.attachedVirtualMachineNames(ctx, cvi)
			if err != nil {
				return reconcile.Result{}, err
			}

			h.setDeletingCondition(
				cvi,
				cvicondition.DeletionBlockedByProtection,
				deletionBlockedByProtectionMessage(attachedVMs),
			)
			return reconcile.Result{}, nil
		}

		requeue, reason, err := h.sources.CleanUp(ctx, cvi)
		if err != nil {
			return reconcile.Result{}, err
		}

		if requeue {
			if reason == "" {
				reason = "Waiting for cleanup to finish"
			}
			h.setDeletingCondition(cvi, cvicondition.DeletionCleanupPending, reason)
			log.Info("ClusterVirtualImage cleanup is pending", slog.String("reason", reason))
			return reconcile.Result{RequeueAfter: time.Second}, nil
		}

		conditions.RemoveCondition(cvicondition.DeletingType, &cvi.Status.Conditions)
		log.Info("Deletion observed: remove cleanup finalizer from ClusterVirtualImage")
		controllerutil.RemoveFinalizer(cvi, v1alpha2.FinalizerCVICleanup)
		return reconcile.Result{}, nil
	}

	conditions.RemoveCondition(cvicondition.DeletingType, &cvi.Status.Conditions)
	controllerutil.AddFinalizer(cvi, v1alpha2.FinalizerCVICleanup)
	return reconcile.Result{}, nil
}

func (h DeletionHandler) setDeletingCondition(cvi *v1alpha2.ClusterVirtualImage, reason cvicondition.DeletingReason, message string) {
	conditions.SetCondition(
		conditions.NewConditionBuilder(cvicondition.DeletingType).
			Generation(cvi.Generation).
			Status(metav1.ConditionFalse).
			Reason(reason).
			Message(service.CapitalizeFirstLetter(message)+"."),
		&cvi.Status.Conditions,
	)
}

func (h DeletionHandler) attachedVirtualMachineNames(ctx context.Context, cvi *v1alpha2.ClusterVirtualImage) ([]string, error) {
	var vms v1alpha2.VirtualMachineList
	err := h.client.List(ctx, &vms, &client.ListOptions{})
	if err != nil {
		return nil, err
	}

	var attachedVMs []string
	for _, vm := range vms.Items {
		_, mounted, err := commonvm.BlockDeviceUsage(ctx, h.client, vm, v1alpha2.ClusterImageDevice, cvi.GetName())
		if err != nil {
			return nil, err
		}

		if mounted {
			attachedVMs = append(attachedVMs, vm.Namespace+"/"+vm.Name)
		}
	}

	sort.Strings(attachedVMs)
	return attachedVMs, nil
}

func deletionBlockedByProtectionMessage(attachedVMs []string) string {
	switch len(attachedVMs) {
	case 0:
		return "The ClusterVirtualImage is protected from deletion by the protection finalizer"
	case 1:
		return "The ClusterVirtualImage is protected from deletion because it is attached to VirtualMachine " + attachedVMs[0]
	default:
		return "The ClusterVirtualImage is protected from deletion because it is attached to VirtualMachines: " + strings.Join(attachedVMs, ", ")
	}
}
