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

package plugin

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	resourcev1 "k8s.io/api/resource/v1"
	"k8s.io/apimachinery/pkg/types"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/client-go/kubernetes"
	"k8s.io/dynamic-resource-allocation/kubeletplugin"

	"github.com/deckhouse/deckhouse/pkg/log"
)

func NewDriver(driverName, nodeName string, kubeClient kubernetes.Interface, allocator Allocator) (*Driver, error) {
	if driverName == "" {
		return nil, fmt.Errorf("driver name is required")
	}

	if err := initPluginDir(driverName); err != nil {
		return nil, fmt.Errorf("failed to initialize plugin directory: %w", err)
	}

	return &Driver{
		driverName: driverName,
		nodeName:   nodeName,
		kubeClient: kubeClient,
		allocator:  allocator,
		log:        slog.With(slog.String("driver", driverName), slog.String("component", "driver")),
	}, nil
}

// Driver is the DRA kubelet plugin: it is what the kubelet talks to (gRPC) to prepare/unprepare resources for pods.
// This package implements the protocol and registration; you only implement Allocator — no duplicate kubelet/ResourceSlice logic.
type Driver struct {
	driverName string
	nodeName   string

	kubeClient kubernetes.Interface
	allocator  Allocator
	log        *slog.Logger

	helper       *kubeletplugin.Helper
	pluginCtx    context.Context
	pluginCancel context.CancelCauseFunc
}

func (d *Driver) Start(ctx context.Context) error {
	ctx, cancel := context.WithCancelCause(ctx)
	d.pluginCtx = ctx
	d.pluginCancel = cancel

	log.Info("Starting dra plugin")
	helper, err := kubeletplugin.Start(
		ctx,
		d,
		kubeletplugin.KubeClient(d.kubeClient),
		kubeletplugin.NodeName(d.nodeName),
		kubeletplugin.DriverName(d.driverName),
		kubeletplugin.RegistrarDirectoryPath(registrarDirPath()),
		kubeletplugin.RegistrarSocketFilename(registrarSocketFile(d.driverName)),
		kubeletplugin.PluginDataDirectoryPath(pluginDirPath(d.driverName)),
	)
	if err != nil {
		return fmt.Errorf("failed to start kubelet plugin: %w", err)
	}

	d.helper = helper
	d.startPublisher(ctx)

	return err
}

func (d *Driver) Wait() {
	if d.pluginCtx != nil {
		<-d.pluginCtx.Done()
	}
}

func (d *Driver) Shutdown() {
	if d.helper != nil {
		d.log.Info("Stopping dra plugin")
		d.helper.Stop()
	}
}

func (d *Driver) PrepareResourceClaims(ctx context.Context, claims []*resourcev1.ResourceClaim) (map[types.UID]kubeletplugin.PrepareResult, error) {
	if len(claims) == 0 {
		return nil, nil
	}

	d.log.Info("Preparing resource claims")

	result := make(map[types.UID]kubeletplugin.PrepareResult, len(claims))

	for _, claim := range claims {
		result[claim.UID] = d.prepareResourceClaim(ctx, claim)
	}

	return result, nil
}

func (d *Driver) prepareResourceClaim(ctx context.Context, claim *resourcev1.ResourceClaim) kubeletplugin.PrepareResult {
	if claim.Status.Allocation == nil {
		return kubeletplugin.PrepareResult{
			Err: fmt.Errorf("claim %s/%s has no allocation", claim.Namespace, claim.Name),
		}
	}

	preparedPBs, err := d.allocator.Prepare(ctx, claim)
	if err != nil {
		return kubeletplugin.PrepareResult{
			Err: fmt.Errorf("error preparing devices for claim %v: %w", claim.UID, err),
		}
	}
	var prepared []kubeletplugin.Device
	for _, preparedPB := range preparedPBs {
		prepared = append(prepared, kubeletplugin.Device{
			Requests:     preparedPB.GetRequestNames(),
			PoolName:     preparedPB.GetPoolName(),
			DeviceName:   preparedPB.GetDeviceName(),
			CDIDeviceIDs: preparedPB.GetCDIDeviceIDs(),
		})
	}

	d.log.Info("Returning newly prepared devices", slog.String("uid", string(claim.UID)), slog.Any("devices", prepared))
	return kubeletplugin.PrepareResult{Devices: prepared}
}

func (d *Driver) UnprepareResourceClaims(ctx context.Context, claims []kubeletplugin.NamespacedObject) (map[types.UID]error, error) {
	if len(claims) == 0 {
		return nil, nil
	}

	d.log.Info("Unpreparing resource claims")

	result := make(map[types.UID]error)

	for _, claim := range claims {
		result[claim.UID] = d.unprepareResourceClaim(ctx, claim)
	}

	return result, nil
}

func (d *Driver) unprepareResourceClaim(ctx context.Context, claim kubeletplugin.NamespacedObject) error {
	if err := d.allocator.Unprepare(ctx, claim.UID); err != nil {
		return fmt.Errorf("error unpreparing devices for claim %v: %w", claim.UID, err)
	}

	return nil
}

func (d *Driver) HandleError(ctx context.Context, err error, msg string) {
	utilruntime.HandleErrorWithContext(ctx, err, msg)
	if !errors.Is(err, kubeletplugin.ErrRecoverable) && d.pluginCancel != nil {
		d.pluginCancel(fmt.Errorf("fatal background error: %w", err))
	}
}

func (d *Driver) startPublisher(ctx context.Context) {
	go func() {
		ch := d.allocator.UpdateChannel()
		for {
			select {
			case <-ctx.Done():
				return
			case resources := <-ch:
				d.log.Info("Publishing devices", slog.Any("resources", resources))
				err := d.helper.PublishResources(ctx, resources)
				if err != nil {
					d.log.Error("Failed to publish devices", slog.Any("err", err))
				}
			}
		}
	}()
}
