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

package usbip

import (
	"net"
	"os"
	"strconv"
	"time"

	"github.com/spf13/pflag"

	"github.com/deckhouse/virtualization-dra/pkg/libusb"
)

type USBIPDConfig struct {
	Address                 string
	Port                    int
	GracefulShutdownTimeout time.Duration
	MaxTcpConnections       int
	ExportEnabled           bool
}

func (c *USBIPDConfig) AddFlags(fs *pflag.FlagSet) {
	fs.StringVar(&c.Address, "usbipd-address", os.Getenv("USBIPD_ADDRESS"), "USBIPD address")
	fs.IntVar(&c.Port, "usbipd-port", 3240, "USBIPD port")
	fs.DurationVar(&c.GracefulShutdownTimeout, "usbipd-graceful-shutdown-timeout", 0, "USBIPD graceful shutdown timeout")
	fs.IntVar(&c.MaxTcpConnections, "usbipd-max-tcp-connections", 0, "USBIPD max TCP connections")
	fs.BoolVar(&c.ExportEnabled, "usbipd-export-enabled", false, "USBIPD export enabled")
}

func (c *USBIPDConfig) Complete(monitor libusb.Monitor) (*USBIPD, error) {
	var opts []Option
	if c.GracefulShutdownTimeout > 0 {
		opts = append(opts, WithGracefulShutdownTimeout(c.GracefulShutdownTimeout))
	}
	if c.MaxTcpConnections > 0 {
		opts = append(opts, WithMaxTCPConnection(c.MaxTcpConnections))
	}
	if c.ExportEnabled {
		opts = append(opts, WithExport(true))
	}

	address := net.JoinHostPort(c.Address, strconv.Itoa(c.Port))

	return NewUSBIPD(address, monitor, opts...), nil
}
