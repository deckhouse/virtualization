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
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/deckhouse/virtualization-controller/pkg/controller/conditions"
	"github.com/deckhouse/virtualization-controller/pkg/controller/vm/internal/state"
	"github.com/deckhouse/virtualization-controller/pkg/eventrecord"
	"github.com/deckhouse/virtualization-controller/pkg/logger"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/vmcondition"
)

const nameClassHandler = "ClassHandler"

func NewClassHandler(client client.Client, recorder eventrecord.EventRecorderLogger) *ClassHandler {
	return &ClassHandler{
		client:   client,
		recorder: recorder,
	}
}

type ClassHandler struct {
	client   client.Client
	recorder eventrecord.EventRecorderLogger
}

func (h *ClassHandler) Handle(ctx context.Context, s state.VirtualMachineState) (reconcile.Result, error) {
	log := logger.FromContext(ctx).With(logger.SlogHandler(nameClassHandler))

	if s.VirtualMachine().IsEmpty() {
		return reconcile.Result{}, nil
	}
	current := s.VirtualMachine().Current()
	changed := s.VirtualMachine().Changed()

	if update := addAllUnknown(changed, vmcondition.TypeClassReady); update {
		return reconcile.Result{}, nil
	}

	if isDeletion(current) {
		return reconcile.Result{}, nil
	}

	class, err := s.Class(ctx)
	if err != nil {
		return reconcile.Result{}, err
	}

	//nolint:staticcheck // it's deprecated.
	mgr := conditions.NewManager(changed.Status.Conditions)
	cb := conditions.NewConditionBuilder(vmcondition.TypeClassReady).
		Generation(current.GetGeneration())

	if class != nil && class.Status.Phase == v1alpha2.ClassPhaseReady {
		if (class.Spec.CPU.Type == v1alpha2.CPUTypeDiscovery || class.Spec.CPU.Type == v1alpha2.CPUTypeFeatures) && len(class.Status.CpuFeatures.Enabled) == 0 {
			mgr.Update(cb.
				Message("No enabled processor features found").
				Reason(vmcondition.ReasonClassNotReady).
				Status(metav1.ConditionFalse).
				Condition())
			changed.Status.Conditions = mgr.Generate()
			return reconcile.Result{RequeueAfter: 2 * time.Second}, nil
		}
		mgr.Update(cb.Reason(vmcondition.ReasonClassReady).Status(metav1.ConditionTrue).Condition())
		changed.Status.Conditions = mgr.Generate()
		return reconcile.Result{}, nil
	}
	className := current.Spec.VirtualMachineClassName
	msg := fmt.Sprintf("VirtualMachineClassName %q is not ready", className)
	reason := vmcondition.ReasonClassNotReady
	if class == nil {
		msg = fmt.Sprintf("VirtualMachineClassName %q not found", className)
		h.recorder.Event(changed, corev1.EventTypeWarning, reason.String(), "VirtualMachineClass not available: waiting for the VirtualMachineClass")
		log.Info("VirtualMachineClass not available: waiting for the VirtualMachineClass")
	}
	mgr.Update(cb.Status(metav1.ConditionFalse).
		Message(msg).
		Reason(reason).
		Condition())
	changed.Status.Conditions = mgr.Generate()
	return reconcile.Result{RequeueAfter: 2 * time.Second}, nil
}

func (h *ClassHandler) Name() string {
	return nameClassHandler
}
