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

	"github.com/deckhouse/virtualization-dra/internal/usbip"
)

func NewUsedPortsCommand() *cobra.Command {
	o := &usedPortsOptions{}
	cmd := &cobra.Command{
		Use:     "ports",
		Short:   "List used ports",
		Example: o.Usage(),
		RunE:    o.Run,
	}

	return cmd
}

type usedPortsOptions struct{}

func (o *usedPortsOptions) Usage() string {
	return `  # List used ports
  $ go-usbip ports
`
}

func (o *usedPortsOptions) Run(cmd *cobra.Command, _ []string) error {
	ports, err := usbip.NewUSBAttacher().GetUsedPorts()
	if err != nil {
		return err
	}

	cmd.Println("Used ports:")
	for _, port := range ports {
		cmd.Println(fmt.Sprintf("- %d", port))
	}

	return nil
}
