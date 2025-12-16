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

	"github.com/deckhouse/deckhouse/pkg/log"
	"k8s.io/api/resource/v1"
	resourceapi "k8s.io/api/resource/v1"
	"k8s.io/apimachinery/pkg/types"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/client-go/kubernetes"
	"k8s.io/dynamic-resource-allocation/kubeletplugin"
	"k8s.io/dynamic-resource-allocation/resourceslice"
)

const DriverName = "virtualization-dra"

func NewDriver(nodeName string, kubeClient kubernetes.Interface, allocator Allocator, log *slog.Logger) *Driver {
	return &Driver{
		nodeName:   nodeName,
		kubeClient: kubeClient,
		allocator:  allocator,
		log:        log.With(slog.String("driver", DriverName), slog.String("component", "driver")),
	}
}

type Driver struct {
	nodeName string

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
		kubeletplugin.DriverName(DriverName),
		kubeletplugin.RegistrarDirectoryPath(virtualizationRegistrarDirPath()),
		kubeletplugin.RegistrarSocketFilename(virtualizationRegistrarSocketFilename),
		kubeletplugin.PluginDataDirectoryPath(virtualizationPluginDirPath()),
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

func (d *Driver) PrepareResourceClaims(ctx context.Context, claims []*v1.ResourceClaim) (map[types.UID]kubeletplugin.PrepareResult, error) {
	d.log.Info("Preparing resource claims")

	result := make(map[types.UID]kubeletplugin.PrepareResult, len(claims))

	for _, claim := range claims {
		result[claim.UID] = d.prepareResourceClaim(ctx, claim)
	}

	return result, nil
}

func (d *Driver) prepareResourceClaim(ctx context.Context, claim *resourceapi.ResourceClaim) kubeletplugin.PrepareResult {
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
			case devices := <-ch:
				d.log.Info("Publishing devices", slog.Any("devices", devices))
				if len(devices) > 0 {
				}
				resources := d.makeResources(devices)
				err := d.helper.PublishResources(ctx, resources)
				if err != nil {
					d.log.Error("Failed to publish devices", slog.Any("err", err))
				}
			}
		}
	}()
}

func (d *Driver) makeResources(devices []resourceapi.Device) resourceslice.DriverResources {
	return resourceslice.DriverResources{
		Pools: map[string]resourceslice.Pool{
			d.nodeName: {
				Slices: []resourceslice.Slice{
					{
						Devices: devices,
					},
				},
			},
		},
	}
}
