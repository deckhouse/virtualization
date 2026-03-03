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

func NewAttachInfoCommand() *cobra.Command {
	o := &attachInfoOptions{}
	cmd := &cobra.Command{
		Use:     "attach-info",
		Short:   "Get attach info",
		Example: o.Usage(),
		RunE:    o.Run,
	}

	o.AddFlags(cmd.Flags())

	return cmd
}

type attachInfoOptions struct {
	watch bool
}

func (o *attachInfoOptions) Usage() string {
	return `  # Get attach info
  $ go-usbip attach-info
`
}

func (o *attachInfoOptions) AddFlags(fs *pflag.FlagSet) {
	fs.BoolVarP(&o.watch, "watch", "w", false, "Watch attach info")
}

func (o *attachInfoOptions) Run(cmd *cobra.Command, _ []string) error {
	if o.watch {
		return o.handleWatch(cmd)
	}
	return o.handleGet(cmd)
}

func (o *attachInfoOptions) handleGet(cmd *cobra.Command) error {
	info, err := usbip.NewUSBAttacher().GetAttachInfo()
	if err != nil {
		return err
	}

	return printer.PrintObject(cmd, info)
}

func (o *attachInfoOptions) handleWatch(cmd *cobra.Command) error {
	ch, err := usbip.NewUSBAttacher().WatchAttachInfo(cmd.Context())
	if err != nil {
		return err
	}

	for info := range ch {
		if err := printer.PrintObject(cmd, info); err != nil {
			return err
		}
	}

	return nil
}
