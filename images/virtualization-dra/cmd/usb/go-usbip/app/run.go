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

package app

import (
	"fmt"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"

	"github.com/deckhouse/virtualization-dra/pkg/libusb"
	"github.com/deckhouse/virtualization-dra/pkg/usbip"
)

func NewRunCommand() *cobra.Command {
	o := &runOptions{
		usbipdConfig: &usbip.USBIPDConfig{},
		monitor:      libusb.NewDefaultMonitorConfig(),
	}
	cmd := &cobra.Command{
		Use:     "run",
		Short:   "Run USBIP server",
		Example: o.Usage(),
		RunE:    o.Run,
		Args:    cobra.NoArgs,
	}

	o.AddFlags(cmd.Flags())

	return cmd
}

type runOptions struct {
	usbipdConfig *usbip.USBIPDConfig
	monitor      *libusb.MonitorConfig
}

func (o *runOptions) Usage() string {
	return `  # Run USBIP server
  $ go-usbip run
`
}

func (o *runOptions) AddFlags(fs *pflag.FlagSet) {
	o.usbipdConfig.AddFlags(fs)
	o.monitor.AddFlags(fs)
}

func (o *runOptions) Run(cmd *cobra.Command, _ []string) error {
	monitor, err := o.monitor.Complete(cmd.Context(), nil)
	if err != nil {
		return fmt.Errorf("failed to create usb monitor: %w", err)
	}

	usbipd, err := o.usbipdConfig.Complete(monitor)
	if err != nil {
		return fmt.Errorf("failed to create usbipd: %w", err)
	}

	err = usbipd.Run(cmd.Context())
	if err != nil {
		return fmt.Errorf("failed to run usbipd: %w", err)
	}

	return nil
}
