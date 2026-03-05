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
	"os"
	"path/filepath"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"

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

	o.AddFlags(cmd.Flags())

	return cmd
}

type initOptions struct {
	logging            *logger.Options
	customBuildPath    string
	tryCustomBuildPath string
}

func (o *initOptions) AddFlags(fs *pflag.FlagSet) {
	o.logging.AddFlags(fs)
	fs.StringVar(&o.customBuildPath, "custom-build-path", "", "Custom build path")
	fs.StringVar(&o.tryCustomBuildPath, "try-custom-build-path", "", "Try custom build path")
}

func (o *initOptions) Complete() {
	log := o.logging.Complete()
	logger.SetDefaultLogger(log)
}

func (o *initOptions) Run(_ *cobra.Command, _ []string) error {
	if o.customBuildPath != "" {
		return o.loadCustomBuildPath(o.tryCustomBuildPath)
	}

	if o.tryCustomBuildPath != "" {
		err := o.loadCustomBuildPath(o.tryCustomBuildPath)
		if err == nil {
			return nil
		}

		slog.Error("failed to load modules from custom build path", slog.String("path", o.tryCustomBuildPath), slog.Any("error", err))
		slog.Info("Trying to load pre-installed modules")
	}

	return o.loadPreInstalled()
}

func (o *initOptions) loadCustomBuildPath(path string) error {
	modules := []string{
		filepath.Join(path, "usbip-core.ko"),
		filepath.Join(path, "usbip-host.ko"),
		filepath.Join(path, "vhci-hcd.ko"),
	}
	slog.Info("Loading modules", slog.Any("modules", modules))

	if err := modprobe.LoadModules(modules...); err != nil {
		return fmt.Errorf("failed to load modules: %w", err)
	}

	slog.Info("Modules loaded successfully")
	return nil
}

func (o *initOptions) loadPreInstalled() error {
	kernelRelease, err := modprobe.KernelRelease()
	if err != nil {
		return fmt.Errorf("failed to get kernel release: %w", err)
	}

	modules, err := o.getPreInstalledModules(kernelRelease)
	if err != nil {
		return fmt.Errorf("failed to get pre-installed modules: %w", err)
	}

	slog.Info("Loading modules", slog.Any("modules", modules))

	if err := modprobe.LoadModules(modules...); err != nil {
		return fmt.Errorf("failed to load modules: %w", err)
	}

	slog.Info("Modules loaded successfully")
	return nil
}

func (o *initOptions) getPreInstalledModules(kernelRelease string) ([]string, error) {
	modules := []string{
		filepath.Join("/lib/modules", kernelRelease, "kernel/drivers/usb/usbip/usbip-core.ko"),
		filepath.Join("/lib/modules", kernelRelease, "kernel/drivers/usb/usbip/usbip-host.ko"),
		filepath.Join("/lib/modules", kernelRelease, "kernel/drivers/usb/usbip/vhci-hcd.ko"),
	}

	for i, m := range modules {
		exists, err := fileExists(m)
		if err != nil {
			return nil, err
		}

		if !exists {
			m += ".zst"
			exists, err = fileExists(m)
			if err != nil {
				return nil, err
			}
			if !exists {
				return nil, fmt.Errorf("module %s not found", m)
			}
			modules[i] = m
		}
	}

	return modules, nil
}

func fileExists(path string) (bool, error) {
	_, err := os.Stat(path)
	if err == nil {
		return true, nil
	}
	if os.IsNotExist(err) {
		return false, nil
	}
	return false, err
}
