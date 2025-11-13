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
	"log/slog"
	"time"

	"github.com/godbus/dbus/v5"
)

type monitor struct {
	callback monitorCallback
	log      *slog.Logger
}

type monitorCallback struct {
	Add    func()
	Update func()
	Delete func()
}

func newUSBMonitor(callback monitorCallback) *monitor {
	return &monitor{
		callback: callback,
		log:      slog.With(slog.String("component", "usb-monitor")),
	}
}

func (m *monitor) Start(ctx context.Context) {
	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			case <-time.After(5 * time.Second):
				if err := m.run(ctx); err != nil {
					m.log.Error("failed to run monitor", slog.Any("err", err))
				}
			}
		}
	}()
}

func (m *monitor) run(ctx context.Context) error {
	conn, err := dbus.ConnectSystemBus(dbus.WithContext(ctx))
	if err != nil {
		return err
	}

	rules := []string{
		"type='signal',interface='org.freedesktop.DBus.ObjectManager',member='InterfacesAdded'",
		"type='signal',interface='org.freedesktop.DBus.ObjectManager',member='InterfacesRemoved'",
	}

	for _, rule := range rules {
		call := conn.BusObject().Call("org.freedesktop.DBus.AddMatch", 0, rule)
		if call.Err != nil {
			m.log.Error("Failed to add rule", slog.String("rule", rule), slog.Any("err", call.Err))
		}
	}

	signals := make(chan *dbus.Signal, 100)
	conn.Signal(signals)

	m.log.Info("Starting USB Monitor...")
	defer m.log.Info("Stopping USB Monitor...")

	for {
		select {
		case <-ctx.Done():
			return nil
		case signal := <-signals:
			m.handleSignal(signal)
		}
	}
}

func (m *monitor) handleSignal(signal *dbus.Signal) {
	switch signal.Name {
	case "org.freedesktop.DBus.ObjectManager.InterfacesAdded":
		if len(signal.Body) >= 2 {
			path, ok := signal.Body[0].(dbus.ObjectPath)
			if !ok {
				return
			}
			interfaces, ok := signal.Body[1].(map[string]map[string]dbus.Variant)
			if !ok {
				return
			}

			if m.isUSBDevice(interfaces) {
				slog.Info("USB Device connected", slog.String("path", string(path)))
				if m.callback.Add != nil {
					m.callback.Add()
				}
			}
		}

	case "org.freedesktop.DBus.ObjectManager.InterfacesRemoved":
		// Sender = {string} ":1.10"
		// Path = {dbus.ObjectPath} "/org/freedesktop/UDisks2"
		// Name = {string} "org.freedesktop.DBus.ObjectManager.InterfacesRemoved"
		// Body = {[]interface{}}
		// Body[0] = interface{} | dbus.ObjectPath "/org/freedesktop/UDisks2/block_devices/sda2"
		// Body[1] = interface{} | []string{"org.freedesktop.UDisks2.Filesystem","org.freedesktop.UDisks2.Partition","org.freedesktop.UDisks2.Block","org.freedesktop.UDisks2.Drive"}

		if len(signal.Body) >= 2 {
			path, ok := signal.Body[0].(dbus.ObjectPath)
			if !ok {
				return
			}
			interfaces, ok := signal.Body[1].([]string)
			if !ok {
				return
			}

			if m.containsUSBInterface(interfaces) {
				slog.Info("USB Device disconnected", slog.String("path", string(path)))
				if m.callback.Delete != nil {
					m.callback.Delete()
				}
			}
		}
	}
}

func (m *monitor) isUSBDevice(interfaces map[string]map[string]dbus.Variant) bool {
	if drive, ok := interfaces["org.freedesktop.UDisks2.Drive"]; ok {
		if connectionBus, ok := drive["ConnectionBus"]; ok {
			bus, _ := connectionBus.Value().(string)
			return bus == "usb"
		}
	}

	if block, ok := interfaces["org.freedesktop.UDisks2.Block"]; ok {
		if _, ok := block["Drive"]; ok {
			// if it has a link to drive, it can be USB
			return true
		}
	}

	return false
}

func (m *monitor) containsUSBInterface(interfaces []string) bool {
	for _, iface := range interfaces {
		if iface == "org.freedesktop.UDisks2.Drive" ||
			iface == "org.freedesktop.UDisks2.Block" {
			return true
		}
	}
	return false
}
