package cpu

import (
	"context"

	"github.com/go-logr/logr"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/manager"

	"github.com/deckhouse/virtualization-controller/api/v1alpha2"
	"github.com/deckhouse/virtualization-controller/pkg/sdk/framework/two_phase_reconciler"
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
