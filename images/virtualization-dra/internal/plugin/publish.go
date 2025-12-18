/*
Copyright 2026 Flant JSC

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

package plugin

import (
	"context"
	"errors"
	"fmt"
	"sync"

	"k8s.io/client-go/kubernetes"
	"k8s.io/dynamic-resource-allocation/kubeletplugin"
	"k8s.io/klog/v2"

	"github.com/deckhouse/virtualization-dra/internal/plugin/wrapresourceslice"
)

type resourcePublisher interface {
	PublishResources(ctx context.Context, resources wrapresourceslice.DriverResources) error
	Stop()
}
type errorHandler func(ctx context.Context, err error, msg string)

func newCustomPublisher(ctx context.Context, driverName, nodeName, poolName string, kubeClient kubernetes.Interface, errorHandler errorHandler) resourcePublisher {
	ctx, cancel := context.WithCancelCause(ctx)
	return &customPublisher{
		driverName:    driverName,
		nodeName:      nodeName,
		poolName:      poolName,
		kubeClient:    kubeClient,
		errorHandler:  errorHandler,
		backgroundCtx: ctx,
		cancel:        cancel,
	}
}

type customPublisher struct {
	driverName    string
	nodeName      string
	poolName      string
	kubeClient    kubernetes.Interface
	backgroundCtx context.Context
	cancel        func(cause error)
	errorHandler  errorHandler

	mutex                   sync.Mutex
	resourceSliceController *wrapresourceslice.Controller
}

func (p *customPublisher) PublishResources(_ context.Context, resources wrapresourceslice.DriverResources) error {
	if p.nodeName == "" {
		return errors.New("no NodeName was set to publish resources")
	}

	p.mutex.Lock()
	defer p.mutex.Unlock()

	owner := wrapresourceslice.Owner{
		APIVersion: "v1",
		Kind:       "Node",
		Name:       p.nodeName,
	}
	driverResources := &wrapresourceslice.DriverResources{
		Pools: resources.Pools,
	}

	if p.resourceSliceController == nil {
		// Start publishing the information. The controller is using
		// our background context, not the one passed into this
		// function, and thus is connected to the lifecycle of the
		// plugin.
		controllerCtx := p.backgroundCtx
		//nolint:contextcheck // copied from dra helper
		controllerLogger := klog.FromContext(controllerCtx)
		controllerLogger = klog.LoggerWithName(controllerLogger, "ResourceSlice controller")
		controllerCtx = klog.NewContext(controllerCtx, controllerLogger)
		var err error
		//nolint:contextcheck // copied from dra helper
		if p.resourceSliceController, err = wrapresourceslice.StartController(controllerCtx,
			wrapresourceslice.Options{
				DriverName: p.driverName,
				KubeClient: p.kubeClient,
				Owner:      &owner,
				Resources:  driverResources,
				ErrorHandler: func(ctx context.Context, err error, msg string) {
					// ResourceSlice publishing errors like dropped fields or
					// invalid spec are not going to get resolved by retrying,
					// but neither is restarting the process going to help
					// -> all errors are recoverable.
					p.errorHandler(ctx, recoverableError{error: err}, msg)
				},
				ReconcileOnlyPoolName: p.poolName,
			}); err != nil {
			return fmt.Errorf("start ResourceSlice controller: %w", err)
		}
		return nil
	}
	// Inform running controller about new information.
	if err := p.resourceSliceController.Update(driverResources); err != nil {
		return fmt.Errorf("failed to update ResourceSlice controller: %w", err)
	}

	return nil
}

func (p *customPublisher) Stop() {
	if p == nil {
		return
	}
	p.cancel(errors.New("customPublisher was stopped"))
}

type recoverableError struct {
	error
}

var _ error = recoverableError{}

func (err recoverableError) Is(other error) bool { return other == kubeletplugin.ErrRecoverable }
func (err recoverableError) Unwrap() error       { return err.error }
