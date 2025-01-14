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

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/deckhouse/deckhouse/pkg/log"
	"github.com/deckhouse/virtualization-controller/pkg/common/object"
	"github.com/deckhouse/virtualization-controller/pkg/controller/conditions"
	"github.com/deckhouse/virtualization-controller/pkg/controller/vmclass/internal/state"
	"github.com/deckhouse/virtualization-controller/pkg/eventrecord"
	virtv2 "github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/vmclasscondition"
)

const nameDeletionHandler = "DeletionHandler"

func NewDeletionHandler(client client.Client, recorder eventrecord.EventRecorderLogger, logger *log.Logger) *DeletionHandler {
	return &DeletionHandler{
		client:   client,
		recorder: recorder,
		logger:   logger.With("handler", nameDeletionHandler),
	}
}

type DeletionHandler struct {
	client   client.Client
	recorder eventrecord.EventRecorderLogger
	logger   *log.Logger
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
	cb := conditions.NewConditionBuilder(vmclasscondition.TypeInUse).Generation(changed.Generation)
	defer func() { conditions.SetCondition(cb, &changed.Status.Conditions) }()
	vms, err := s.VirtualMachines(ctx)
	if err != nil {
		return reconcile.Result{}, err
	}
	if len(vms) > 0 {
		var vmNamespacedNames []string
		for i := range vms {
			vmNamespacedNames = append(vmNamespacedNames, object.NamespacedName(&vms[i]).String())
		}
		msg := fmt.Sprintf("VirtualMachineClass cannot be deleted, there are VMs that use it. %q", vmNamespacedNames)
		cb.
			Status(metav1.ConditionTrue).
			Reason(vmclasscondition.ReasonVMClassInUse).
			Message(msg)
		return reconcile.Result{RequeueAfter: 60 * time.Second}, nil
	}
	cb.
		Status(metav1.ConditionFalse).
		Reason(vmclasscondition.ReasonVMClassFree).
		Message("")
	h.logger.Info("Deletion observed: remove cleanup finalizer from VirtualMachineClass")
	controllerutil.RemoveFinalizer(changed, virtv2.FinalizerVMCleanup)
	return reconcile.Result{}, nil
}

func (h *DeletionHandler) Name() string {
	return nameDeletionHandler
}
