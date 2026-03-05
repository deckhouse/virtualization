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
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"

	"github.com/deckhouse/virtualization-dra/pkg/usbip"
)

func NewUnExportCommand() *cobra.Command {
	o := &unExportOptions{}
	cmd := &cobra.Command{
		Use:     "unexport [:host:] [:busID:]",
		Short:   "UnExport USB device on USBIP server",
		Example: o.Usage(),
		RunE:    o.Run,
		Args:    cobra.ExactArgs(2),
	}

	o.AddFlags(cmd.Flags())

	return cmd
}

type unExportOptions struct {
	port int
}

func (o *unExportOptions) Usage() string {
	return `  # UnExport USB devices on USBIP server
  $ go-usbip unexport 192.168.1.1 3-1
`
}

func (o *unExportOptions) AddFlags(fs *pflag.FlagSet) {
	fs.IntVar(&o.port, "port", 3240, "Remote port for unexporting")
}

func (o *unExportOptions) Run(_ *cobra.Command, args []string) error {
	host := args[0]
	busID := args[1]

	return usbip.NewUSBExporter().Unexport(host, busID, o.port)
}
