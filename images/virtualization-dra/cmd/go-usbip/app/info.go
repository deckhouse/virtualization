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

package app

import (
	"cmp"
	"slices"

	"github.com/spf13/cobra"

	"github.com/deckhouse/virtualization-dra/pkg/libusb"
)

func NewInfoCommand() *cobra.Command {
	o := &infoOptions{}
	cmd := &cobra.Command{
		Use:     "info",
		Short:   "Get info",
		Example: o.Usage(),
		RunE:    o.Run,
	}

	return cmd
}

type infoOptions struct{}

func (o *infoOptions) Usage() string {
	return `  # Get info
  $ go-usbip info
`
}

func (o *infoOptions) Run(cmd *cobra.Command, _ []string) error {
	discoverDevices, err := libusb.DiscoverPluggedUSBDevices()
	if err != nil {
		return err
	}

	devices := make([]*libusb.USBDevice, 0, len(discoverDevices))

	for _, device := range discoverDevices {
		devices = append(devices, device)
	}

	slices.SortFunc(devices, func(a, b *libusb.USBDevice) int {
		return cmp.Compare(a.Path, b.Path)
	})

	return printer.PrintObject(cmd, devices)
}
