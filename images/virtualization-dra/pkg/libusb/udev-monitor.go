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

package libusb

import (
	"context"
	"log/slog"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/deckhouse/virtualization-dra/pkg/udev"
)

// UdevMonitor is a USB device monitor that uses the udev package for netlink events.
// It provides a clean separation between the generic udev event handling and
// USB-specific device management.
type UdevMonitor struct {
	store       *USBDeviceStore
	log         *slog.Logger
	udevMonitor udevMonitor

	// Configuration
	resyncPeriod     time.Duration
	debounceDuration time.Duration
	useHostNetNS     bool

	// Debouncing
	pendingEvents map[string]*debounceEntry
	debounceMu    sync.Mutex
}

type debounceEntry struct {
	action udev.Action
	timer  *time.Timer
}

// UdevMonitorOption is a functional option for MonitorV5
type UdevMonitorOption func(*UdevMonitor)

// UdevWithResyncPeriod sets the resync period
func UdevWithResyncPeriod(d time.Duration) UdevMonitorOption {
	return func(m *UdevMonitor) {
		m.resyncPeriod = d
	}
}

// UdevWithDebounceDuration sets the debounce duration
func UdevWithDebounceDuration(d time.Duration) UdevMonitorOption {
	return func(m *UdevMonitor) {
		m.debounceDuration = d
	}
}

// UdevWithLogger sets the logger
func UdevWithLogger(log *slog.Logger) UdevMonitorOption {
	return func(m *UdevMonitor) {
		m.log = log
	}
}

// UdevWithHostNetNS configures the monitor to create the netlink socket in the host
// network namespace. This is required when running in a container without host
// networking to receive udev events.
func UdevWithHostNetNS() UdevMonitorOption {
	return func(m *UdevMonitor) {
		m.useHostNetNS = true
	}
}

type udevMonitor interface {
	Start(context.Context) (<-chan *udev.UEvent, <-chan error)
}

// NewUdevMonitor creates a new USB monitor that uses the udev package
func NewUdevMonitor(ctx context.Context, opts ...UdevMonitorOption) (Monitor, error) {
	devices, err := DiscoverPluggedUSBDevices()
	if err != nil {
		return nil, err
	}

	log := slog.With(slog.String("component", "udev-usb-monitor"))

	m := &UdevMonitor{
		store:            NewUSBDeviceStore(devices, log),
		log:              log,
		resyncPeriod:     5 * time.Minute,
		debounceDuration: 100 * time.Millisecond,
		pendingEvents:    make(map[string]*debounceEntry),
	}

	for _, opt := range opts {
		opt(m)
	}

	// Create udev monitor
	udevOpts := []udev.MonitorOption{
		udev.WithMode(udev.KernelEvent),
		udev.WithLogger(m.log),
	}

	if m.useHostNetNS {
		udevOpts = append(udevOpts, udev.WithConnOptions(udev.WithNetNS(udev.HostNetNS)))
	}

	m.udevMonitor = udev.NewMonitor(newUSBDeviceMatcher(), udevOpts...)

	go m.run(ctx)

	return m, nil
}

func (m *UdevMonitor) run(ctx context.Context) {
	// Start udev monitor and get event channel
	eventCh, errCh := m.udevMonitor.Start(ctx)

	// Create resync ticker
	resyncTicker := time.NewTicker(m.resyncPeriod)
	defer resyncTicker.Stop()

	m.log.Info("USB monitor started",
		slog.Duration("resync_period", m.resyncPeriod),
		slog.Duration("debounce_duration", m.debounceDuration),
	)

	for {
		select {
		case <-ctx.Done():
			m.log.Info("USB monitor stopped")
			return

		case event, ok := <-eventCh:
			if !ok {
				m.log.Debug("event channel closed")
				return
			}
			m.handleEvent(event)

		case err, ok := <-errCh:
			if !ok {
				continue
			}
			m.log.Error("udev monitor error", slog.String("error", err.Error()))
			return

		case <-resyncTicker.C:
			m.resync()
		}
	}
}

func (m *UdevMonitor) handleEvent(event *udev.UEvent) {
	m.log.Debug("received uevent",
		slog.String("action", event.Action.String()),
		slog.String("kobj", event.KObj),
	)

	// Convert KObj to sysfs path
	// KObj is like /devices/pci0000:00/.../3-2
	// We need /sys/bus/usb/devices/3-2
	busID := filepath.Base(event.KObj)
	sysfsPath := filepath.Join(pathToUSBDevices, busID)

	m.scheduleEvent(sysfsPath, event.Action)
}

func (m *UdevMonitor) scheduleEvent(path string, action udev.Action) {
	m.debounceMu.Lock()
	defer m.debounceMu.Unlock()

	// Cancel existing timer if any
	if entry, ok := m.pendingEvents[path]; ok {
		entry.timer.Stop()
		// Prioritize remove action
		if action == udev.ActionRemove {
			entry.action = udev.ActionRemove
		}
		delete(m.pendingEvents, path)
	}

	finalAction := action
	timer := time.AfterFunc(m.debounceDuration, func() {
		m.debounceMu.Lock()
		delete(m.pendingEvents, path)
		m.debounceMu.Unlock()

		m.processEvent(path, finalAction)
	})

	m.pendingEvents[path] = &debounceEntry{
		action: action,
		timer:  timer,
	}
}

func (m *UdevMonitor) processEvent(path string, action udev.Action) {
	switch action {
	case udev.ActionAdd, udev.ActionChange, udev.ActionBind, udev.ActionOnline:
		m.handleDeviceUpdate(path)

	case udev.ActionRemove, udev.ActionUnbind, udev.ActionOffline:
		m.handleDeviceRemove(path)
	}
}

func (m *UdevMonitor) handleDeviceUpdate(path string) {
	// Small delay for sysfs to be fully populated
	time.Sleep(50 * time.Millisecond)

	// Check if path exists
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return
	}

	if !isUsbPath(path) {
		return
	}

	slog.Debug("Load usb device", slog.String("path", path))

	device, err := LoadUSBDevice(path)
	if err != nil {
		m.log.Debug("failed to load device", slog.String("path", path), slog.String("error", err.Error()))
		return
	}

	slog.Debug("Validate usb device", slog.String("path", path))

	if err := device.Validate(); err != nil {
		m.log.Debug("device validation failed", slog.String("path", path), slog.String("error", err.Error()))
		return
	}

	slog.Debug("Add usb device", slog.String("path", path))

	m.store.AddDevice(path, &device)
}

func (m *UdevMonitor) handleDeviceRemove(path string) {
	slog.Debug("Remove usb device", slog.String("path", path))
	m.store.RemoveDevice(path)
}

func (m *UdevMonitor) resync() {
	devices, err := DiscoverPluggedUSBDevices()
	if err != nil {
		m.log.Error("failed to discover USB devices during resync", slog.String("error", err.Error()))
		return
	}

	m.store.Resync(devices)
}

// GetDevices returns a copy of all discovered USB devices
func (m *UdevMonitor) GetDevices() []USBDevice {
	return m.store.GetDevices()
}

// GetDevice returns a device by path
func (m *UdevMonitor) GetDevice(path string) (*USBDevice, bool) {
	return m.store.GetDevice(path)
}

// GetDeviceByBusID returns a device by BusID
func (m *UdevMonitor) GetDeviceByBusID(busID string) (*USBDevice, bool) {
	return m.store.GetDeviceByBusID(busID)
}

// DeviceChanges returns a channel that is sent on when the device list changes.
func (m *UdevMonitor) DeviceChanges() <-chan struct{} {
	return m.store.Changes()
}

func newUSBDeviceMatcher() udev.Matcher {
	return &udev.SubsystemDevTypeMatcher{
		Subsystem: "usb",
		DevType:   "usb_device",
	}
}
