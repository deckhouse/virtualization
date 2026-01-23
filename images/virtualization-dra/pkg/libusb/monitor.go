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
	AddNotifier(notifier Notifier)
	RemoveNotifier(notifier Notifier)
}

type Notifier interface {
	Notify()
}
type FuncNotifier func()

func (f FuncNotifier) Notify() {
	f()
}

type MonitorType string

const (
	UdevMonitorType MonitorType = "udev"
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
	ResyncPeriod time.Duration
	Logger       *slog.Logger

	// UDEV
	DebounceDuration time.Duration
	HostNetNs        bool
}

func (c *MonitorConfig) AddFlags(fs *pflag.FlagSet) {
	fs.Var(&c.MonitorType, "usb-monitor-type", "USB monitor type")
	fs.DurationVar(&c.ResyncPeriod, "usb-monitor-resync-period", c.ResyncPeriod, "USB monitor resync period")
	fs.DurationVar(&c.DebounceDuration, "udev-usb-monitor-debounce-duration", c.DebounceDuration, "UDEV USB monitor debounce duration")
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
