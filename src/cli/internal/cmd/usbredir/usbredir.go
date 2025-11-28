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

package usbredir

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"

	"github.com/deckhouse/virtualization/src/cli/internal/clientconfig"
	"github.com/deckhouse/virtualization/src/cli/internal/templates"
)

const usbRedirectClient string = "usbredirect"

func NewCommand() *cobra.Command {
	usbRedir := &USBRedir{}
	cmd := &cobra.Command{
		Use:     "usb-redirect (VirtualMachine)",
		Short:   "Redirect USB devices to a virtual machine.",
		Example: usbRedir.Usage(),
		Args:    templates.ExactArgs("usb-redirect", 1),
		RunE:    usbRedir.Run,
	}

	cmd.SetUsageTemplate(templates.UsageTemplate())
	usbRedir.AddFlags(cmd.Flags())

	_ = cmd.MarkFlagRequired("device")
	_ = cmd.MarkFlagRequired("bus")

	return cmd
}

type USBRedir struct {
	device         int
	bus            int
	redirVerbosity int
	tool           bool
	toolSudo       bool
}

func (c *USBRedir) AddFlags(fs *pflag.FlagSet) {
	fs.IntVarP(&c.device, "device", "d", 0, "(required) The device you want to redirect.")
	fs.IntVarP(&c.bus, "bus", "b", 0, "(required) The bus of the device you want to redirect.")
	fs.IntVar(&c.redirVerbosity, "verbosity", 1, "(optional) Verbosity level of the usbredirect.")
	fs.BoolVar(&c.tool, "tool", false, "(optional) Use subtool usbredirect.")
	fs.BoolVar(&c.toolSudo, "sudo", false, "(optional) Use sudo to run the usbredirect subtool.")
}

func (c *USBRedir) Validate() error {
	if c.device == 0 {
		return fmt.Errorf("device is required")
	}
	if c.bus == 0 {
		return fmt.Errorf("bus is required")
	}
	if c.tool && !c.toolSudo && os.Getuid() != 0 {
		return fmt.Errorf("sudo is required to run the command as root")
	}
	return nil
}

func (c *USBRedir) Usage() string {
	return `# Find the device you want to redirect (linux):
	❯ lsusb | grep Transcend
	Bus 004 Device 003: ID 8564:1000 Transcend Information, Inc. JetFlash

	# Redirect it with bus-device:
    {{ProgramName}} usbredir myvm --bus 4 --device 3
	`
}

func (c *USBRedir) newRedirector() Redirector {
	if c.tool {
		return newUsbToolRedirector(c.bus, c.device, c.redirVerbosity, usbRedirectClient, c.toolSudo)
	}
	return newNativeUsbRedirector(c.bus, c.device, c.redirVerbosity)
}

func (c *USBRedir) Run(cmd *cobra.Command, args []string) error {
	if err := c.Validate(); err != nil {
		return err
	}

	client, defaultNamespace, _, err := clientconfig.ClientAndNamespaceFromContext(cmd.Context())
	if err != nil {
		return err
	}

	namespace, name, err := templates.ParseTarget(args[0])
	if err != nil {
		return err
	}
	if namespace == "" {
		namespace = defaultNamespace
	}

	stream, err := client.VirtualMachines(namespace).USBRedir(cmd.Context(), name)
	if err != nil {
		return err
	}

	redir := NewClient(stream, c.newRedirector())

	return redir.Redirect(cmd.Context())
}
