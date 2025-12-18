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

package main

import (
	"context"
	"encoding/json"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"

	"github.com/deckhouse/virtualization-dra/pkg/libusb"
	"github.com/deckhouse/virtualization-dra/pkg/logger"
)

func main() {
	ctx, _ := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)

	if err := NewUSBMonitorCommand().ExecuteContext(ctx); err != nil {
		slog.Error("failed to execute command", slog.Any("err", err))
		os.Exit(1)
	}
}

func NewUSBMonitorCommand() *cobra.Command {
	o := &options{
		monitor: libusb.NewDefaultMonitorConfig(),
		logging: &logger.Options{},
	}

	cmd := &cobra.Command{
		Use:           "usb-monitor",
		Short:         "USB monitor",
		SilenceUsage:  true,
		SilenceErrors: true,
		PreRun: func(cmd *cobra.Command, args []string) {
			o.Complete()
		},
		RunE: o.Run,
	}

	o.AddFlags(cmd.Flags())

	return cmd
}

type options struct {
	monitor *libusb.MonitorConfig
	logging *logger.Options
}

func (o *options) Complete() {
	log := o.logging.Complete()
	logger.SetDefaultLogger(log)
}

func (o *options) AddFlags(fs *pflag.FlagSet) {
	o.monitor.AddFlags(fs)
	o.logging.AddFlags(fs)
}

func (o *options) Run(cmd *cobra.Command, _ []string) error {
	monitor, err := o.monitor.Complete(cmd.Context(), nil)
	if err != nil {
		return err
	}

	changes := monitor.DeviceChanges()
	for {
		select {
		case <-cmd.Context().Done():
			return nil
		case _, ok := <-changes:
			if !ok {
				return nil
			}
			slog.Info("USB devices changed")
			devices := monitor.GetDevices()
			b, err := json.Marshal(devices)
			if err != nil {
				slog.Error("failed to marshal devices", slog.Any("err", err))
				continue
			}
			cmd.Println(string(b))
		}
	}
}
