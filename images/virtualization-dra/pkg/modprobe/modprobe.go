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
	"strconv"
	"strings"

	"golang.org/x/sys/unix"
)

func LoadModules(modules ...string) error {
	for _, module := range modules {
		if err := loadModule(module); err != nil {
			return fmt.Errorf("load module %s: %w", module, err)
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

func KernelRelease() (string, error) {
	var uts unix.Utsname
	if err := unix.Uname(&uts); err != nil {
		return "", fmt.Errorf("uname: %w", err)
	}
	return unix.ByteSliceToString(uts.Release[:]), nil

}

func KernelSupportsZst(release string) bool {
	parts := strings.Split(release, ".")
	if len(parts) < 2 {
		return false
	}

	major, err1 := strconv.Atoi(parts[0])
	minor, err2 := strconv.Atoi(parts[1])
	if err1 != nil || err2 != nil {
		return false
	}

	// ZST is supported since 5.16
	if major > 5 {
		return true
	}
	if major == 5 && minor >= 16 {
		return true
	}
	return false
}
