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

package registry

import (
	"bytes"
	"context"
	"encoding/binary"
	"io"
	"math"
	"os/exec"
	"testing"
	"time"

	"github.com/golang/snappy"
	cdiv1 "kubevirt.io/containerized-data-importer-api/pkg/apis/core/v1beta1"
	"kubevirt.io/containerized-data-importer/pkg/importer"
)

const (
	// fuzzDestImageName is never contacted: the fuzz targets stop before the
	// upload, the name only has to be parseable by NewDataProcessor.
	fuzzDestImageName = "localhost:5000/fuzz:latest"

	// qemu-img and file run as child processes. The deadline turns a hung
	// helper into an error instead of a stuck fuzzing worker.
	fuzzImageInfoTimeout = 30 * time.Second

	// fuzzMaxInputSize bounds what an iteration is allowed to carry. Every byte
	// above the header the format detection reads is copied through a temporary
	// file and handed to a child process, and a bigger hostile image is not a
	// more interesting one.
	fuzzMaxInputSize = 1 << 20
)

// imageFormatMagics are the magic numbers of the disk formats the fuzzing
// assignment names besides qcow2, raw and iso, at the offset each format keeps
// them at.
var imageFormatMagics = []struct {
	offset int
	magic  string
}{
	{0, "KDMV"},                // vmdk, the sparse extent header
	{0x40, "\x7f\x10\xda\xbe"}, // vdi
	{0, "conectix"},            // vpc, the footer cookie of a vhd
	{0, "cxsparse"},            // vpc, the dynamic disk header of a vhd
	{0, "vhdxfile"},            // vhdx
}

// requireImageTools fails the target when a binary getImageInfo shells out to
// is missing. Without them getImageInfo fails on the same line for every input,
// the targets take that as a rejected image, and the run reports success over
// nothing at thousands of execs per second.
func requireImageTools(f *testing.F) {
	f.Helper()

	for _, binary := range []string{"qemu-img", "file"} {
		if _, err := exec.LookPath(binary); err != nil {
			f.Fatalf("%s is required to reach the image parsing path: %v", binary, err)
		}
	}
}

// FuzzImageInfo feeds fuzzed bytes into the image parsing path without going
// through the HTTP layer: the payload is carried by the CDI UploadDataSource
// that NewDataProcessor is given, and the format detection reads the image
// from there. A malformed image is the normal case here and must only produce
// an error - a panic, a hang or a wrong size accounting is the finding.
func FuzzImageInfo(f *testing.F) {
	requireImageTools(f)

	var (
		// A 512 byte header is what the format detection reads before it
		// decides anything, so these are complete inputs from its point of view.
		qcow2V2 = qcow2Header(2, 64<<20, 16, 0, 0)
		qcow2V3 = qcow2Header(3, 64<<20, 16, 0, 0)
	)

	f.Add(qcow2V2)
	f.Add(qcow2V3)
	f.Add(plausibleQCow2())
	// Versions the module never writes: 0 and 1 are pre-qcow2, and the upper
	// bound checks the version comparison itself.
	f.Add(qcow2Header(0, 64<<20, 16, 0, 0))
	f.Add(qcow2Header(1, 64<<20, 16, 0, 0))
	f.Add(qcow2Header(math.MaxUint32, 64<<20, 16, 0, 0))
	// Absurd virtual sizes. MaxUint64 and 1<<63 both overflow the signed parse
	// of the size field, MaxInt64 is the largest value that still fits.
	f.Add(qcow2Header(3, math.MaxUint64, 16, 0, 0))
	f.Add(qcow2Header(3, 1<<63, 16, 0, 0))
	f.Add(qcow2Header(2, math.MaxInt64, 16, 0, 0))
	f.Add(qcow2Header(3, 0, 16, 0, 0))
	// Cluster sizes outside the 512 B - 2 MiB range qcow2 allows.
	f.Add(qcow2Header(3, 64<<20, 0, 0, 0))
	f.Add(qcow2Header(3, 64<<20, 8, 0, 0))
	f.Add(qcow2Header(3, 64<<20, 63, 0, 0))
	f.Add(qcow2Header(3, 64<<20, math.MaxUint32, 0, 0))
	// Backing file references pointing outside the image, and a backing file
	// name that claims to be longer than the whole header.
	f.Add(qcow2Header(3, 64<<20, 16, math.MaxUint64, math.MaxUint32))
	f.Add(qcow2Header(3, 64<<20, 16, 1<<62, 1<<20))
	f.Add(withBackingFileName(qcow2Header(3, 64<<20, 16, 108, 1024), "/etc/shadow"))
	f.Add(withBackingFileName(qcow2Header(2, 64<<20, 16, 108, 11), "../../../etc/passwd"))
	// Truncated headers: shorter than the 512 bytes the reader stack insists on.
	f.Add(qcow2V2[:4])
	f.Add(qcow2V2[:24])
	f.Add(qcow2V2[:31])
	f.Add(qcow2V3[:72])
	f.Add(qcow2V3[:511])
	f.Add([]byte{})
	f.Add([]byte{0x00})
	// Raw payloads: sparse, all-ones, repetitive and incompressible.
	f.Add(bytes.Repeat([]byte{0x00}, 64<<10))
	f.Add(bytes.Repeat([]byte{0xff}, 64<<10))
	f.Add(bytes.Repeat([]byte{0xde, 0xad, 0xbe, 0xef}, 1024))
	f.Add(patternBytes(f, 64<<10))
	// An ISO 9660 image: qemu-img calls it raw, so the format comes from `file`.
	// Then the same image with the volume descriptor cut in half and with a
	// broken CD001 identifier, which both have to fall back to raw.
	f.Add(isoImage())
	f.Add(isoImage()[:32*1024+3])
	f.Add(patchByte(isoImage(), 32*1024+2, 0x00))
	// The disk formats of the fuzzing assignment other than qcow2, raw and iso.
	// Each one gets an input that ends right after its magic - short of the 512
	// bytes the reader stack reads in one go - the padded header the detection
	// accepts, and the same header with every length, offset and size field
	// behind the magic set to its maximum.
	for _, format := range imageFormatMagics {
		f.Add(headerAt(format.offset, format.magic)[:format.offset+len(format.magic)])
		f.Add(headerAt(format.offset, format.magic))
		f.Add(withHostileFields(headerAt(format.offset, format.magic), format.offset+len(format.magic)))
	}
	// The textual form of a vmdk, plain and with an extent that claims the whole
	// 64 bit sector range.
	f.Add(headerAt(0, "# Disk DescriptorFile\nversion=1\nCID=fffffffe\ncreateType=\"monolithicSparse\"\n"))
	f.Add(headerAt(0, "# Disk DescriptorFile\nversion=1\nCID=fffffffe\ncreateType=\"monolithicSparse\"\nRW 18446744073709551615 SPARSE \"disk-s001.vmdk\"\n"))
	// Headers of the wrappers the detection unwraps before it sees a disk image.
	f.Add(headerAt(0, "\x1f\x8b\x08\x00\x00\x00\x00\x00\x00\x03"))
	f.Add(headerAt(0, "\xfd7zXZ\x00"))
	f.Add(headerAt(0, "\x28\xb5\x2f\xfd"))
	f.Add(headerAt(0x101, "ustar"))
	// A qcow2 header hiding behind a compression magic that decompresses to
	// nothing: the detection loop has to give up instead of looping.
	f.Add(append([]byte("\x1f\x8b\x08\x00\x00\x00\x00\x00\x00\x03"), qcow2V2...))
	f.Add(append([]byte("\x28\xb5\x2f\xfd"), qcow2V3...))

	f.Fuzz(func(t *testing.T, data []byte) {
		if len(data) > fuzzMaxInputSize {
			t.Skip()
		}

		info, err := fuzzImageInfo(t, io.NopCloser(bytes.NewReader(data)), len(data))
		if err != nil {
			return
		}

		// Uncompressed sources are counted byte by byte, and every byte has to
		// be counted exactly once: a double counted header inflates the
		// VirtualImage size and breaks the size validation of disks cloned
		// from it. Compressed sources are excluded because there the counted
		// bytes are the decompressed ones.
		if info.Format == "raw" || info.Format == isoImageType {
			if !hasCompressionHeader(data) && info.VirtualSize != uint64(len(data)) {
				t.Fatalf("%s source of %d bytes reports virtual size %d", info.Format, len(data), info.VirtualSize)
			}
		}
	})
}

// FuzzImageInfoSnappy covers the blockdevice-clone path, where the uploader
// wraps the incoming stream into a snappy reader before the image parsing sees
// it. The fuzzed bytes are therefore the snappy frames themselves: both the
// framing and the image inside it are attacker controlled.
func FuzzImageInfoSnappy(f *testing.F) {
	requireImageTools(f)

	var (
		qcow2V2 = qcow2Header(2, 64<<20, 16, 0, 0)
		qcow2V3 = qcow2Header(3, 64<<20, 16, 0, 0)
		// A well formed frame stream, used as the base of the broken ones. The
		// payload does not compress, so its chunks are long enough to corrupt
		// a checksum and a length field independently.
		framedRaw = snappyFrame(f, patternBytes(f, 4096))
	)

	// Well formed frames around every format the parsing distinguishes.
	f.Add(snappyFrame(f, qcow2V2))
	f.Add(snappyFrame(f, qcow2V3))
	f.Add(snappyFrame(f, plausibleQCow2()))
	f.Add(snappyFrame(f, qcow2Header(3, math.MaxUint64, 16, 0, 0)))
	f.Add(snappyFrame(f, qcow2Header(3, 64<<20, 16, math.MaxUint64, math.MaxUint32)))
	f.Add(snappyFrame(f, bytes.Repeat([]byte{0x00}, 64<<10)))
	f.Add(snappyFrame(f, bytes.Repeat([]byte{0xff}, 64<<10)))
	f.Add(snappyFrame(f, isoImage()))
	f.Add(framedRaw)
	// The same disk formats the uncompressed target starts from, framed: the
	// padded header and the header whose fields all claim their maximum.
	for _, format := range imageFormatMagics {
		f.Add(snappyFrame(f, headerAt(format.offset, format.magic)))
		f.Add(snappyFrame(f, withHostileFields(headerAt(format.offset, format.magic), format.offset+len(format.magic))))
	}
	// A frame stream that decodes to less than the 512 bytes the reader stack
	// needs, and one that decodes to nothing at all.
	f.Add(snappyFrame(f, qcow2V2[:24]))
	f.Add(snappyFrame(f, []byte{0x00}))
	// Broken framing. The stream identifier is the first thing snappy checks.
	f.Add([]byte("\xff\x06\x00\x00sNaPpY"))
	f.Add([]byte("\xff\x06\x00\x00sNaP"))
	f.Add([]byte("\xff\x06\x00\x00SNAPPY"))
	f.Add([]byte("\xff\x07\x00\x00sNaPpY\x00"))
	f.Add([]byte("\xff\xff\xff\xffsNaPpY"))
	f.Add([]byte("\xff\x06\x00\x00sNaPpY\xff\x06\x00\x00sNaPpY"))
	// A data chunk that never announced the stream.
	f.Add([]byte("\x01\x05\x00\x00\x00\x00\x00\x00hello"))
	// Chunk types the format reserves: unskippable ones must fail, skippable
	// ones must be ignored without losing the payload behind them.
	f.Add(append([]byte("\xff\x06\x00\x00sNaPpY\x02\x04\x00\x00dead"), framedRaw...))
	f.Add(append([]byte("\xff\x06\x00\x00sNaPpY\x7f\x04\x00\x00dead"), framedRaw...))
	f.Add(append([]byte("\xff\x06\x00\x00sNaPpY\x80\x04\x00\x00dead"), framedRaw[10:]...))
	f.Add(append([]byte("\xff\x06\x00\x00sNaPpY\xfe\x00\x00\x00"), framedRaw[10:]...))
	// Corrupted checksum, corrupted chunk length and a truncated chunk body of
	// an otherwise valid stream.
	f.Add(patchByte(framedRaw, 10, 0x03))
	f.Add(patchByte(framedRaw, 15, 0x00))
	f.Add(patchByte(patchByte(framedRaw, 11, 0xff), 12, 0xff))
	f.Add(framedRaw[:len(framedRaw)/2])
	f.Add(framedRaw[:20])
	// A compressed block declaring a decoded length of 2^32-1 bytes.
	f.Add([]byte("\xff\x06\x00\x00sNaPpY\x00\x09\x00\x00\x00\x00\x00\x00\xff\xff\xff\xff\x0f\x00"))
	// Unframed payloads: the reader has to reject them rather than pass them on.
	f.Add(qcow2V2)
	f.Add([]byte{})

	f.Fuzz(func(t *testing.T, data []byte) {
		if len(data) > fuzzMaxInputSize {
			t.Skip()
		}

		// This mirrors newContentReader of the uploader for the
		// blockdevice-clone content type.
		stream := io.NopCloser(snappy.NewReader(bytes.NewReader(data)))

		_, _ = fuzzImageInfo(t, stream, len(data))
	})
}

// fuzzImageInfo wires the stream the way the uploader does and reads the image
// information from it. Only failures that cannot be blamed on the payload are
// reported; a rejected image is returned as an error for the caller to ignore.
func fuzzImageInfo(t *testing.T, stream io.ReadCloser, length int) (ImageInfo, error) {
	t.Helper()

	uds := importer.NewUploadDataSource(stream, cdiv1.DataVolumeKubeVirt, length)
	defer func() { _ = uds.Close() }()

	if _, err := NewDataProcessor(uds, DestinationRegistry{ImageName: fuzzDestImageName}, nil); err != nil {
		t.Fatalf("NewDataProcessor rejected the upload data source: %v", err)
	}

	reader, err := uds.ReadCloser()
	if err != nil {
		t.Fatalf("upload data source returned no reader: %v", err)
	}

	ctx, cancel := context.WithTimeout(t.Context(), fuzzImageInfoTimeout)
	defer cancel()

	info, err := getImageInfo(ctx, reader)
	if err != nil {
		return ImageInfo{}, err
	}

	// The format ends up in an image label and in the VirtualImage status, so
	// an accepted image without a format would be published as an empty one.
	if info.Format == "" {
		t.Fatalf("image accepted without a format, virtual size %d", info.VirtualSize)
	}

	return info, nil
}

// qcow2Header builds the 512 bytes a qcow2 image starts with. The arguments are
// the fields a crafted image controls: the version, the virtual disk size, the
// cluster size exponent and the reference to a backing file.
func qcow2Header(version uint32, size uint64, clusterBits uint32, backingOffset uint64, backingSize uint32) []byte {
	header := make([]byte, 512)

	copy(header, []byte{'Q', 'F', 'I', 0xfb})
	binary.BigEndian.PutUint32(header[4:], version)
	binary.BigEndian.PutUint64(header[8:], backingOffset)
	binary.BigEndian.PutUint32(header[16:], backingSize)
	binary.BigEndian.PutUint32(header[20:], clusterBits)
	binary.BigEndian.PutUint64(header[24:], size)
	binary.BigEndian.PutUint32(header[36:], 1)   // l1_size
	binary.BigEndian.PutUint64(header[40:], 512) // l1_table_offset
	binary.BigEndian.PutUint64(header[48:], 512) // refcount_table_offset
	binary.BigEndian.PutUint32(header[56:], 1)   // refcount_table_clusters

	if version >= 3 {
		binary.BigEndian.PutUint32(header[96:], 4)    // refcount_order
		binary.BigEndian.PutUint32(header[100:], 104) // header_length
	}

	return header
}

// withBackingFileName writes a backing file name at the offset the header
// already points to, so that the name is actually reachable.
func withBackingFileName(header []byte, name string) []byte {
	offset := binary.BigEndian.Uint64(header[8:])
	copy(header[offset:], name)

	return header
}

// plausibleQCow2 builds an empty but complete qcow2 image of 1 MiB with 512 byte
// clusters, laid out the way qemu-img writes it: the header, the refcount table,
// one refcount block and the L1 table, one cluster each. Unlike a bare header
// this image is accepted, so the fuzzing starts from something that parses and
// mutates its way out of it.
func plausibleQCow2() []byte {
	const (
		clusterBits = 9
		clusterSize = 1 << clusterBits
		virtualSize = 1 << 20
		// One L1 entry covers a whole L2 table worth of guest clusters.
		l1Entries = virtualSize / (clusterSize * clusterSize / 8)
		// The header, the refcount table, the refcount block and the L1 table.
		usedClusters = 4
	)

	image := make([]byte, 3*clusterSize+l1Entries*8)
	copy(image, qcow2Header(3, virtualSize, clusterBits, 0, 0))

	binary.BigEndian.PutUint32(image[36:], l1Entries)     // l1_size
	binary.BigEndian.PutUint64(image[40:], 3*clusterSize) // l1_table_offset
	binary.BigEndian.PutUint64(image[48:], clusterSize)   // refcount_table_offset
	binary.BigEndian.PutUint32(image[56:], 1)             // refcount_table_clusters
	binary.BigEndian.PutUint32(image[100:], 112)          // header_length

	binary.BigEndian.PutUint64(image[clusterSize:], 2*clusterSize) // the only refcount block
	for cluster := range usedClusters {
		binary.BigEndian.PutUint16(image[2*clusterSize+2*cluster:], 1)
	}

	return image
}

// headerAt places a magic number at the offset the format detection expects it
// and pads the result to the header size the reader stack reads in one go.
func headerAt(offset int, magic string) []byte {
	header := make([]byte, 512)
	copy(header[offset:], magic)

	return header
}

// withHostileFields fills everything behind the magic with ones, so that every
// length, offset and size field the header carries claims its maximum.
func withHostileFields(header []byte, magicEnd int) []byte {
	hostile := bytes.Clone(header)
	for i := magicEnd; i < len(hostile); i++ {
		hostile[i] = 0xff
	}

	return hostile
}

// isoImage builds the primary volume descriptor of an ISO 9660 filesystem,
// which is what the `file` fallback of the raw path looks for.
func isoImage() []byte {
	const primaryVolumeDescriptor = 32 * 1024

	image := make([]byte, primaryVolumeDescriptor+8*1024)
	image[primaryVolumeDescriptor] = 0x01
	copy(image[primaryVolumeDescriptor+1:], "CD001")
	image[primaryVolumeDescriptor+6] = 0x01

	return image
}

// patternBytes materializes the deterministic non-zero stream of patternReader,
// which matches no known image header and does not compress.
func patternBytes(tb testing.TB, size int64) []byte {
	tb.Helper()

	data, err := io.ReadAll(&patternReader{size: size})
	if err != nil {
		tb.Fatalf("failed to build a %d byte pattern: %v", size, err)
	}

	return data
}

// hasCompressionHeader reports whether the detection unwraps the payload before
// counting its bytes, in which case the counted size is the decompressed one.
func hasCompressionHeader(data []byte) bool {
	for _, magic := range []string{"\x1f\x8b", "\x28\xb5\x2f\xfd", "\xfd7zXZ\x00"} {
		if bytes.HasPrefix(data, []byte(magic)) {
			return true
		}
	}

	return false
}

// snappyFrame encodes a payload the way the blockdevice-clone client does.
func snappyFrame(tb testing.TB, payload []byte) []byte {
	tb.Helper()

	var framed bytes.Buffer

	writer := snappy.NewBufferedWriter(&framed)
	if _, err := writer.Write(payload); err != nil {
		tb.Fatalf("failed to snappy-encode %d bytes: %v", len(payload), err)
	}
	if err := writer.Close(); err != nil {
		tb.Fatalf("failed to close the snappy writer: %v", err)
	}

	return framed.Bytes()
}

// patchByte copies the input with a single byte replaced, to break a checksum
// or a length field of an otherwise valid stream.
func patchByte(data []byte, index int, value byte) []byte {
	patched := bytes.Clone(data)
	patched[index] = value

	return patched
}
