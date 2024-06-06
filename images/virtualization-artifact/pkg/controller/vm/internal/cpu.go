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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/deckhouse/virtualization-controller/pkg/controller/conditions"
	"github.com/deckhouse/virtualization-controller/pkg/controller/service"
	"github.com/deckhouse/virtualization-controller/pkg/controller/vm/internal/state"
	virtv2 "github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/vmcondition"
)

const nameCpuHandler = "CPUHandler"

func NewCPUHandler(client client.Client, recorder record.EventRecorder, logger *slog.Logger) *CPUHandler {
	service.NewProtectionService(client, virtv2.FinalizerVMCPUProtection)
	return &CPUHandler{
		client:     client,
		recorder:   recorder,
		logger:     logger.With("handler", nameCpuHandler),
		protection: service.NewProtectionService(client, virtv2.FinalizerVMCPUProtection),
	}
}

type CPUHandler struct {
	client     client.Client
	recorder   record.EventRecorder
	logger     *slog.Logger
	protection *service.ProtectionService
}

func (h *CPUHandler) Handle(ctx context.Context, s state.VirtualMachineState) (reconcile.Result, error) {
	if s.VirtualMachine().IsEmpty() {
		return reconcile.Result{}, nil
	}
	current := s.VirtualMachine().Current()
	changed := s.VirtualMachine().Changed()

	if update := addAllUnknown(changed, string(vmcondition.TypeCPUModelReady)); update {
		return reconcile.Result{Requeue: true}, nil
	}

	model, err := s.CPUModel(ctx)
	if err != nil {
		return reconcile.Result{}, err
	}

	if isDeletion(current) {
		return reconcile.Result{}, h.protection.RemoveProtection(ctx, model)
	}
	err = h.protection.AddProtection(ctx, model)
	if err != nil {
		return reconcile.Result{}, err
	}

	mgr := conditions.NewManager(changed.Status.Conditions)
	cb := conditions.NewConditionBuilder2(vmcondition.TypeCPUModelReady).
		Generation(current.GetGeneration())

	if model != nil && model.Status.Phase == virtv2.VMCPUPhaseReady {
		mgr.Update(cb.Reason2(vmcondition.ReasonCPUModelReady).Status(metav1.ConditionTrue).Condition())
		changed.Status.Conditions = mgr.Generate()
		return reconcile.Result{}, nil
	}
	msg := fmt.Sprintf("VirtualMachineCPUModelName %q is not ready", namespacedName(model).String())
	reason := vmcondition.ReasonCPUModelNotReady
	if model == nil {
		msg = fmt.Sprintf("VirtualMachineCPUModelName %q not found", namespacedName(model).String())
		h.recorder.Event(changed, corev1.EventTypeWarning, reason.String(), "CPU model not available: waiting for the CPU model")
		h.logger.Error("CPU model not available: waiting for the CPU model")
	}
	mgr.Update(cb.Status(metav1.ConditionFalse).
		Message(msg).
		Reason2(reason).
		Condition())
	changed.Status.Conditions = mgr.Generate()
	return reconcile.Result{RequeueAfter: 2 * time.Second}, nil
}

func (h *CPUHandler) Name() string {
	return nameCpuHandler
}
