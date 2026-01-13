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

package modprobe

import (
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"

	"golang.org/x/sys/unix"
)

func LoadModules(modules []string) error {
	var uts unix.Utsname
	if err := unix.Uname(&uts); err != nil {
		return fmt.Errorf("uname: %w", err)
	}

	kernel := unix.ByteSliceToString(uts.Release[:])
	base := filepath.Join("/lib/modules", kernel)

	for _, m := range modules {
		path := filepath.Join(base, m)
		if err := loadModule(path); err != nil {
			return fmt.Errorf("load module %s: %w", path, err)
		}
	}

	return nil
}

func loadModule(path string) error {
	f, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("open %s: %w", path, err)
	}
	defer f.Close()

	if err = unix.FinitModule(int(f.Fd()), "", 0); err != nil {
		if errors.Is(err, unix.EEXIST) {
			slog.Info("Module already loaded", slog.String("path", path))
			return nil
		}
		return fmt.Errorf("finit_module %s: %w", path, err)
	}

	slog.Info("Module loaded", slog.String("path", path))

	return nil
}
