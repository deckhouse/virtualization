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
	"os/exec"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"

	"github.com/deckhouse/virtualization/src/cli/internal/clientconfig"
	"github.com/deckhouse/virtualization/src/cli/internal/templates"
)

const usbRedirectClient string = "usbredirect"

func NewCommand() *cobra.Command {
	usbRedir := &USBRedir{}
	cmd := &cobra.Command{
		Use:     "usbredir (VirtualMachine)",
		Short:   "Redirect USB devices to a virtual machine.",
		Example: usbRedir.Usage(),
		Args:    templates.ExactArgs("usbredir", 1),
		RunE:    usbRedir.Run,
	}

	cmd.SetUsageTemplate(templates.UsageTemplate())
	usbRedir.AddFlags(cmd.Flags())

	_ = cmd.MarkFlagRequired("device")
	_ = cmd.MarkFlagRequired("bus")

	return cmd
}

type USBRedir struct {
	device string
	bus    string
	sudo   bool
}

func (c *USBRedir) AddFlags(fs *pflag.FlagSet) {
	fs.StringVarP(&c.device, "device", "d", "", "(required) The device you want to redirect.")
	fs.StringVarP(&c.bus, "bus", "b", "", "(required) The bus of the device you want to redirect.")
	fs.BoolVar(&c.sudo, "sudo", false, "(optional) Use sudo to run the usbredirect command.")
}

func (c *USBRedir) Validate() error {
	if c.device == "" {
		return fmt.Errorf("device is required")
	}
	if c.bus == "" {
		return fmt.Errorf("bus is required")
	}
	if !c.sudo && os.Getuid() != 0 {
		return fmt.Errorf("sudo is required to run the command as root")
	}
	return nil
}

func (c *USBRedir) Usage() string {
	return `# Find the device you want to redirect (linux):
	❯ lsusb | grep Transcend
	Bus 004 Device 003: ID 8564:1000 Transcend Information, Inc. JetFlash

	# Redirect it with bus-device:
    {{ProgramName}} usbredir myvm --bus 004 --device 003
	`
}

func (c *USBRedir) Run(cmd *cobra.Command, args []string) error {
	if err := c.Validate(); err != nil {
		return err
	}

	if _, err := exec.LookPath(usbRedirectClient); err != nil {
		return fmt.Errorf("error on finding %s in $PATH: %s", usbRedirectClient, err.Error())
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

	redir := NewClient(stream, newUsbRedirector(usbRedirectClient, c.sudo))
	device := fmt.Sprintf("%s-%s", c.bus, c.device)

	return redir.Redirect(cmd.Context(), device)
}
