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
	"log/slog"
	"time"

	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/deckhouse/virtualization-controller/pkg/controller/service"
	"github.com/deckhouse/virtualization-controller/pkg/controller/vm/internal/state"
	"github.com/deckhouse/virtualization-controller/pkg/sdk/framework/helper"
	virtv2 "github.com/deckhouse/virtualization/api/core/v1alpha2"
)

const nameDeletionHandler = "DeletionHandler"

func NewDeletionHandler(client client.Client, logger *slog.Logger) *DeletionHandler {
	return &DeletionHandler{
		client:     client,
		logger:     logger.With("handler", nameDeletionHandler),
		protection: service.NewProtectionService(client, virtv2.FinalizerKVVMProtection),
	}
}

type DeletionHandler struct {
	client     client.Client
	logger     *slog.Logger
	protection *service.ProtectionService
}

func (h *DeletionHandler) Handle(ctx context.Context, s state.VirtualMachineState) (reconcile.Result, error) {
	if s.VirtualMachine().IsEmpty() {
		return reconcile.Result{}, nil
	}
	if s.VirtualMachine().Current().GetDeletionTimestamp().IsZero() {
		changed := s.VirtualMachine().Changed()
		controllerutil.AddFinalizer(changed, virtv2.FinalizerVMCleanup)
		return reconcile.Result{}, nil
	}
	h.logger.Info("Delete VM, remove protective finalizers")
	kvvm, err := s.KVVM(ctx)
	if err != nil {
		return reconcile.Result{}, err
	}
	err = h.protection.RemoveProtection(ctx, kvvm)
	if err != nil {
		return reconcile.Result{}, fmt.Errorf("failed to update finalizer on the KVVM %q: %w", kvvm.GetName(), err)
	}
	if kvvm != nil {
		err = helper.DeleteObject(ctx, h.client, kvvm)
		if err != nil {
			return reconcile.Result{}, err
		}
		requeueAfter := 30 * time.Second
		if p := s.VirtualMachine().Current().Spec.TerminationGracePeriodSeconds; p != nil {
			newRequeueAfter := time.Duration(*p) * time.Second
			if requeueAfter > newRequeueAfter {
				requeueAfter = newRequeueAfter
			}
		}
		return reconcile.Result{RequeueAfter: requeueAfter}, nil
	}

	controllerutil.RemoveFinalizer(s.VirtualMachine().Changed(), virtv2.FinalizerVMCleanup)
	return reconcile.Result{}, nil
}

func (h *DeletionHandler) Name() string {
	return nameDeletionHandler
}
