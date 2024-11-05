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

	corev1 "k8s.io/api/core/v1"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/deckhouse/virtualization-controller/pkg/controller/common"
	"github.com/deckhouse/virtualization-controller/pkg/controller/vmclass/internal/state"
	virtv2 "github.com/deckhouse/virtualization/api/core/v1alpha2"
)

const nameDeletionHandler = "DeletionHandler"

func NewDeletionHandler(client client.Client, recorder record.EventRecorder, logger *slog.Logger) *DeletionHandler {
	return &DeletionHandler{
		client:   client,
		recorder: recorder,
		logger:   logger.With("handler", nameDeletionHandler),
	}
}

type DeletionHandler struct {
	client   client.Client
	recorder record.EventRecorder
	logger   *slog.Logger
}

func (h *DeletionHandler) Handle(ctx context.Context, s state.VirtualMachineClassState) (reconcile.Result, error) {
	if s.VirtualMachineClass().IsEmpty() {
		return reconcile.Result{}, nil
	}
	changed := s.VirtualMachineClass().Changed()
	if s.VirtualMachineClass().Current().GetDeletionTimestamp().IsZero() {
		controllerutil.AddFinalizer(changed, virtv2.FinalizerVMCleanup)
		return reconcile.Result{}, nil
	}
	vms, err := s.VirtualMachines(ctx)
	if err != nil {
		return reconcile.Result{}, err
	}
	if len(vms) > 0 {
		msg := fmt.Sprintf("VirtualMachineClass cannot be deleted, there are VMs that use it. %s...", common.NamespacedName(&vms[0]))
		h.logger.Info(msg)
		h.recorder.Event(changed, corev1.EventTypeWarning, virtv2.ReasonVMClassInUse, msg)
		return reconcile.Result{RequeueAfter: 60 * time.Second}, nil
	}
	h.logger.Info("Deletion observed: remove cleanup finalizer from VirtualMachineClass")
	controllerutil.RemoveFinalizer(changed, virtv2.FinalizerVMCleanup)
	return reconcile.Result{}, nil
}

func (h *DeletionHandler) Name() string {
	return nameDeletionHandler
}
