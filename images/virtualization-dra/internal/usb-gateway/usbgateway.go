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

package usbgateway

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"strconv"
	"time"

	resourcev1 "k8s.io/api/resource/v1"
	"k8s.io/apimachinery/pkg/util/wait"

	"github.com/deckhouse/virtualization-dra/internal/consts"
	"github.com/deckhouse/virtualization-dra/internal/usb-gateway/informer"
	"github.com/deckhouse/virtualization-dra/pkg/usbip"
)

type USBGateway interface {
	Attach(ctx context.Context, deviceName string) error
	Detach(deviceName string) error
	GetAttachedBusID(deviceName string) (string, error)
	GetAttachedDeviceNames() (map[string]struct{}, error)
}

func (c *USBGatewayController) Attach(ctx context.Context, deviceName string) error {
	busID, host, port, err := c.getDeps(deviceName)
	if err != nil {
		return fmt.Errorf("failed to get attach deps: %w", err)
	}

	log := c.log.With(
		slog.String("deviceName", deviceName),
		slog.String("busID", busID),
		slog.String("host", host),
		slog.Int("port", port),
	)

	err = c.attachRecordManager.Refresh()
	if err != nil {
		return fmt.Errorf("failed to Refresh attach record: %w", err)
	}

	if entry := c.findEntry(deviceName); entry != nil {
		log.Info("Device is already attached", slog.Any("entry", entry))
		return nil
	}

	log.Info("Exporting USB device")
	err = c.usbIP.Export(host, busID, port)
	if err != nil {
		return fmt.Errorf("failed to export device %s: %w", deviceName, err)
	}

	log.Info("Attaching USB device")
	rhport, err := c.usbIP.Attach(host, busID, port)
	if err != nil {
		return fmt.Errorf("failed to attach device %s: %w", deviceName, err)
	}

	usedInfo, err := c.waitUsbAttachInfo(ctx, rhport)
	if err != nil {
		return fmt.Errorf("failed to wait for usb attach info: %w, detach error: %w", err, c.usbIP.Detach(rhport))
	}

	return c.storeAttachRecordOrDetach(deviceName, usedInfo.LocalBusID, rhport)
}

func (c *USBGatewayController) Detach(deviceName string) error {
	busID, host, port, err := c.getDeps(deviceName)
	if err != nil {
		return err
	}

	log := c.log.With(
		slog.String("deviceName", deviceName),
		slog.String("busID", busID),
		slog.String("host", host),
		slog.Int("port", port),
	)

	err = c.attachRecordManager.Refresh()
	if err != nil {
		return fmt.Errorf("failed to Refresh attach record: %w", err)
	}

	entry := c.findEntry(deviceName)
	if entry != nil {
		log.Info("Detaching USB device")
		err = c.usbIP.Detach(entry.Rhport)
		if err != nil {
			return fmt.Errorf("failed to detach device %s: %w", deviceName, err)
		}
	}

	log.Info("Unexporting USB device")
	err = c.usbIP.Unexport(host, busID, port)
	if err != nil {
		return fmt.Errorf("failed to unexport device %s: %w", deviceName, err)
	}

	return nil
}

func (c *USBGatewayController) GetAttachedBusID(deviceName string) (string, error) {
	if err := c.attachRecordManager.Refresh(); err != nil {
		return "", fmt.Errorf("failed to Refresh attach record: %w", err)
	}

	if entry := c.findEntry(deviceName); entry != nil {
		return entry.BusID, nil
	}

	return "", fmt.Errorf("device %s is not attached", deviceName)
}

func (c *USBGatewayController) GetAttachedDeviceNames() (map[string]struct{}, error) {
	if err := c.attachRecordManager.Refresh(); err != nil {
		return nil, fmt.Errorf("failed to Refresh attach record: %w", err)
	}

	entries := c.attachRecordManager.GetEntries()

	names := make(map[string]struct{}, len(entries))
	for _, entry := range entries {
		names[entry.DeviceName] = struct{}{}
	}

	return names, nil
}

func (c *USBGatewayController) getDeps(deviceName string) (string, string, int, error) {
	device, pool, err := c.getDevice(deviceName)
	if err != nil {
		return "", "", -1, err
	}

	busID, err := c.getBusID(device)
	if err != nil {
		return "", "", -1, err
	}

	host, port, err := c.resolveRemoteAddress(pool)
	if err != nil {
		return "", "", -1, err
	}

	return busID, host, port, nil
}

func (c *USBGatewayController) getDevice(deviceName string) (*resourcev1.Device, string, error) {
	resourceSlices, err := c.getVirtualizationDraResourceSlices()
	if err != nil {
		return nil, "", err
	}

	for _, slice := range resourceSlices {
		for _, device := range slice.Spec.Devices {
			if device.Name == deviceName {
				if slice.Spec.Pool.Name == c.nodeName {
					return nil, "", fmt.Errorf("device is not allowed to be attached to itself")
				}

				return &device, slice.Spec.Pool.Name, nil
			}
		}
	}

	return nil, "", fmt.Errorf("device %s is not found", deviceName)
}

func (c *USBGatewayController) getVirtualizationDraResourceSlices() ([]resourcev1.ResourceSlice, error) {
	slicesObj, err := c.resourceSliceIndexer.ByIndex(informer.DriverIndex, consts.VirtualizationDraUSBDriverName)
	if err != nil {
		return nil, err
	}
	var slices []resourcev1.ResourceSlice
	for _, obj := range slicesObj {
		slice, ok := obj.(*resourcev1.ResourceSlice)
		if !ok {
			return nil, fmt.Errorf("unexpected type of resource slice: %T", obj)
		}
		slices = append(slices, *slice.DeepCopy())
	}
	return slices, nil
}

func (c *USBGatewayController) getBusID(device *resourcev1.Device) (string, error) {
	if attr, ok := device.Attributes[consts.AttrBusID]; ok && attr.StringValue != nil {
		return *attr.StringValue, nil
	}
	return "", fmt.Errorf("busID attribute is not exist")
}

func (c *USBGatewayController) resolveRemoteAddress(pool string) (string, int, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	addr, exist := c.nodeAddresses[pool]
	if !exist {
		return "", -1, fmt.Errorf("pool %s is not found", pool)
	}

	host, portStr, err := net.SplitHostPort(addr)
	if err != nil {
		return "", -1, fmt.Errorf("failed to split host and port: %w", err)
	}
	port, err := strconv.Atoi(portStr)
	if err != nil {
		return "", -1, fmt.Errorf("failed to parse port: %w", err)
	}

	return host, port, nil
}

func (c *USBGatewayController) storeAttachRecordOrDetach(deviceName, busID string, rhport int) (err error) {
	entry := AttachEntry{
		Rhport:     rhport,
		BusID:      busID,
		DeviceName: deviceName,
	}

	const maxRetries = 3

	for range maxRetries {
		c.log.Info("Adding entry to attach record", slog.Any("entry", entry))
		err = c.attachRecordManager.AddEntry(entry)
		if err == nil {
			return nil
		}

		c.log.Error("Failed to add entry to attach record", slog.Any("error", err))
	}

	for range maxRetries {
		c.log.Info("Detaching device", slog.Any("deviceName", deviceName))
		err = c.usbIP.Detach(rhport)
		if err == nil {
			return fmt.Errorf("failed to store attach record: %w", err)
		}

		c.log.Error("Failed to detach device", slog.Any("error", err))
	}

	return fmt.Errorf("failed to detach device: %w", err)
}

func (c *USBGatewayController) findEntry(deviceName string) *AttachEntry {
	for _, entry := range c.attachRecordManager.GetEntries() {
		if entry.DeviceName == deviceName {
			return &entry
		}
	}
	return nil
}

func (c *USBGatewayController) waitUsbAttachInfo(ctx context.Context, rhport int) (*usbip.AttachInfo, error) {
	// command attach was successful, but we need to wait until usb is real attached
	var usedInfo *usbip.AttachInfo

	err := wait.PollUntilContextCancel(ctx, time.Second, true, func(ctx context.Context) (bool, error) {
		c.log.Info("Get attach info for store localBusID")
		infos, err := c.usbIP.GetAttachInfo()
		if err != nil {
			c.log.Info("Failed to get used info", slog.String("error", err.Error()))
			return false, nil
		}
		for _, info := range infos {
			if info.Port == rhport {
				usedInfo = &info
				return true, nil
			}
		}
		c.log.Info("Usb are not attached yet")
		return false, nil
	})

	return usedInfo, err
}
