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

	"github.com/deckhouse/virtualization-dra/pkg/usbip"
)

func NewAttachInfoCommand() *cobra.Command {
	o := &attachInfoOptions{}
	cmd := &cobra.Command{
		Use:     "attach-info",
		Short:   "Get attach info",
		Example: o.Usage(),
		RunE:    o.Run,
	}

	return cmd
}

type attachInfoOptions struct{}

func (o *attachInfoOptions) Usage() string {
	return `  # Get attach info
  $ go-usbip attach-info
`
}

func (o *attachInfoOptions) Run(cmd *cobra.Command, _ []string) error {
	infos, err := usbip.NewUSBAttacher().GetAttachInfo()
	if err != nil {
		return err
	}

	return printer.PrintObject(cmd, infos)
}
