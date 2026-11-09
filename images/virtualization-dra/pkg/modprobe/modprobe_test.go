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

package modprobe

import (
	"bytes"
	"compress/gzip"
	"io"
	"os"
	"path/filepath"

	"github.com/klauspost/compress/zstd"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/ulikunitz/xz"
)

var _ = Describe("uncompressModuleToTmp", func() {
	payload := []byte("usbip-host module body")

	DescribeTable("restores the module from a compressed file",
		func(ext string, newWriter func(io.Writer) (io.WriteCloser, error)) {
			var buf bytes.Buffer
			w, err := newWriter(&buf)
			Expect(err).NotTo(HaveOccurred())
			_, err = w.Write(payload)
			Expect(err).NotTo(HaveOccurred())
			Expect(w.Close()).To(Succeed())

			path := filepath.Join(GinkgoT().TempDir(), "usbip-host.ko"+ext)
			Expect(os.WriteFile(path, buf.Bytes(), 0o644)).To(Succeed())
			Expect(isCompressed(path)).To(BeTrue())

			out, err := uncompressModuleToTmp(path)
			Expect(err).NotTo(HaveOccurred())
			DeferCleanup(os.Remove, out)

			Expect(os.ReadFile(out)).To(Equal(payload))
		},
		Entry("gzip", ".gz", func(w io.Writer) (io.WriteCloser, error) { return gzip.NewWriter(w), nil }),
		Entry("xz", ".xz", func(w io.Writer) (io.WriteCloser, error) { return xz.NewWriter(w) }),
		Entry("zstd", ".zst", func(w io.Writer) (io.WriteCloser, error) { return zstd.NewWriter(w) }),
	)

	It("fails on a file that is not in the format its suffix names and leaves no temporary file", func() {
		path := filepath.Join(GinkgoT().TempDir(), "usbip-host.ko.xz")
		Expect(os.WriteFile(path, payload, 0o644)).To(Succeed())

		tmpDir := GinkgoT().TempDir()
		GinkgoT().Setenv("TMPDIR", tmpDir)

		_, err := uncompressModuleToTmp(path)
		Expect(err).To(HaveOccurred())
		Expect(os.ReadDir(tmpDir)).To(BeEmpty())
	})

	It("treats a plain .ko as not compressed", func() {
		Expect(isCompressed("usbip-host.ko")).To(BeFalse())
	})
})
