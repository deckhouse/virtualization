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

package postponehandler

import (
	"context"
	"fmt"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/deckhouse/virtualization-controller/pkg/controller/conditions"
	"github.com/deckhouse/virtualization-controller/pkg/controller/reconciler"
	"github.com/deckhouse/virtualization-controller/pkg/eventrecord"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/cvicondition"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/vdcondition"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/vicondition"
)

type DVCRService interface {
	GetMaintenanceSecret(ctx context.Context) (*corev1.Secret, error)
	IsMaintenanceInitiatedOrInProgress(*corev1.Secret) bool
}

var PostponePeriod = time.Second * 15

type Postpone[object client.Object] struct {
	dvcrService DVCRService
	recorder    eventrecord.EventRecorderLogger
}

func New[T client.Object](dvcrService DVCRService, recorder eventrecord.EventRecorderLogger) *Postpone[T] {
	return &Postpone[T]{
		dvcrService: dvcrService,
		recorder:    recorder,
	}
}

// Handle sets Ready condition to Provisioning for newly created resources
// if dvcr is in the maintenance mode.
// Applicable for ClusterVirtualImage, VirtualImage, and VirtualDisk.
func (p *Postpone[T]) Handle(ctx context.Context, obj T) (reconcile.Result, error) {
	maintenanceSecret, err := p.dvcrService.GetMaintenanceSecret(ctx)
	if err != nil {
		return reconcile.Result{}, fmt.Errorf("checking DVCR maintenance mode: %w", err)
	}

	conditionsPtr := conditions.NewConditionsAccessor(obj).Conditions()
	// Only newly created resources are marked to postpone.

	readyCondition, exists := conditions.GetCondition(getReadyType(obj), *conditionsPtr)
	isAlreadyPostponed := exists && readyCondition.Reason == ProvisioningPostponedReason.String()
	isMaintenance := p.dvcrService.IsMaintenanceInitiatedOrInProgress(maintenanceSecret)

	// Clear PostponeProvisioning reason if maintenance finished.
	if !isMaintenance {
		if isAlreadyPostponed {
			p.recorder.Event(
				obj,
				corev1.EventTypeNormal,
				v1alpha2.ReasonImageOperationContinueAfterDVCRMaintenance,
				"Continue image operation after finishing DVCR maintenance mode.",
			)
			conditions.RemoveCondition(getReadyType(obj), conditionsPtr)
		}
		return reconcile.Result{}, nil
	}

	// Postpone only newly created resources without Ready condition.
	if !exists {
		p.recorder.Event(
			obj,
			corev1.EventTypeNormal,
			v1alpha2.ReasonImageOperationPostponedDueToDVCRMaintenance,
			"Postpone image operation until the end of DVCR maintenance mode.",
		)

		// Set Provisioning to False.
		cb := conditions.NewConditionBuilder(getReadyType(obj)).Generation(obj.GetGeneration())
		cb.Status(metav1.ConditionFalse).
			Reason(ProvisioningPostponedReason).
			Message("DVCR is in maintenance mode: wait until it finishes before creating provisioner.")
		conditions.SetCondition(cb, conditions.NewConditionsAccessor(obj).Conditions())
		return reconcile.Result{RequeueAfter: PostponePeriod}, reconciler.ErrStopHandlerChain
	}

	// Pass through resources existed before enabling maintenance mode.
	return reconcile.Result{}, nil
}

func (p *Postpone[T]) Name() string {
	return "postpone-on-dvcr-maintenance-handler"
}

func getReadyType(obj client.Object) conditions.Stringer {
	switch obj.(type) {
	case *v1alpha2.ClusterVirtualImage:
		return cvicondition.ReadyType
	case *v1alpha2.VirtualImage:
		return vicondition.ReadyType
	case *v1alpha2.VirtualDisk:
		return vdcondition.ReadyType
	}

	return stringer{str: "Ready"}
}

type stringer struct {
	str string
}

func (s stringer) String() string {
	return s.str
}

var ProvisioningPostponedReason = stringer{str: "ProvisioningPostponed"}
