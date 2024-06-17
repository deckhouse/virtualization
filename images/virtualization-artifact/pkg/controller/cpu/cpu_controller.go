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

package cpu

import (
	"context"

	"github.com/go-logr/logr"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/manager"

	"github.com/deckhouse/virtualization-controller/pkg/sdk/framework/two_phase_reconciler"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
)

const VMCPUControllerName = "vmcpu-controller"

func NewVMCPUController(
	ctx context.Context,
	mgr manager.Manager,
	log logr.Logger,
) (controller.Controller, error) {
	reconciler := NewVMCPUReconciler()

	reconcilerCore := two_phase_reconciler.NewReconcilerCore[*VMCPUReconcilerState](
		reconciler,
		NewVMCPUReconcilerState,
		two_phase_reconciler.ReconcilerOptions{
			Client:   mgr.GetClient(),
			Cache:    mgr.GetCache(),
			Recorder: mgr.GetEventRecorderFor(VMCPUControllerName),
			Scheme:   mgr.GetScheme(),
			Log:      log.WithName(VMCPUControllerName),
		})

	vmcpuController, err := controller.New(VMCPUControllerName, mgr, controller.Options{Reconciler: reconcilerCore})
	if err != nil {
		return nil, err
	}
	if err = reconciler.SetupController(ctx, mgr, vmcpuController); err != nil {
		return nil, err
	}

	if err = builder.WebhookManagedBy(mgr).
		For(&v1alpha2.VirtualMachineCPUModel{}).
		WithValidator(NewVMCPUValidator(log)).
		Complete(); err != nil {
		return nil, err
	}

	log.Info("Initialized VirtualMachineCPUModel controller")

	return vmcpuController, nil
}
