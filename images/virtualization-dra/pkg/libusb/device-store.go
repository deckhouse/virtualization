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
	"log/slog"
	"slices"
	"strings"
	"sync"
)

// USBDeviceStore provides thread-safe storage for USB devices with notification support.
type USBDeviceStore struct {
	mu        sync.RWMutex
	devices   map[string]*USBDevice
	changesCh chan struct{}
	log       *slog.Logger
}

// NewUSBDeviceStore creates a new device store with initial devices.
func NewUSBDeviceStore(devices map[string]*USBDevice, log *slog.Logger) *USBDeviceStore {
	if devices == nil {
		devices = make(map[string]*USBDevice)
	}
	return &USBDeviceStore{
		devices:   devices,
		changesCh: make(chan struct{}, 1),
		log:       log,
	}
}

// Changes return a channel sent on when the device list changes.
// The channel is closed when the store is closed.
func (s *USBDeviceStore) Changes() <-chan struct{} {
	return s.changesCh
}

// Close closes the store and releases resources. No further changes will be sent.
func (s *USBDeviceStore) Close() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.changesCh != nil {
		close(s.changesCh)
		s.changesCh = nil
	}
}

// GetDevices returns a sorted copy of all discovered USB devices.
func (s *USBDeviceStore) GetDevices() []USBDevice {
	s.mu.RLock()
	devices := make([]USBDevice, 0, len(s.devices))
	for _, device := range s.devices {
		devices = append(devices, *device)
	}
	s.mu.RUnlock()

	slices.SortFunc(devices, func(a, b USBDevice) int {
		return strings.Compare(a.Path, b.Path)
	})

	return devices
}

// GetDevice returns a device by path.
func (s *USBDeviceStore) GetDevice(path string) (*USBDevice, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	device, ok := s.devices[path]
	if !ok {
		return nil, false
	}

	deviceCopy := *device
	return &deviceCopy, true
}

// GetDeviceByBusID returns a device by BusID.
func (s *USBDeviceStore) GetDeviceByBusID(busID string) (*USBDevice, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	for _, device := range s.devices {
		if device.BusID == busID {
			deviceCopy := *device
			return &deviceCopy, true
		}
	}
	return nil, false
}

func (s *USBDeviceStore) unlockedSendChange() {
	ch := s.changesCh
	if ch != nil {
		s.log.Debug("Notifying USB device store")
		select {
		case ch <- struct{}{}:
		default:
			// consumer hasn't read yet, skip
		}
	}
}

// AddDevice adds or updates a device and notifies if changed.
func (s *USBDeviceStore) AddDevice(path string, device *USBDevice) bool {
	s.mu.Lock()
	defer s.mu.Unlock()

	oldDevice, exists := s.devices[path]
	needNotify := false
	if !exists || !device.Equal(oldDevice) {
		s.devices[path] = device
		s.log.Info("device added",
			slog.String("path", path),
			slog.String("busid", device.BusID),
			slog.String("product", device.Product),
			slog.String("manufacturer", device.Manufacturer),
			slog.String("serial", device.Serial),
		)
		needNotify = true
	}

	if needNotify {
		s.unlockedSendChange()
	}
	return needNotify
}

// RemoveDevice removes a device and notifies if it existed.
func (s *USBDeviceStore) RemoveDevice(path string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()

	device, exists := s.devices[path]
	needNotify := false
	if exists {
		s.log.Info("device removed",
			slog.String("path", path),
			slog.String("busid", device.BusID),
		)
		delete(s.devices, path)
		needNotify = true
	}

	if needNotify {
		s.unlockedSendChange()
	}
	return needNotify
}

// Resync synchronizes the store with discovered devices and notifies if changed.
func (s *USBDeviceStore) Resync(devices map[string]*USBDevice) bool {
	s.mu.Lock()
	defer s.mu.Unlock()

	changed := false

	// Check for removed devices
	for path := range s.devices {
		if _, exists := devices[path]; !exists {
			s.log.Info("device removed (resync)", slog.String("path", path))
			delete(s.devices, path)
			changed = true
		}
	}

	// Check for added or changed devices
	for path, device := range devices {
		oldDevice, exists := s.devices[path]
		if !exists {
			s.log.Info("device added (resync)",
				slog.String("path", path),
				slog.String("busid", device.BusID),
				slog.String("product", device.Product),
			)
			s.devices[path] = device
			changed = true
		} else if !device.Equal(oldDevice) {
			s.log.Info("device changed (resync)", slog.String("path", path))
			s.devices[path] = device
			changed = true
		}
	}

	if changed {
		s.unlockedSendChange()
	}

	return changed
}

// Exists checks if a device exists at the given path.
func (s *USBDeviceStore) Exists(path string) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	_, exists := s.devices[path]
	return exists
}
