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

	"github.com/fsnotify/fsnotify"
)

// InotifyMonitor is a USB device monitor that uses inotify/fsnotify to watch
// the sysfs USB devices directory and uevent files for changes.
type InotifyMonitor struct {
	store   *USBDeviceStore
	watcher *fsnotify.Watcher
	log     *slog.Logger

	// Configuration
	resyncPeriod     time.Duration
	debounceDuration time.Duration

	// Debouncing
	debounceTimer *time.Timer
	debounceMu    sync.Mutex

	// Track watched paths
	watchedPaths map[string]struct{}
	watchMu      sync.Mutex
}

// InotifyMonitorOption is a functional option for InotifyMonitor
type InotifyMonitorOption func(*InotifyMonitor)

// InotifyWithResyncPeriod sets the resync period
func InotifyWithResyncPeriod(d time.Duration) InotifyMonitorOption {
	return func(m *InotifyMonitor) {
		m.resyncPeriod = d
	}
}

// InotifyWithDebounceDuration sets the debounce duration for events
func InotifyWithDebounceDuration(d time.Duration) InotifyMonitorOption {
	return func(m *InotifyMonitor) {
		m.debounceDuration = d
	}
}

// InotifyWithLogger sets the logger
func InotifyWithLogger(log *slog.Logger) InotifyMonitorOption {
	return func(m *InotifyMonitor) {
		m.log = log
	}
}

// NewInotifyMonitor creates a new USB monitor that uses inotify/fsnotify
func NewInotifyMonitor(ctx context.Context, opts ...InotifyMonitorOption) (Monitor, error) {
	devices, err := DiscoverPluggedUSBDevices()
	if err != nil {
		return nil, err
	}

	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, err
	}

	log := slog.With(slog.String("component", "inotify-usb-monitor"))

	m := &InotifyMonitor{
		store:            NewUSBDeviceStore(devices, log),
		watcher:          watcher,
		log:              log,
		resyncPeriod:     5 * time.Minute,
		debounceDuration: 200 * time.Millisecond,
		watchedPaths:     make(map[string]struct{}),
	}

	for _, opt := range opts {
		opt(m)
	}

	// Watch the main USB devices directory
	if err := m.watchDirectory(PathToUSBDevices); err != nil {
		watcher.Close()
		return nil, err
	}

	// Watch existing device directories and their uevent files
	m.watchExistingDevices()

	go m.run(ctx)

	return m, nil
}

func (m *InotifyMonitor) run(ctx context.Context) {
	resyncTicker := time.NewTicker(m.resyncPeriod)
	defer resyncTicker.Stop()
	defer m.watcher.Close()

	m.log.Info("Inotify USB monitor started",
		slog.Duration("resync_period", m.resyncPeriod),
		slog.Duration("debounce_duration", m.debounceDuration),
	)

	for {
		select {
		case <-ctx.Done():
			m.log.Info("Inotify USB monitor stopped")
			return

		case event, ok := <-m.watcher.Events:
			if !ok {
				m.log.Debug("watcher events channel closed")
				return
			}
			m.handleEvent(event)

		case err, ok := <-m.watcher.Errors:
			if !ok {
				m.log.Debug("watcher errors channel closed")
				return
			}
			m.log.Error("inotify watcher error", slog.String("error", err.Error()))

		case <-resyncTicker.C:
			m.resync()
		}
	}
}

func (m *InotifyMonitor) handleEvent(event fsnotify.Event) {
	m.log.Debug("received fsnotify event",
		slog.String("name", event.Name),
		slog.String("op", event.Op.String()),
	)

	// Handle directory creation (new USB device)
	if event.Op&fsnotify.Create != 0 {
		m.handleCreate(event.Name)
	}

	// Handle directory removal (USB device disconnected)
	if event.Op&fsnotify.Remove != 0 {
		m.handleRemove(event.Name)
	}

	// Handle uevent file changes
	if event.Op&fsnotify.Write != 0 {
		m.handleWrite(event.Name)
	}
}

func (m *InotifyMonitor) handleCreate(path string) {
	// Only handle new USB device directories in /sys/bus/usb/devices/
	if filepath.Dir(path) != PathToUSBDevices {
		return
	}

	if !isUsbPath(path) {
		return
	}

	m.log.Debug("new USB device directory", slog.String("path", path))
	m.watchDeviceDirectory(path)
	m.scheduleResync()
}

func (m *InotifyMonitor) handleRemove(path string) {
	// Check if this was a watched USB device directory
	if filepath.Dir(path) == PathToUSBDevices {
		m.log.Debug("USB device directory removed", slog.String("path", path))
		m.unwatchPath(path)
		m.unwatchPath(filepath.Join(path, "uevent"))
		m.scheduleResync()
		return
	}

	// For any other watched path that was removed
	m.unwatchPath(path)
}

func (m *InotifyMonitor) handleWrite(path string) {
	// Write events are only for files, not directories.
	// We only care about uevent file changes.
	if filepath.Base(path) != "uevent" {
		return
	}

	devicePath := filepath.Dir(path)
	m.log.Debug("uevent file changed", slog.String("path", devicePath))
	m.scheduleResync()
}

func (m *InotifyMonitor) watchDirectory(path string) error {
	m.watchMu.Lock()
	defer m.watchMu.Unlock()

	if _, exists := m.watchedPaths[path]; exists {
		return nil
	}

	if err := m.watcher.Add(path); err != nil {
		return err
	}

	m.watchedPaths[path] = struct{}{}
	m.log.Debug("watching directory", slog.String("path", path))
	return nil
}

func (m *InotifyMonitor) unwatchPath(path string) {
	m.watchMu.Lock()
	defer m.watchMu.Unlock()

	if _, exists := m.watchedPaths[path]; !exists {
		return
	}

	_ = m.watcher.Remove(path)
	delete(m.watchedPaths, path)
	m.log.Debug("unwatched path", slog.String("path", path))
}

func (m *InotifyMonitor) watchExistingDevices() {
	entries, err := os.ReadDir(PathToUSBDevices)
	if err != nil {
		m.log.Error("failed to read USB devices directory",
			slog.String("path", PathToUSBDevices),
			slog.String("error", err.Error()),
		)
		return
	}

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		devicePath := filepath.Join(PathToUSBDevices, entry.Name())
		if !isUsbPath(devicePath) {
			continue
		}

		m.watchDeviceDirectory(devicePath)
	}
}

func (m *InotifyMonitor) watchDeviceDirectory(devicePath string) {
	// Watch the device directory itself
	if err := m.watchDirectory(devicePath); err != nil {
		m.log.Debug("failed to watch device directory",
			slog.String("path", devicePath),
			slog.String("error", err.Error()),
		)
		return
	}

	// Watch the uevent file for changes
	ueventPath := filepath.Join(devicePath, "uevent")
	if _, err := os.Stat(ueventPath); err == nil {
		if err := m.watchDirectory(ueventPath); err != nil {
			m.log.Debug("failed to watch uevent file",
				slog.String("path", ueventPath),
				slog.String("error", err.Error()),
			)
		}
	}
}

// scheduleResync schedules a resync with debouncing
func (m *InotifyMonitor) scheduleResync() {
	m.debounceMu.Lock()
	defer m.debounceMu.Unlock()

	if m.debounceTimer != nil {
		m.debounceTimer.Stop()
	}

	m.debounceTimer = time.AfterFunc(m.debounceDuration, func() {
		m.resync()
	})
}

func (m *InotifyMonitor) resync() {
	devices, err := DiscoverPluggedUSBDevices()
	if err != nil {
		m.log.Error("failed to discover USB devices during resync",
			slog.String("error", err.Error()),
		)
		return
	}

	// Update watches for any new devices
	m.watchExistingDevices()

	m.store.Resync(devices)
}

// GetDevices returns a copy of all discovered USB devices
func (m *InotifyMonitor) GetDevices() []USBDevice {
	return m.store.GetDevices()
}

// GetDevice returns a device by path
func (m *InotifyMonitor) GetDevice(path string) (*USBDevice, bool) {
	return m.store.GetDevice(path)
}

// GetDeviceByBusID returns a device by BusID
func (m *InotifyMonitor) GetDeviceByBusID(busID string) (*USBDevice, bool) {
	return m.store.GetDeviceByBusID(busID)
}

// DeviceChanges returns a channel that is sent on when the device list changes.
func (m *InotifyMonitor) DeviceChanges() <-chan struct{} {
	return m.store.Changes()
}
