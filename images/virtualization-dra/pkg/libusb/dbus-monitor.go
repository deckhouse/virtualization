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
	"sync"
	"time"

	"github.com/godbus/dbus/v5"
)

// DBusMonitor is a USB device monitor that uses D-Bus to listen for USB device events
// via UDisks2 interface. It provides an alternative to udev-based monitoring.
type DBusMonitor struct {
	store *USBDeviceStore
	log   *slog.Logger

	// Configuration
	resyncPeriod     time.Duration
	reconnectDelay   time.Duration
	debounceDuration time.Duration

	// Debouncing
	debounceTimer *time.Timer
	debounceMu    sync.Mutex
}

// DBusMonitorOption is a functional option for DBusMonitor
type DBusMonitorOption func(*DBusMonitor)

// DBusWithResyncPeriod sets the resync period
func DBusWithResyncPeriod(d time.Duration) DBusMonitorOption {
	return func(m *DBusMonitor) {
		m.resyncPeriod = d
	}
}

// DBusWithReconnectDelay sets the delay before reconnecting after an error
func DBusWithReconnectDelay(d time.Duration) DBusMonitorOption {
	return func(m *DBusMonitor) {
		m.reconnectDelay = d
	}
}

// DBusWithLogger sets the logger
func DBusWithLogger(log *slog.Logger) DBusMonitorOption {
	return func(m *DBusMonitor) {
		m.log = log
	}
}

// DBusWithDebounceDuration sets the debounce duration for events
func DBusWithDebounceDuration(d time.Duration) DBusMonitorOption {
	return func(m *DBusMonitor) {
		m.debounceDuration = d
	}
}

// NewDBusMonitor creates a new USB monitor that uses D-Bus
func NewDBusMonitor(ctx context.Context, opts ...DBusMonitorOption) (Monitor, error) {
	devices, err := DiscoverPluggedUSBDevices()
	if err != nil {
		return nil, err
	}

	log := slog.With(slog.String("component", "dbus-usb-monitor"))

	m := &DBusMonitor{
		store:            NewUSBDeviceStore(devices, log),
		log:              log,
		resyncPeriod:     5 * time.Minute,
		reconnectDelay:   5 * time.Second,
		debounceDuration: 200 * time.Millisecond,
	}

	for _, opt := range opts {
		opt(m)
	}

	go m.run(ctx)

	return m, nil
}

func (m *DBusMonitor) run(ctx context.Context) {
	m.log.Info("D-Bus USB monitor started",
		slog.Duration("resync_period", m.resyncPeriod),
		slog.Duration("reconnect_delay", m.reconnectDelay),
	)

	for {
		select {
		case <-ctx.Done():
			m.log.Info("D-Bus USB monitor stopped")
			return
		default:
			if err := m.runMonitor(ctx); err != nil {
				m.log.Error("D-Bus monitor error, will reconnect",
					slog.String("error", err.Error()),
					slog.Duration("delay", m.reconnectDelay),
				)
			}

			select {
			case <-ctx.Done():
				return
			case <-time.After(m.reconnectDelay):
				// Retry connection
			}
		}
	}
}

func (m *DBusMonitor) runMonitor(ctx context.Context) error {
	conn, err := dbus.ConnectSystemBus(dbus.WithContext(ctx))
	if err != nil {
		return err
	}
	defer conn.Close()

	// Subscribe to UDisks2 ObjectManager signals
	rules := []string{
		"type='signal',interface='org.freedesktop.DBus.ObjectManager',member='InterfacesAdded'",
		"type='signal',interface='org.freedesktop.DBus.ObjectManager',member='InterfacesRemoved'",
	}

	for _, rule := range rules {
		call := conn.BusObject().Call("org.freedesktop.DBus.AddMatch", 0, rule)
		if call.Err != nil {
			m.log.Error("failed to add D-Bus match rule",
				slog.String("rule", rule),
				slog.Any("error", call.Err),
			)
			return call.Err
		}
	}

	signals := make(chan *dbus.Signal, 100)
	conn.Signal(signals)

	resyncTicker := time.NewTicker(m.resyncPeriod)
	defer resyncTicker.Stop()

	m.log.Debug("D-Bus signal listener started")

	for {
		select {
		case <-ctx.Done():
			return nil
		case signal, ok := <-signals:
			if !ok {
				return nil
			}
			m.handleSignal(signal)
		case <-resyncTicker.C:
			m.resync()
		}
	}
}

func (m *DBusMonitor) handleSignal(signal *dbus.Signal) {
	switch signal.Name {
	case "org.freedesktop.DBus.ObjectManager.InterfacesAdded":
		m.handleInterfacesAdded(signal)
	case "org.freedesktop.DBus.ObjectManager.InterfacesRemoved":
		m.handleInterfacesRemoved(signal)
	}
}

func (m *DBusMonitor) handleInterfacesAdded(signal *dbus.Signal) {
	if len(signal.Body) < 2 {
		return
	}

	path, ok := signal.Body[0].(dbus.ObjectPath)
	if !ok {
		return
	}

	interfaces, ok := signal.Body[1].(map[string]map[string]dbus.Variant)
	if !ok {
		return
	}

	if m.isUSBDevice(interfaces) {
		m.log.Debug("USB device connected via D-Bus", slog.String("path", string(path)))
		m.scheduleResync()
	}
}

func (m *DBusMonitor) handleInterfacesRemoved(signal *dbus.Signal) {
	if len(signal.Body) < 2 {
		return
	}

	path, ok := signal.Body[0].(dbus.ObjectPath)
	if !ok {
		return
	}

	interfaces, ok := signal.Body[1].([]string)
	if !ok {
		return
	}

	if m.containsUSBInterface(interfaces) {
		m.log.Debug("USB device disconnected via D-Bus", slog.String("path", string(path)))
		m.scheduleResync()
	}
}

// scheduleResync schedules a resync with debouncing to avoid multiple
// rapid resyncs when multiple events arrive in quick succession.
func (m *DBusMonitor) scheduleResync() {
	m.debounceMu.Lock()
	defer m.debounceMu.Unlock()

	// Cancel existing timer if any
	if m.debounceTimer != nil {
		m.debounceTimer.Stop()
	}

	m.debounceTimer = time.AfterFunc(m.debounceDuration, func() {
		m.resync()
	})
}

func (m *DBusMonitor) isUSBDevice(interfaces map[string]map[string]dbus.Variant) bool {
	if drive, ok := interfaces["org.freedesktop.UDisks2.Drive"]; ok {
		if connectionBus, ok := drive["ConnectionBus"]; ok {
			bus, _ := connectionBus.Value().(string)
			return bus == "usb"
		}
	}

	if block, ok := interfaces["org.freedesktop.UDisks2.Block"]; ok {
		if _, ok := block["Drive"]; ok {
			// If it has a link to drive, it can be USB
			return true
		}
	}

	return false
}

func (m *DBusMonitor) containsUSBInterface(interfaces []string) bool {
	for _, iface := range interfaces {
		if iface == "org.freedesktop.UDisks2.Drive" ||
			iface == "org.freedesktop.UDisks2.Block" {
			return true
		}
	}
	return false
}

func (m *DBusMonitor) resync() {
	devices, err := DiscoverPluggedUSBDevices()
	if err != nil {
		m.log.Error("failed to discover USB devices during resync",
			slog.String("error", err.Error()),
		)
		return
	}

	m.store.Resync(devices)
}

// GetDevices returns a copy of all discovered USB devices
func (m *DBusMonitor) GetDevices() []USBDevice {
	return m.store.GetDevices()
}

// GetDevice returns a device by path
func (m *DBusMonitor) GetDevice(path string) (*USBDevice, bool) {
	return m.store.GetDevice(path)
}

// GetDeviceByBusID returns a device by BusID
func (m *DBusMonitor) GetDeviceByBusID(busID string) (*USBDevice, bool) {
	return m.store.GetDeviceByBusID(busID)
}

// AddNotifier adds a notifier to be called on device changes
func (m *DBusMonitor) AddNotifier(notifier Notifier) {
	m.store.AddNotifier(notifier)
}

// RemoveNotifier removes a notifier
func (m *DBusMonitor) RemoveNotifier(notifier Notifier) {
	m.store.RemoveNotifier(notifier)
}
