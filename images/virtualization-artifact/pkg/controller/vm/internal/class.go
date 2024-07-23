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
	"github.com/deckhouse/virtualization-controller/pkg/controller/vm/internal/state"
	virtv2 "github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/vmcondition"
)

const nameClassHandler = "ClassHandler"

func NewClassHandler(client client.Client, recorder record.EventRecorder, logger *slog.Logger) *ClassHandler {
	return &ClassHandler{
		client:   client,
		recorder: recorder,
		logger:   logger.With("handler", nameClassHandler),
	}
}

type ClassHandler struct {
	client   client.Client
	recorder record.EventRecorder
	logger   *slog.Logger
}

func (h *ClassHandler) Handle(ctx context.Context, s state.VirtualMachineState) (reconcile.Result, error) {
	if s.VirtualMachine().IsEmpty() {
		return reconcile.Result{}, nil
	}
	current := s.VirtualMachine().Current()
	changed := s.VirtualMachine().Changed()

	if update := addAllUnknown(changed, string(vmcondition.TypeClassReady)); update {
		return reconcile.Result{Requeue: true}, nil
	}

	if isDeletion(current) {
		return reconcile.Result{}, nil
	}

	class, err := s.Class(ctx)
	if err != nil {
		return reconcile.Result{}, err
	}

	mgr := conditions.NewManager(changed.Status.Conditions)
	cb := conditions.NewConditionBuilder2(vmcondition.TypeClassReady).
		Generation(current.GetGeneration())

	if class != nil && class.Status.Phase == virtv2.ClassPhaseReady {
		if (class.Spec.CPU.Type == virtv2.CPUTypeDiscovery || class.Spec.CPU.Type == virtv2.CPUTypeFeatures) && len(class.Status.CpuFeatures.Enabled) == 0 {
			mgr.Update(cb.
				Message("No enabled processor features found").
				Reason2(vmcondition.ReasonClassNotReady).
				Status(metav1.ConditionFalse).
				Condition())
			changed.Status.Conditions = mgr.Generate()
			return reconcile.Result{RequeueAfter: 2 * time.Second}, nil
		}
		mgr.Update(cb.Reason2(vmcondition.ReasonClassReady).Status(metav1.ConditionTrue).Condition())
		changed.Status.Conditions = mgr.Generate()
		return reconcile.Result{}, nil
	}
	className := current.Spec.VirtualMachineClassName
	msg := fmt.Sprintf("VirtualMachineClassName %q is not ready", className)
	reason := vmcondition.ReasonClassNotReady
	if class == nil {
		msg = fmt.Sprintf("VirtualMachineClassName %q not found", className)
		h.recorder.Event(changed, corev1.EventTypeWarning, reason.String(), "VirtualMachineClass not available: waiting for the VirtualMachineClass")
		h.logger.Error("VirtualMachineClass not available: waiting for the CPU model")
	}
	mgr.Update(cb.Status(metav1.ConditionFalse).
		Message(msg).
		Reason2(reason).
		Condition())
	changed.Status.Conditions = mgr.Generate()
	return reconcile.Result{RequeueAfter: 2 * time.Second}, nil
}

func (h *ClassHandler) Name() string {
	return nameClassHandler
}
