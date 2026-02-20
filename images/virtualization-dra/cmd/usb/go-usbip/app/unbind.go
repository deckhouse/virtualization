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
	"github.com/spf13/cobra"

	"github.com/deckhouse/virtualization-dra/pkg/usbip"
)

func NewUnbindCommand() *cobra.Command {
	o := &unbindOptions{}
	cmd := &cobra.Command{
		Use:     "unbind [:busID:]",
		Short:   "Unbind USB devices from USBIP server",
		Example: o.Usage(),
		RunE:    o.Run,
		Args:    cobra.ExactArgs(1),
	}

	return cmd
}

type unbindOptions struct{}

func (o *unbindOptions) Usage() string {
	return `  # Unbind USB devices from USBIP server
  $ go-usbip unbind 3-1
`
}

func (o *unbindOptions) Run(_ *cobra.Command, args []string) error {
	busID := args[0]
	return usbip.NewUSBBinder().Unbind(busID)
}
