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
	"log/slog"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/deckhouse/virtualization-dra/pkg/logger"
	"github.com/deckhouse/virtualization-dra/pkg/modprobe"
)

func NewInitCommand() *cobra.Command {
	o := &initOptions{
		logging: &logger.Options{},
	}

	cmd := &cobra.Command{
		Use:   "init",
		Short: "Init USB gateway",
		PreRun: func(_ *cobra.Command, _ []string) {
			o.Complete()
		},
		RunE: o.Run,
	}

	return cmd
}

type initOptions struct {
	logging *logger.Options
}

func (o *initOptions) Complete() {
	log := o.logging.Complete()
	logger.SetDefaultLogger(log)
}

func (o *initOptions) Run(_ *cobra.Command, _ []string) error {
	kernelRelease, err := modprobe.KernelRelease()
	if err != nil {
		return fmt.Errorf("failed to get kernel release: %w", err)
	}

	slog.Info("Detected kernel release", slog.String("release", kernelRelease))

	modules := []string{
		filepath.Join("/lib/modules", kernelRelease, "kernel/drivers/usb/usbip/usbip-core.ko"),
		filepath.Join("/lib/modules", kernelRelease, "kernel/drivers/usb/usbip/usbip-host.ko"),
		filepath.Join("/lib/modules", kernelRelease, "kernel/drivers/usb/usbip/vhci-hcd.ko"),
	}

	zstSupported, err := modprobe.KernelSupportsZst(kernelRelease)
	if err != nil {
		return fmt.Errorf("failed to check kernel support for zst: %w", err)
	}
	if zstSupported {
		for i := range modules {
			modules[i] += ".zst"
		}
	}

	slog.Info("Loading modules", slog.Any("modules", modules))

	if err := modprobe.LoadModules(modules...); err != nil {
		return fmt.Errorf("failed to load modules: %w", err)
	}

	slog.Info("Modules loaded successfully")

	return nil
}
