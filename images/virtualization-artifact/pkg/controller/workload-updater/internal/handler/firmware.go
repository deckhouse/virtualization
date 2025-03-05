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

package handler

import (
	"context"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/deckhouse/virtualization-controller/pkg/common/annotations"
	"github.com/deckhouse/virtualization-controller/pkg/controller/conditions"
	"github.com/deckhouse/virtualization-controller/pkg/logger"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/vmcondition"
)

const firmwareHandler = "FirmwareHandler"

func NewFirmwareHandler(client client.Client, migration OneShotMigration, firmwareImage string) *FirmwareHandler {
	return &FirmwareHandler{
		client:           client,
		oneShotMigration: migration,
		firmwareImage:    firmwareImage,
	}
}

type FirmwareHandler struct {
	client           client.Client
	oneShotMigration OneShotMigration
	firmwareImage    string
}

func (h *FirmwareHandler) Handle(ctx context.Context, vm *v1alpha2.VirtualMachine) (reconcile.Result, error) {
	if vm == nil || !vm.GetDeletionTimestamp().IsZero() {
		return reconcile.Result{}, nil
	}

	if !h.needUpdate(vm) {
		return reconcile.Result{}, nil
	}

	log := logger.FromContext(ctx).With(logger.SlogHandler(firmwareHandler))
	h.oneShotMigration.SetLogger(log)

	migrate, err := h.oneShotMigration.OnceMigrate(ctx, vm, annotations.AnnVMOPWorkloadUpdateImage, h.firmwareImage)
	if migrate {
		log.Info("The virtual machine was triggered to migrate by the firmware handler.")
	}

	return reconcile.Result{}, err
}

func (h *FirmwareHandler) Name() string {
	return firmwareHandler
}

func (h *FirmwareHandler) needUpdate(vm *v1alpha2.VirtualMachine) bool {
	if upToDate, exists := conditions.GetCondition(vmcondition.TypeFirmwareUpToDate, vm.Status.Conditions); exists {
		return upToDate.Status == metav1.ConditionFalse
	}
	return false
}
