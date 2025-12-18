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
	"context"
	"time"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"

	"github.com/deckhouse/virtualization-dra/internal/usbip"
	"github.com/deckhouse/virtualization-dra/pkg/usb"
)

func NewRunCommand() *cobra.Command {
	o := &runOptions{}
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
	port         int
	resyncPeriod time.Duration
}

func (o *runOptions) Usage() string {
	return `  # Run USBIP server
  $ go-usbip run
`
}

func (o *runOptions) AddFlags(fs *pflag.FlagSet) {
	fs.IntVar(&o.port, "port", 3240, "Port to listen on")
	fs.DurationVar(&o.resyncPeriod, "resync-period", time.Second*300, "Resync period")
}

func (o *runOptions) Run(cmd *cobra.Command, _ []string) error {
	monitor, err := usb.NewMonitor(context.Background(), o.resyncPeriod)
	if err != nil {
		return err
	}

	config := usbip.USBIPDConfig{
		Port:    o.port,
		Monitor: monitor,
	}
	err = config.Validate()
	if err != nil {
		return err
	}

	usbipd, err := config.Complete()
	if err != nil {
		return err
	}

	err = usbipd.Start(cmd.Context())
	if err != nil {
		return err
	}

	<-cmd.Context().Done()

	return nil
}
