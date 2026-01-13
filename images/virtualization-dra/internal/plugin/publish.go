package plugin

import (
	"context"
	"errors"
	"fmt"
	"sync"

	"k8s.io/client-go/kubernetes"
	"k8s.io/dynamic-resource-allocation/kubeletplugin"
	"k8s.io/dynamic-resource-allocation/resourceslice"
	"k8s.io/klog/v2"
)

type resourcePublisher interface {
	PublishResources(ctx context.Context, resources resourceslice.DriverResources) error
	Stop()
}
type errorHandler func(ctx context.Context, err error, msg string)

func newNonOwnerPublisher(ctx context.Context, kubeClient kubernetes.Interface, errorHandler errorHandler) resourcePublisher {
	ctx, cancel := context.WithCancelCause(ctx)
	return &nonOwnerPublisher{
		driverName:    DriverName,
		kubeClient:    kubeClient,
		errorHandler:  errorHandler,
		backgroundCtx: ctx,
		cancel:        cancel,
	}
}

type nonOwnerPublisher struct {
	driverName    string
	kubeClient    kubernetes.Interface
	backgroundCtx context.Context
	cancel        func(cause error)
	errorHandler  errorHandler

	mutex                   sync.Mutex
	resourceSliceController *resourceslice.Controller
}

func (p *nonOwnerPublisher) PublishResources(_ context.Context, resources resourceslice.DriverResources) error {
	p.mutex.Lock()
	defer p.mutex.Unlock()

	driverResources := &resourceslice.DriverResources{
		Pools: resources.Pools,
	}

	if p.resourceSliceController == nil {
		// Start publishing the information. The controller is using
		// our background context, not the one passed into this
		// function, and thus is connected to the lifecycle of the
		// plugin.
		controllerCtx := p.backgroundCtx
		controllerLogger := klog.FromContext(controllerCtx)
		controllerLogger = klog.LoggerWithName(controllerLogger, "ResourceSlice controller")
		controllerCtx = klog.NewContext(controllerCtx, controllerLogger)
		var err error
		if p.resourceSliceController, err = resourceslice.StartController(controllerCtx,
			resourceslice.Options{
				DriverName: p.driverName,
				KubeClient: p.kubeClient,
				Resources:  driverResources,
				ErrorHandler: func(ctx context.Context, err error, msg string) {
					// ResourceSlice publishing errors like dropped fields or
					// invalid spec are not going to get resolved by retrying,
					// but neither is restarting the process going to help
					// -> all errors are recoverable.
					p.errorHandler(ctx, recoverableError{error: err}, msg)
				},
			}); err != nil {
			return fmt.Errorf("start ResourceSlice controller: %w", err)
		}
	} else {
		// Inform running controller about new information.
		p.resourceSliceController.Update(driverResources)
	}

	return nil
}

func (p *nonOwnerPublisher) Stop() {
	if p == nil {
		return
	}
	p.cancel(errors.New("nonOwnerPublisher was stopped"))
}

type recoverableError struct {
	error
}

var _ error = recoverableError{}

func (err recoverableError) Is(other error) bool { return other == kubeletplugin.ErrRecoverable }
func (err recoverableError) Unwrap() error       { return err.error }
