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

package importer

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"encoding/binary"
	"io"
	"math"
	"path/filepath"
	"strings"
	"testing"

	"github.com/klauspost/compress/zstd"
	"github.com/ulikunitz/xz"

	metrics "github.com/deckhouse/virtualization/images/pvc-artifact/pkg/monitoring/metrics/pvc-importer"
)

// Seeds carrying non-ASCII text are written as \x byte escapes rather than as
// literal characters, so this file stays ASCII-only and the CI linters that
// reject non-ASCII source have nothing to flag. The escapes encode the same
// UTF-8 bytes; do not "simplify" them back into literals.

const (
	// fuzzMaxInput is a technical cap only: bigger payloads add nothing to the
	// header and path parsing under test but cost the fuzzer memory and time.
	fuzzMaxInput = 64 << 10

	// fuzzMaxDrain bounds how much the reader stack is drained per iteration, so
	// that a compression bomb ends the iteration instead of the worker.
	fuzzMaxDrain = 1 << 20

	// fuzzScratchDir stands in for the scratch space the layer is unpacked into.
	// safeJoinPaths does no I/O, so no directory has to exist.
	fuzzScratchDir = "/scratch/disk"

	// fuzzImageDir is the path prefix processLayer looks for inside a layer.
	fuzzImageDir = "disk"
)

// FuzzFormatReaders drives the header and compression detection of
// format-readers.go over raw bytes. The payload is a container image layer or
// the disk file inside it, both of which come from the registry the import
// points at, so every byte is attacker controlled. Rejecting a payload is the
// normal outcome; a panic, a hang or a format verdict that contradicts the
// bytes is the finding.
func FuzzFormatReaders(f *testing.F) {
	var (
		qcow2V2 = qcow2Header(2, 64<<20)
		qcow2V3 = qcow2Header(3, 64<<20)
	)

	// Formats the detection is supposed to recognize, each padded to the 512
	// bytes the reader stack reads in one go.
	f.Add(qcow2V2)
	f.Add(qcow2V3)
	f.Add(qcow2Header(0, 0))
	f.Add(qcow2Header(math.MaxUint32, 1<<20))
	// Virtual sizes that break the hex round trip qcow2NopReader does: the top
	// bit set overflows the signed parse of the 8 byte size field.
	f.Add(qcow2Header(3, math.MaxUint64))
	f.Add(qcow2Header(3, 1<<63))
	f.Add(qcow2Header(2, math.MaxInt64))
	// Truncated headers: shorter than the 512 bytes io.ReadFull insists on.
	f.Add(qcow2V2[:4])
	f.Add(qcow2V3[:511])
	f.Add([]byte{})
	f.Add([]byte{0x00})
	f.Add([]byte("QFI"))
	// The remaining magic numbers, at the offsets the detection expects them.
	// The vhd magic is what qemu-img calls vpc, so these five headers plus the
	// qcow2 ones cover every format of the allow-list.
	f.Add(headerAt(0, "KDMV"))
	f.Add(headerAt(0, "conectix"))
	f.Add(headerAt(0, "vhdxfile"))
	f.Add(headerAt(0x40, "\x7f\x10\xda\xbe"))
	f.Add(headerAt(0x101, "ustar"))
	// The same magic numbers without the padding the reader stack requires, and
	// with the last byte of the magic cut off.
	f.Add([]byte("KDMV"))
	f.Add([]byte("conectix"))
	f.Add([]byte("vhdxfile"))
	f.Add(headerAt(0, "KDM"))
	f.Add(headerAt(0, "conecti"))
	f.Add(headerAt(0, "vhdxfil"))
	f.Add(headerAt(0x40, "\x7f\x10\xda"))
	f.Add(headerAt(0x100, "ustar"))
	// Magic numbers followed by garbage instead of the rest of the header.
	f.Add(append([]byte("KDMV"), bytes.Repeat([]byte{0xff}, 600)...))
	f.Add(append([]byte("conectix"), bytes.Repeat([]byte{0xff}, 600)...))
	f.Add(append([]byte("vhdxfile"), bytes.Repeat([]byte{0xff}, 600)...))
	f.Add(withBytesAt(qcow2Header(3, 64<<20), 32, bytes.Repeat([]byte{0xff}, 480)))
	// A VMware sparse descriptor, which is text and carries no KDMV magic.
	f.Add(headerAt(0, "# Disk DescriptorFile\nversion=1\nCID=fffffffe\ncreateType=\"monolithicSparse\"\n"))
	// A tar header whose size field has the top bit set, which is the branch
	// where image.Header.Size fails instead of returning a size.
	f.Add(withBytesAt(headerAt(0x101, "ustar"), 124, bytes.Repeat([]byte{0xff}, 8)))
	// Two magic numbers in one header: which one wins depends on the map
	// iteration order of knownHeaders, so the verdict must stay usable either way.
	f.Add(withBytesAt(headerAt(0x101, "ustar"), 0, []byte("QFI\xfb")))
	f.Add(withBytesAt(headerAt(0x40, "\x7f\x10\xda\xbe"), 0, []byte("KDMV")))
	// Compression wrappers around a qcow2 image, which is the realistic
	// container disk layout, plus the broken variants of each.
	f.Add(gzipBytes(f, qcow2V3))
	f.Add(zstdBytes(f, qcow2V3))
	f.Add(xzBytes(f, qcow2V3))
	f.Add(gzipBytes(f, qcow2V3)[:20])
	f.Add(append([]byte{0x1f, 0x8b}, bytes.Repeat([]byte{0xff}, 600)...))
	f.Add(append([]byte{0x28, 0xb5, 0x2f, 0xfd}, bytes.Repeat([]byte{0xff}, 600)...))
	f.Add(append([]byte{0xfd, '7', 'z', 'X', 'Z', 0x00}, bytes.Repeat([]byte{0xff}, 600)...))
	// Double compression: the second gz header is no longer looked for once the
	// first one was consumed.
	f.Add(gzipBytes(f, gzipBytes(f, qcow2V3)))
	f.Add(gzipBytes(f, zstdBytes(f, qcow2V2)))
	// A whole container disk layer: a gzipped tar carrying disk/disk.img.
	f.Add(gzipBytes(f, tarLayer(f, "disk/disk.img", qcow2V3)))
	f.Add(tarLayer(f, "disk/disk.img", bytes.Repeat([]byte{0x00}, 1024)))
	// An ISO 9660 image. This module has no ISO detection, so the payload has to
	// come out as raw rather than as an unknown format.
	f.Add(isoImage())
	// Payloads that match nothing: sparse, all ones, non-ASCII filler.
	f.Add(bytes.Repeat([]byte{0x00}, 64<<10))
	f.Add(bytes.Repeat([]byte{0xff}, 4096))
	f.Add(bytes.Repeat([]byte("\xd0\x9f\xd1\x80\xd0\xb8\xd0\xb2\xd0\xb5\xd1\x82"), 64))
	f.Add(bytes.Repeat([]byte{0xc3, 0x28, 0xa0, 0xa1}, 256))

	f.Fuzz(func(t *testing.T, data []byte) {
		if len(data) > fuzzMaxInput {
			t.Skip()
		}

		fr, err := NewFormatReaders(io.NopCloser(bytes.NewReader(data)), 0)
		if fr == nil {
			t.Fatalf("NewFormatReaders returned no readers for %d bytes, err %v", len(data), err)
		}
		defer func() { _ = fr.Close() }()

		if err != nil {
			return
		}

		requireConsistentFormat(t, fr)

		// The direct-transfer path writes the stream verbatim onto the target
		// when it believes the image is already in the target format, so a
		// qcow2 image reported as raw would corrupt the disk.
		if isPlainQCow2(data) && fr.ImageFormat != "qcow2" {
			t.Fatalf("qcow2 header reported as %q", fr.ImageFormat)
		}

		// Draining is what processLayer does next. A broken stream must surface
		// as an error, and a compression bomb must not outgrow the cap.
		_, _ = io.Copy(io.Discard, io.LimitReader(fr.TopReader(), fuzzMaxDrain))
	})
}

// FuzzSafeJoinPaths drives the zip-slip guard of transport.go with the entry
// names of a container image layer. The names come from the tar inside the
// layer, so they are chosen by whoever published the image; the guard is the
// only thing that keeps the unpacking inside the scratch directory.
func FuzzSafeJoinPaths(f *testing.F) {
	// Names a well formed container disk layer carries.
	f.Add("disk/disk.img")
	f.Add("./disk/disk.img")
	f.Add("disk/")
	f.Add("disk")
	f.Add("")
	// Traversal, in the spellings that survive a naive prefix check.
	f.Add("disk/../../etc/passwd")
	f.Add("disk/../../../../../../etc/shadow")
	f.Add("./disk/../../root/.ssh/authorized_keys")
	f.Add("disk/" + strings.Repeat("../", 128) + "etc/passwd")
	f.Add("..")
	f.Add("../")
	f.Add("../disk/disk.img")
	f.Add("/etc/passwd")
	f.Add("//etc/passwd")
	f.Add("/../disk/disk.img")
	// A sibling directory that shares the prefix: the guard has to keep the
	// separator, and hasPrefix accepts this name.
	f.Add("diskette/disk.img")
	f.Add("disk../disk.img")
	// Redundant separators and current-directory segments.
	f.Add("disk//disk.img")
	f.Add("disk/./disk.img")
	f.Add("disk/sub/../disk.img")
	f.Add("disk/sub/../../disk/disk.img")
	// Whiteout entries, which the caller drops before writing anything.
	f.Add("disk/.wh.disk.img")
	f.Add("disk/.wh..wh..opq")
	// Control bytes, invalid UTF-8, non-ASCII and bidi overrides.
	f.Add("disk/\x00disk.img")
	f.Add("disk/\xff\xfe.img")
	f.Add("disk/\n../etc/passwd")
	f.Add("disk/\u202e gpj.ksid")
	f.Add("disk/\xd0\xbe\xd0\xb1\xd1\x80\xd0\xb0\xd0\xb7.img")
	f.Add("\xd0\xb4\xd0\xb8\xd1\x81\xd0\xba/\xd0\xbe\xd0\xb1\xd1\x80\xd0\xb0\xd0\xb7.img")
	// Windows spellings and shell metacharacters, which are ordinary bytes here.
	f.Add(`disk\..\..\etc\passwd`)
	f.Add("C:/disk/disk.img")
	f.Add("disk/$HOME/../../etc/passwd")
	f.Add("~/disk.img")
	// Oversized names.
	f.Add("disk/" + strings.Repeat("a", 8192))
	f.Add(strings.Repeat("disk/", 2048) + "disk.img")

	f.Fuzz(func(t *testing.T, name string) {
		if len(name) > fuzzMaxInput {
			t.Skip()
		}

		// processLayer applies these three filters before joining anything.
		if !hasPrefix(name, fuzzImageDir) || isWhiteout(name) {
			return
		}

		joined, err := safeJoinPaths(fuzzScratchDir, name)
		if err != nil {
			if joined != "" {
				t.Fatalf("rejected entry %q still produced the path %q", name, joined)
			}

			return
		}

		// An accepted destination must stay inside the scratch directory. The
		// relative path is computed independently of the guard's own check.
		rel, relErr := filepath.Rel(fuzzScratchDir, joined)
		if relErr != nil {
			t.Fatalf("entry %q was accepted as %q, which is not relative to %q: %v", name, joined, fuzzScratchDir, relErr)
		}
		if rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
			t.Fatalf("entry %q escapes the scratch directory as %q", name, joined)
		}
		if filepath.IsAbs(rel) {
			t.Fatalf("entry %q was accepted as the absolute path %q", name, joined)
		}

		// A path that still contains traversal segments would be re-resolved by
		// the file creation that follows.
		if cleaned := filepath.Clean(joined); cleaned != joined {
			t.Fatalf("entry %q was accepted as the unclean path %q", name, joined)
		}
	})
}

// FuzzEnvsToLabels drives the conversion of container image environment
// variables into the labels the importer reports through its termination
// message. The environment comes from the image config of the registry image
// being imported, so both the names and the values are attacker controlled,
// and the result ends up in the status of a VirtualImage.
func FuzzEnvsToLabels(f *testing.F) {
	// Entries a container disk actually carries.
	f.Add("KUBEVIRT_IO_OS=linux")
	f.Add("KUBEVIRT_IO_ARCH=amd64")
	f.Add("KUBEVIRT_IO_OS=linux\nKUBEVIRT_IO_ARCH=amd64\nPATH=/usr/bin")
	// Entries without the prefix, which have to be dropped.
	f.Add("PATH=/usr/local/sbin:/usr/local/bin")
	f.Add("kubevirt_io_os=linux")
	f.Add("KUBEVIRT-IO-OS=linux")
	// The prefix in the middle and at the very end of the name.
	f.Add("VENDOR_KUBEVIRT_IO_OS=linux")
	f.Add("A_B_C_KUBEVIRT_IO_OS=linux")
	f.Add("_KUBEVIRT_IO_OS=linux")
	f.Add("KUBEVIRT_IO_=linux")
	f.Add("KUBEVIRT_IO_")
	// Missing or misplaced separators.
	f.Add("KUBEVIRT_IO_OS")
	f.Add("=linux")
	f.Add("=")
	f.Add("")
	f.Add("KUBEVIRT_IO_OS=")
	f.Add("KUBEVIRT_IO_OS==linux=")
	// Two entries that collapse onto the same label key.
	f.Add("KUBEVIRT_IO_OS=linux\nKUBEVIRT_IO_OS=windows")
	f.Add("KUBEVIRT_IO_O_S=linux\nKUBEVIRT_IO_O__S=linux")
	// Bytes that are not allowed in a label key or value.
	f.Add("KUBEVIRT_IO_OS=linux\n")
	f.Add("KUBEVIRT_IO_ OS =linux")
	f.Add("KUBEVIRT_IO_OS/NAME=linux")
	f.Add("KUBEVIRT_IO_OS=li\nnux")
	f.Add("KUBEVIRT_IO_OS=\x00linux")
	f.Add("KUBEVIRT_IO_\xff\xfe=linux")
	f.Add("KUBEVIRT_IO_\xd0\x9e\xd0\xa1=\xd0\xbb\xd0\xb8\xd0\xbd\xd1\x83\xd0\xba\xd1\x81")
	f.Add("KUBEVIRT_IO_OS=\xd0\xb7\xd0\xbd\xd0\xb0\xd1\x87\xd0\xb5\xd0\xbd\xd0\xb8\xd0\xb5")
	// Oversized names, values and entry counts. The termination message the
	// labels end up in is capped at 4096 bytes by the kubelet.
	f.Add("KUBEVIRT_IO_OS=" + strings.Repeat("a", 8192))
	f.Add("KUBEVIRT_IO_" + strings.Repeat("A", 8192) + "=linux")
	f.Add(strings.Repeat("KUBEVIRT_IO_OS=linux\n", 512))
	f.Add(strings.Repeat("_", 4096) + "KUBEVIRT_IO_OS=linux")

	f.Fuzz(func(t *testing.T, env string) {
		if len(env) > fuzzMaxInput {
			t.Skip()
		}

		envs := strings.Split(env, "\n")
		labels := envsToLabels(envs)

		if len(labels) > len(envs) {
			t.Fatalf("%d entries produced %d labels", len(envs), len(labels))
		}

		for key, value := range labels {
			// Only the kubevirt namespace may be written, and the keys are
			// lower cased so that two spellings of one variable cannot produce
			// two labels.
			if !strings.Contains(key, kubevirtLabelPrefix) {
				t.Fatalf("label %q is outside the %q namespace", key, kubevirtLabelPrefix)
			}
			if key != strings.ToLower(key) {
				t.Fatalf("label key %q is not lower cased", key)
			}

			// Every value has to be the verbatim tail of an entry: the
			// conversion may rewrite names, never values.
			if !hasEnvValue(envs, value) {
				t.Fatalf("label %q carries the value %q, which no entry supplied", key, value)
			}
		}
	})
}

// FuzzNbdcopyProgress drives the parsing of the progress output of nbdcopy,
// which the target importer reads from a pipe the child process writes to. The
// parsed number is added to the exported import progress counter, so a line the
// parser misreads is published as the progress of a real import.
func FuzzNbdcopyProgress(f *testing.F) {
	// The shape nbdcopy --progress writes.
	f.Add("0/100")
	f.Add("1/100")
	f.Add("25/100")
	f.Add("99/100")
	f.Add("100/100")
	f.Add("50.5/100")
	f.Add(" 50/100 ")
	f.Add("\t75/100\n")
	// Shapes the parser has to ignore, including the qemu-img one.
	f.Add("")
	f.Add("/")
	f.Add("//")
	f.Add("50/100%")
	f.Add("(50.00/100%)")
	f.Add("50/1000")
	f.Add("50/10")
	f.Add("50/100/100")
	f.Add("50 / 100")
	f.Add("/100")
	f.Add("50/")
	f.Add("nbdcopy: error: cannot connect to nbd://127.0.0.1:10809")
	// Numbers that are not progress: negative, zero, huge, non-finite, and
	// spellings Go's float parser accepts but nbdcopy never writes.
	f.Add("-1/100")
	f.Add("-0/100")
	f.Add("0.0000001/100")
	f.Add("1e300/100")
	f.Add("+Inf/100")
	f.Add("-Inf/100")
	f.Add("NaN/100")
	f.Add("nan/100")
	f.Add("0x1p10/100")
	f.Add("1_000/100")
	f.Add(strings.Repeat("9", 4096) + "/100")
	// Non-ASCII, invalid UTF-8 and control bytes.
	f.Add("\xc2\xbd/100")
	f.Add("\xd0\xbf\xd1\x80\xd0\xbe\xd0\xb3\xd1\x80\xd0\xb5\xd1\x81\xd1\x81/100")
	f.Add("50\xff/100")
	f.Add("50/100\x00")

	f.Fuzz(func(t *testing.T, line string) {
		if len(line) > fuzzMaxInput {
			t.Skip()
		}

		const uid = "fuzz-nbdcopy-progress"

		// Each iteration starts from a counter of its own, otherwise the
		// verdict would depend on the previous input.
		metrics.Progress(uid).Delete()

		reportNbdcopyProgress(line, uid)

		progress, err := metrics.Progress(uid).Get()
		if err != nil {
			t.Fatalf("reading the progress counter failed: %v", err)
		}
		if progress < 0 {
			t.Fatalf("line %q drove the progress counter to %v", line, progress)
		}

		// Only the exact "<value>/100" shape may move the counter.
		if progress != 0 && !strings.HasSuffix(strings.TrimSpace(line), "/100") {
			t.Fatalf("line %q was accepted as progress %v", line, progress)
		}

		// The counter is monotonic: replaying the same line must not add to it
		// a second time, and it must never be asked to decrease.
		reportNbdcopyProgress(line, uid)

		again, err := metrics.Progress(uid).Get()
		if err != nil {
			t.Fatalf("reading the progress counter failed: %v", err)
		}
		if again != progress {
			t.Fatalf("replaying line %q moved the progress from %v to %v", line, progress, again)
		}

		// An empty owner UID means the importer runs without a progress metric,
		// which must stay a no-op whatever the line says.
		reportNbdcopyProgress(line, "")
	})
}

// requireConsistentFormat pins the two facts the callers of FormatReaders rely
// on: the detected format is one of the formats the import can handle, and the
// conversion flag agrees with it. processLayerToFile compares ImageFormat with
// the target format to decide whether the stream may be written verbatim.
func requireConsistentFormat(t *testing.T, fr *FormatReaders) {
	t.Helper()

	switch fr.ImageFormat {
	case "raw":
		if fr.Convert {
			t.Fatalf("a raw image was flagged for conversion")
		}
	case "qcow2", "vmdk", "vdi", "vhd", "vhdx":
		if !fr.Convert {
			t.Fatalf("a %s image was not flagged for conversion", fr.ImageFormat)
		}
	default:
		t.Fatalf("unknown image format %q", fr.ImageFormat)
	}

	if fr.TopReader() == nil {
		t.Fatalf("format %q was detected without a reader", fr.ImageFormat)
	}
}

// hasEnvValue reports whether any entry carries the given value after its first
// separator, which is the only place a label value may come from.
func hasEnvValue(envs []string, value string) bool {
	for _, env := range envs {
		if _, v, found := strings.Cut(env, "="); found && v == value {
			return true
		}
	}

	return false
}

// isPlainQCow2 reports whether the payload is a qcow2 image that no other known
// magic number competes for. The two magics that sit at a non-zero offset can
// coexist with the qcow2 one, and then the winner depends on the map iteration
// order of knownHeaders, so those inputs are left out of the check.
func isPlainQCow2(data []byte) bool {
	if !bytes.HasPrefix(data, []byte{'Q', 'F', 'I', 0xfb}) {
		return false
	}
	if bytes.HasPrefix(data[min(0x40, len(data)):], []byte{0x7f, 0x10, 0xda, 0xbe}) {
		return false
	}

	return !bytes.HasPrefix(data[min(0x101, len(data)):], []byte("ustar"))
}

// qcow2Header builds the 512 bytes a qcow2 image starts with, with the version
// and the virtual size a crafted image controls. The size field is the one
// qcow2NopReader parses back out of the header.
func qcow2Header(version uint32, size uint64) []byte {
	header := make([]byte, 512)

	copy(header, []byte{'Q', 'F', 'I', 0xfb})
	binary.BigEndian.PutUint32(header[4:], version)
	binary.BigEndian.PutUint32(header[20:], 16) // cluster_bits
	binary.BigEndian.PutUint64(header[24:], size)

	return header
}

// headerAt places a magic number at the offset the detection expects it and pads
// the result to the header size the reader stack reads in one go.
func headerAt(offset int, magic string) []byte {
	header := make([]byte, 512)
	copy(header[offset:], magic)

	return header
}

// isoImage builds the primary volume descriptor of an ISO 9660 filesystem. No
// magic number of the detection sits there, so an ISO is an ordinary raw image
// to this module.
func isoImage() []byte {
	const primaryVolumeDescriptor = 32 * 1024

	image := make([]byte, primaryVolumeDescriptor+2048)
	image[primaryVolumeDescriptor] = 0x01
	copy(image[primaryVolumeDescriptor+1:], "CD001")
	image[primaryVolumeDescriptor+6] = 0x01

	return image
}

// withBytesAt overwrites a range of an already built header, to combine two
// magic numbers or to break a size field.
func withBytesAt(header []byte, offset int, value []byte) []byte {
	patched := bytes.Clone(header)
	copy(patched[offset:], value)

	return patched
}

func gzipBytes(tb testing.TB, payload []byte) []byte {
	tb.Helper()

	var compressed bytes.Buffer

	writer := gzip.NewWriter(&compressed)
	if _, err := writer.Write(payload); err != nil {
		tb.Fatalf("failed to gzip %d bytes: %v", len(payload), err)
	}
	if err := writer.Close(); err != nil {
		tb.Fatalf("failed to close the gzip writer: %v", err)
	}

	return compressed.Bytes()
}

func zstdBytes(tb testing.TB, payload []byte) []byte {
	tb.Helper()

	var compressed bytes.Buffer

	writer, err := zstd.NewWriter(&compressed)
	if err != nil {
		tb.Fatalf("failed to create a zstd writer: %v", err)
	}
	if _, err := writer.Write(payload); err != nil {
		tb.Fatalf("failed to zstd %d bytes: %v", len(payload), err)
	}
	if err := writer.Close(); err != nil {
		tb.Fatalf("failed to close the zstd writer: %v", err)
	}

	return compressed.Bytes()
}

func xzBytes(tb testing.TB, payload []byte) []byte {
	tb.Helper()

	var compressed bytes.Buffer

	writer, err := xz.NewWriter(&compressed)
	if err != nil {
		tb.Fatalf("failed to create an xz writer: %v", err)
	}
	if _, err := writer.Write(payload); err != nil {
		tb.Fatalf("failed to xz %d bytes: %v", len(payload), err)
	}
	if err := writer.Close(); err != nil {
		tb.Fatalf("failed to close the xz writer: %v", err)
	}

	return compressed.Bytes()
}

// tarLayer builds the uncompressed body of a container image layer holding a
// single file, which is the layout the importer looks for.
func tarLayer(tb testing.TB, name string, content []byte) []byte {
	tb.Helper()

	var archive bytes.Buffer

	writer := tar.NewWriter(&archive)
	header := &tar.Header{
		Name:     name,
		Mode:     0o644,
		Size:     int64(len(content)),
		Typeflag: tar.TypeReg,
	}
	if err := writer.WriteHeader(header); err != nil {
		tb.Fatalf("failed to write the tar header of %q: %v", name, err)
	}
	if _, err := writer.Write(content); err != nil {
		tb.Fatalf("failed to write %d bytes into the tar: %v", len(content), err)
	}
	if err := writer.Close(); err != nil {
		tb.Fatalf("failed to close the tar writer: %v", err)
	}

	return archive.Bytes()
}
