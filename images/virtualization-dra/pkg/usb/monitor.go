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

package usb

import (
	"context"
	"log/slog"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/deckhouse/virtualization-dra/pkg/udev"
)

// Monitor is a USB device monitor that uses the udev package for netlink events.
// It provides a clean separation between the generic udev event handling and
// USB-specific device management.
type Monitor struct {
	mu        sync.RWMutex
	devices   map[string]*Device
	notifiers []Notifier
	log       *slog.Logger

	// Configuration
	resyncPeriod     time.Duration
	debounceDuration time.Duration

	// Debouncing
	pendingEvents map[string]*debounceEntry
	debounceMu    sync.Mutex
}

type debounceEntry struct {
	action udev.Action
	timer  *time.Timer
}

// MonitorOption is a functional option for MonitorV5
type MonitorOption func(*Monitor)

// WithResyncPeriod sets the resync period
func WithResyncPeriod(d time.Duration) MonitorOption {
	return func(m *Monitor) {
		m.resyncPeriod = d
	}
}

// WithDebounceDuration sets the debounce duration
func WithDebounceDuration(d time.Duration) MonitorOption {
	return func(m *Monitor) {
		m.debounceDuration = d
	}
}

// WithLogger sets the logger
func WithLogger(log *slog.Logger) MonitorOption {
	return func(m *Monitor) {
		m.log = log
	}
}

// NewMonitor creates a new USB monitor that uses the udev package
func NewMonitor(ctx context.Context, opts ...MonitorOption) (*Monitor, error) {
	devices, err := DiscoverPluggedUSBDevices(PathToUSBDevices)
	if err != nil {
		return nil, err
	}
	if devices == nil {
		devices = make(map[string]*Device)
	}

	m := &Monitor{
		devices:          devices,
		log:              slog.With(slog.String("component", "usb-monitor")),
		resyncPeriod:     5 * time.Minute,
		debounceDuration: 100 * time.Millisecond,
		pendingEvents:    make(map[string]*debounceEntry),
	}

	for _, opt := range opts {
		opt(m)
	}

	go m.run(ctx)

	return m, nil
}

func (m *Monitor) run(ctx context.Context) {
	// Create udev monitor
	udevMonitor := udev.NewMonitor(
		newUSBDeviceMatcher(),
		udev.WithMode(udev.KernelEvent),
		udev.WithLogger(m.log),
	)

	// Start udev monitor and get event channel
	eventCh, errCh := udevMonitor.Start(ctx)

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

func (m *Monitor) handleEvent(event *udev.UEvent) {
	m.log.Debug("received uevent",
		slog.String("action", event.Action.String()),
		slog.String("kobj", event.KObj),
	)

	// Convert KObj to sysfs path
	// KObj is like /devices/pci0000:00/.../3-2
	// We need /sys/bus/usb/devices/3-2
	busID := filepath.Base(event.KObj)
	sysfsPath := filepath.Join(PathToUSBDevices, busID)

	m.scheduleEvent(sysfsPath, event.Action)
}

func (m *Monitor) scheduleEvent(path string, action udev.Action) {
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

func (m *Monitor) processEvent(path string, action udev.Action) {
	m.mu.Lock()
	defer m.mu.Unlock()

	switch action {
	case udev.ActionAdd, udev.ActionChange, udev.ActionBind, udev.ActionOnline:
		m.handleDeviceUpdate(path)

	case udev.ActionRemove, udev.ActionUnbind, udev.ActionOffline:
		m.handleDeviceRemove(path)
	}
}

func (m *Monitor) handleDeviceUpdate(path string) {
	// Small delay for sysfs to be fully populated
	time.Sleep(50 * time.Millisecond)

	// Check if path exists
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return
	}

	if !isUsbPath(path) {
		return
	}

	device, err := LoadDevice(path)
	if err != nil {
		m.log.Debug("failed to load device", slog.String("path", path), slog.String("error", err.Error()))
		return
	}

	if err := device.Validate(); err != nil {
		m.log.Debug("device validation failed", slog.String("path", path), slog.String("error", err.Error()))
		return
	}

	oldDevice, exists := m.devices[path]
	if !exists {
		m.devices[path] = &device
		m.log.Info("device added",
			slog.String("path", path),
			slog.String("busid", device.BusID),
			slog.String("product", device.Product),
			slog.String("manufacturer", device.Manufacturer),
			slog.String("serial", device.Serial),
		)
		m.notify()
	} else if !device.Equal(oldDevice) {
		m.devices[path] = &device
		m.log.Info("device updated",
			slog.String("path", path),
			slog.String("busid", device.BusID),
			slog.String("product", device.Product),
		)
		m.notify()
	}
}

func (m *Monitor) handleDeviceRemove(path string) {
	if device, exists := m.devices[path]; exists {
		m.log.Info("device removed",
			slog.String("path", path),
			slog.String("busid", device.BusID),
		)
		delete(m.devices, path)
		m.notify()
	}
}

func (m *Monitor) resync() {
	devices, err := DiscoverPluggedUSBDevices(PathToUSBDevices)
	if err != nil {
		m.log.Error("failed to discover USB devices during resync", slog.String("error", err.Error()))
		return
	}
	if devices == nil {
		devices = make(map[string]*Device)
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	changed := false

	// Check for removed devices
	for path := range m.devices {
		if _, exists := devices[path]; !exists {
			m.log.Info("device removed (resync)", slog.String("path", path))
			delete(m.devices, path)
			changed = true
		}
	}

	// Check for added or changed devices
	for path, device := range devices {
		oldDevice, exists := m.devices[path]
		if !exists {
			m.log.Info("device added (resync)", slog.String("path", path))
			m.devices[path] = device
			changed = true
		} else if !device.Equal(oldDevice) {
			m.log.Info("device changed (resync)", slog.String("path", path))
			m.devices[path] = device
			changed = true
		}
	}

	if changed {
		m.notify()
	}
}

// GetDevices returns a copy of all discovered USB devices
func (m *Monitor) GetDevices() []Device {
	m.mu.RLock()
	devices := make([]Device, 0, len(m.devices))
	for _, device := range m.devices {
		devices = append(devices, *device)
	}
	m.mu.RUnlock()

	slices.SortFunc(devices, func(a, b Device) int {
		return strings.Compare(a.DevicePath, b.DevicePath)
	})

	return devices
}

// GetDevice returns a device by path
func (m *Monitor) GetDevice(path string) (*Device, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	device, ok := m.devices[path]
	if !ok {
		return nil, false
	}

	deviceCopy := *device
	return &deviceCopy, true
}

// GetDeviceByBusID returns a device by BusID
func (m *Monitor) GetDeviceByBusID(busID string) (*Device, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	for _, device := range m.devices {
		if device.BusID == busID {
			deviceCopy := *device
			return &deviceCopy, true
		}
	}
	return nil, false
}

// AddNotifier adds a notifier to be called on device changes
func (m *Monitor) AddNotifier(notifier Notifier) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.notifiers = append(m.notifiers, notifier)
}

// RemoveNotifier removes a notifier
func (m *Monitor) RemoveNotifier(notifier Notifier) {
	m.mu.Lock()
	defer m.mu.Unlock()

	for i, n := range m.notifiers {
		if n == notifier {
			m.notifiers = slices.Delete(m.notifiers, i, i+1)
			return
		}
	}
}

func (m *Monitor) notify() {
	for _, notifier := range m.notifiers {
		notifier.Notify()
	}
}

type Notifier interface {
	Notify()
}

type FuncNotifier func()

func (f FuncNotifier) Notify() {
	f()
}

func newUSBDeviceMatcher() udev.Matcher {
	return &udev.SubsystemDevTypeMatcher{
		Subsystem: "usb",
		DevType:   "usb_device",
	}
}
