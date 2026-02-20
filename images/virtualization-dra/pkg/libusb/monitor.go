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
	"fmt"
	"log/slog"
	"time"

	"github.com/spf13/pflag"
	"k8s.io/utils/ptr"
)

type Monitor interface {
	GetDevices() []USBDevice
	GetDevice(path string) (*USBDevice, bool)
	GetDeviceByBusID(busID string) (*USBDevice, bool)
	// DeviceChanges returns a channel that is sent on when the device list changes.
	// The channel is closed when the monitor is closed.
	DeviceChanges() <-chan struct{}
}

type MonitorType string

const (
	UdevMonitorType MonitorType = "udev"

	DefaultMonitorType = UdevMonitorType
)

func (m *MonitorType) String() string {
	return string(*m)
}

func (m *MonitorType) Set(s string) error {
	switch s {
	case ptr.To(UdevMonitorType).String():
		*m = UdevMonitorType
	default:
		return fmt.Errorf("invalid monitor type: %s", s)
	}
	return nil
}

func (m *MonitorType) Type() string {
	return "monitor-type"
}

type MonitorConfig struct {
	MonitorType MonitorType

	// ALL
	ResyncPeriod     time.Duration
	DebounceDuration time.Duration
	Logger           *slog.Logger

	// UDEV
	HostNetNs bool
}

// NewDefaultMonitorConfig creates a MonitorConfig with default values.
func NewDefaultMonitorConfig() *MonitorConfig {
	return &MonitorConfig{
		MonitorType:      DefaultMonitorType,
		ResyncPeriod:     5 * time.Minute,
		DebounceDuration: 200 * time.Millisecond,
	}
}

func (c *MonitorConfig) AddFlags(fs *pflag.FlagSet) {
	fs.Var(&c.MonitorType, "usb-monitor-type", fmt.Sprintf("USB monitor type: %s, (default %q)", UdevMonitorType, DefaultMonitorType))
	fs.DurationVar(&c.ResyncPeriod, "usb-monitor-resync-period", c.ResyncPeriod, "USB monitor resync period")
	fs.DurationVar(&c.DebounceDuration, "usb-monitor-debounce-duration", c.DebounceDuration, "USB monitor debounce duration")
	fs.BoolVar(&c.HostNetNs, "udev-usb-monitor-host-netns", c.HostNetNs, "UDEV USB monitor host netns")
}

func (c *MonitorConfig) Complete(ctx context.Context, logger *slog.Logger) (Monitor, error) {
	c.Logger = logger

	switch c.MonitorType {
	case UdevMonitorType:
		return NewUdevMonitor(ctx, c.makeUdevOpts()...)
	default:
		return nil, fmt.Errorf("unsupported monitor type: %s", c.MonitorType)
	}
}

func (c *MonitorConfig) makeUdevOpts() []UdevMonitorOption {
	var opts []UdevMonitorOption
	if c.ResyncPeriod > 0 {
		opts = append(opts, UdevWithResyncPeriod(c.ResyncPeriod))
	}
	if c.Logger != nil {
		opts = append(opts, UdevWithLogger(c.Logger))
	}
	if c.DebounceDuration > 0 {
		opts = append(opts, UdevWithDebounceDuration(c.DebounceDuration))
	}
	if c.HostNetNs {
		opts = append(opts, UdevWithHostNetNS())
	}

	return opts
}
