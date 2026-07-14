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
	"compress/gzip"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"github.com/klauspost/compress/zstd"
	"github.com/ulikunitz/xz"
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
	if isCompressed(path) {
		uncompressedPath, err := uncompressModuleToTmp(path)
		if err != nil {
			return fmt.Errorf("uncompress module %s: %w", path, err)
		}
		defer func() {
			if err := os.Remove(uncompressedPath); err != nil {
				slog.Error("remove uncompressed module", "path", uncompressedPath, "err", err)
			}
		}()
		path = uncompressedPath
	}

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

func uncompressModuleToTmp(path string) (string, error) {
	pattern := filepath.Base(path) + "-*"
	uncompress, err := os.CreateTemp("", pattern)
	if err != nil {
		return "", err
	}
	defer uncompress.Close()

	in, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer in.Close()

	decoder, err := newDecompressor(path, in)
	if err != nil {
		return "", err
	}
	if closer, ok := decoder.(io.Closer); ok {
		defer func() { _ = closer.Close() }()
	}

	if _, err := io.Copy(uncompress, decoder); err != nil {
		return "", err
	}

	return uncompress.Name(), nil
}

// isCompressed reports whether the module is stored in one of the compression
// formats the kernel supports for modules (MODULE_COMPRESS_{GZIP,XZ,ZSTD}).
func isCompressed(path string) bool {
	return strings.HasSuffix(path, ".zst") ||
		strings.HasSuffix(path, ".xz") ||
		strings.HasSuffix(path, ".gz")
}

func newDecompressor(path string, in io.Reader) (io.Reader, error) {
	switch {
	case strings.HasSuffix(path, ".zst"):
		return zstd.NewReader(in)
	case strings.HasSuffix(path, ".xz"):
		return xz.NewReader(in)
	case strings.HasSuffix(path, ".gz"):
		return gzip.NewReader(in)
	default:
		return nil, fmt.Errorf("unsupported compression format: %s", path)
	}
}

func KernelRelease() (string, error) {
	var uts unix.Utsname
	if err := unix.Uname(&uts); err != nil {
		return "", fmt.Errorf("uname: %w", err)
	}
	return unix.ByteSliceToString(uts.Release[:]), nil
}
