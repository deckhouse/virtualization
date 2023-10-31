package controller

import (
	"context"

	"github.com/go-logr/logr"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/serializer"
	"k8s.io/client-go/rest"
	virtv1 "kubevirt.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/manager"

	"github.com/deckhouse/virtualization-controller/pkg/controller/kvapi"
	"github.com/deckhouse/virtualization-controller/pkg/sdk/framework/two_phase_reconciler"
)

const VMBDAControllerName = "vmbda-controller"

func NewVMBDAController(
	ctx context.Context,
	mgr manager.Manager,
	log logr.Logger,
) (controller.Controller, error) {
	config := *mgr.GetConfig()

	config.GroupVersion = &virtv1.StorageGroupVersion
	config.NegotiatedSerializer = serializer.WithoutConversionCodecFactory{CodecFactory: serializer.NewCodecFactory(runtime.NewScheme())}
	config.APIPath = "/apis"
	config.ContentType = runtime.ContentTypeJSON

	restClient, err := rest.RESTClientFor(&config)
	if err != nil {
		return nil, err
	}

	reconciler := NewVMBDAReconciler(kvapi.NewClient(restClient))

	reconcilerCore := two_phase_reconciler.NewReconcilerCore[*VMBDAReconcilerState](
		reconciler,
		NewVMBDAReconcilerState,
		two_phase_reconciler.ReconcilerOptions{
			Client:   mgr.GetClient(),
			Cache:    mgr.GetCache(),
			Recorder: mgr.GetEventRecorderFor(VMBDAControllerName),
			Scheme:   mgr.GetScheme(),
			Log:      log.WithName(VMBDAControllerName),
		})

	cvmiController, err := controller.New(VMBDAControllerName, mgr, controller.Options{Reconciler: reconcilerCore})
	if err != nil {
		return nil, err
	}
	if err = reconciler.SetupController(ctx, mgr, cvmiController); err != nil {
		return nil, err
	}

	log.Info("Initialized VirtualMachineBlockDeviceAttachment controller")

	return cvmiController, nil
}
