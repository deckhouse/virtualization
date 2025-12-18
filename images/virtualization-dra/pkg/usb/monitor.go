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

package usb

import (
	"context"
	"fmt"
	"log/slog"
	"maps"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"
)

type Monitor struct {
	mu        sync.RWMutex
	devices   map[string]*Device
	watcher   *fsnotify.Watcher
	notifiers []Notifier
}

func NewMonitor(ctx context.Context, resyncPeriod time.Duration) (*Monitor, error) {
	devices, err := DiscoverPluggedUSBDevices(PathToUSBDevices)
	if err != nil {
		return nil, err
	}
	if devices == nil {
		devices = make(map[string]*Device)
	}

	// TODO: recursive watcher
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, err
	}
	if err = watcher.Add(PathToUSBDevices); err != nil {
		_ = watcher.Close()
		return nil, fmt.Errorf("failed to add USB devices path to fsnotify watcher: %w", err)

	}

	monitor := &Monitor{
		devices: devices,
		watcher: watcher,
	}

	go func() {
		monitor.run(ctx, resyncPeriod)
	}()

	return monitor, nil
}

func (m *Monitor) run(ctx context.Context, resyncPeriod time.Duration) {
	for {
		select {
		case <-ctx.Done():
			_ = m.watcher.Close()
			return
		case event := <-m.watcher.Events:
			switch event.Op {
			case fsnotify.Create, fsnotify.Write:
				if err := m.handleUpdate(event); err != nil {
					slog.Error("failed to handle update", slog.String("error", err.Error()))
				}
			case fsnotify.Remove:
				m.handleRemove(event)
			default:
				continue
			}
		case err := <-m.watcher.Errors:
			slog.Error("error watching USB devices", slog.String("error", err.Error()))

		case <-time.After(resyncPeriod):
			devices, err := DiscoverPluggedUSBDevices(PathToUSBDevices)
			if err != nil {
				slog.Error("failed to discover USB devices", slog.String("error", err.Error()))
				continue
			}
			if devices == nil {
				devices = make(map[string]*Device)
			}
			m.mu.Lock()
			if !maps.Equal(m.devices, devices) {
				m.devices = devices
				m.notify()
			}
			m.mu.Unlock()
		}
	}
}

func (m *Monitor) handleUpdate(event fsnotify.Event) error {
	path := event.Name
	if !isUsbPath(path) {
		return nil
	}

	// Get device information
	device, err := LoadDevice(path)
	if err != nil {
		return err
	}
	if err = device.Validate(); err != nil {
		slog.Error("failed to validate device, skip...", slog.Any("device", device), slog.String("error", err.Error()))
		return nil
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	oldDevice, ok := m.devices[path]
	if !ok || !device.Equal(oldDevice) {
		m.devices[path] = &device
		m.notify()
	}

	return nil
}

func (m *Monitor) handleRemove(event fsnotify.Event) {
	path := event.Name

	m.mu.Lock()
	defer m.mu.Unlock()

	if _, ok := m.devices[path]; ok {
		delete(m.devices, path)
		m.notify()
	}
}

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

func (m *Monitor) AddNotifier(notifier Notifier) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.notifiers = append(m.notifiers, notifier)
}

func (m *Monitor) notify() {
	for _, notifier := range m.notifiers {
		notifier.Notify()
	}
}

type Notifier interface {
	Notify()
}
