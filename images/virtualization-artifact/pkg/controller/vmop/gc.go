package vmop

import (
	"log/slog"
	"time"

	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/manager"

	"github.com/deckhouse/virtualization-controller/pkg/controller/gc"
	"github.com/deckhouse/virtualization-controller/pkg/sdk/framework/helper"
	virtv2 "github.com/deckhouse/virtualization/api/core/v1alpha2"
)

const gcControllerName = "vmop-gc-controller"

func SetupGC(
	mgr manager.Manager,
	log *slog.Logger,
) error {
	return gc.SetupGcController(gcControllerName,
		mgr,
		log,
		gc.NewCronSource(mgr.GetClient(),
			"0 * * * *",
			&virtv2.VirtualMachineOperationList{},
			gc.CronSourceOption{
				GetOlder: func(objList client.ObjectList) client.ObjectList {
					return objList
				},
			},
			log,
		),
		func() client.Object {
			return &virtv2.VirtualMachineOperation{}
		},
		func(obj client.Object) bool {
			vmop, ok := obj.(*virtv2.VirtualMachineOperation)
			if !ok {
				return false
			}
			if vmopIsFinal(vmop) && helper.GetAge(vmop) > 24*time.Hour {
				return true
			}
			return false
		},
	)
}

func vmopIsFinal(vmop *virtv2.VirtualMachineOperation) bool {
	return vmop.Status.Phase == virtv2.VMOPPhaseCompleted || vmop.Status.Phase == virtv2.VMOPPhaseFailed
}
