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

	"github.com/deckhouse/virtualization-dra/pkg/modprobe"
)

func NewInitCommand() *cobra.Command {
	o := &initOptions{}

	cmd := &cobra.Command{
		Use:   "init",
		Short: "Init USB gateway",
		RunE:  o.Run,
	}

	return cmd
}

type initOptions struct{}

func (o *initOptions) Run(_ *cobra.Command, _ []string) error {
	modules := []string{
		"kernel/drivers/usb/usbip/usbip-core.ko",
		"kernel/drivers/usb/usbip/vhci-hcd.ko",
	}

	if err := modprobe.LoadModules(modules); err != nil {
		return fmt.Errorf("failed to load modules: %w", err)
	}

	return nil
}
