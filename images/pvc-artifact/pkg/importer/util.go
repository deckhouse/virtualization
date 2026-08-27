/*
Copyright 2018 The CDI Authors.
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
	"bytes"
	"io"
	"net/url"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/pkg/errors"
	"k8s.io/klog/v2"

	"github.com/deckhouse/virtualization/images/pvc-artifact/pkg/common"
	"github.com/deckhouse/virtualization/images/pvc-artifact/pkg/util"
)

const (
	kubevirtEnvPrefix   = "KUBEVIRT_IO_"
	kubevirtLabelPrefix = "kubevirt.io/"

	// copyBufferSize is the block size used when streaming image data to the target
	// file/device. Hardcoded to 1 MiB.
	copyBufferSize = 1024 * 1024
)

// zeroRanger is implemented by a destination that can zero a range without
// receiving the zeroes. copyBuffered uses it for all-zero blocks and falls back
// to a plain write when the destination does not implement it or the backend
// refuses the operation.
type zeroRanger interface {
	ZeroRange(start, length int64) error
}

// zeroAwareFile adapts *os.File to zeroRanger. Exposing only Write also keeps
// io.ReaderFrom of the underlying file hidden, so a future switch back to
// io.Copy cannot silently bypass the buffering below.
type zeroAwareFile struct {
	f *os.File
}

func (z zeroAwareFile) Write(p []byte) (int, error) { return z.f.Write(p) }

func (z zeroAwareFile) ZeroRange(start, length int64) error {
	return util.ZeroRange(z.f, start, length)
}

// copyBuffered streams r to w accumulating full copyBufferSize chunks before each
// write. Network readers deliver short reads (tens of KiB), and passing them
// through to the block device produces small unaligned bios that trigger
// read-modify-write on every write, which is disastrous for remote volumes
// (e.g. diskless DRBD). Filling the buffer makes each write exactly
// copyBufferSize bytes except the last one, unconditionally: unlike
// bufio.Writer + io.Copy, this does not depend on disabling io.WriterTo of the
// source or io.ReaderFrom of the destination.
//
// Only io.EOF ends the copy. io.ReadFull is deliberately not used here: it
// reports io.ErrUnexpectedEOF both for a legitimate tail shorter than the
// buffer and for a source cut off mid-stream (gzip, tar and net/http all
// report a truncated stream that way), so the two cannot be told apart and a
// broken transfer would look like a complete one.
func copyBuffered(w io.Writer, r io.Reader) error {
	buf := make([]byte, copyBufferSize)
	zeroes := make([]byte, copyBufferSize)
	zr, zeroRangeSupported := w.(zeroRanger)
	var offset int64
	filled := 0

	// flush hands one full or trailing block to the destination: zeroing it
	// through the kernel when the block is all zeroes and the backend supports
	// that, writing it out otherwise.
	flush := func(block []byte) error {
		if zeroRangeSupported && bytes.Equal(block, zeroes[:len(block)]) {
			err := zr.ZeroRange(offset, int64(len(block)))
			if err == nil {
				offset += int64(len(block))
				return nil
			}
			// Backends differ in which error they raise for an unsupported
			// operation, so stop trying after the first refusal instead of
			// paying for a failing syscall per block.
			klog.V(1).Infof("Zeroing ranges not available on this target (%v), writing zeroes instead", err)
			zeroRangeSupported = false
		}
		if _, err := w.Write(block); err != nil {
			return err
		}
		offset += int64(len(block))
		return nil
	}

	for {
		n, err := r.Read(buf[filled:])
		filled += n
		if filled == len(buf) {
			if ew := flush(buf); ew != nil {
				return ew
			}
			filled = 0
		}
		if err != nil {
			if filled > 0 {
				if ew := flush(buf[:filled]); ew != nil {
					return ew
				}
			}
			if err == io.EOF {
				return nil
			}
			return err
		}
	}
}

// ParseEndpoint parses the required endpoint and return the url struct.
func ParseEndpoint(endpt string) (*url.URL, error) {
	if endpt == "" {
		// Because we are passing false, we won't decode anything and there is no way to error.
		endpt, _ = util.ParseEnvVar(common.ImporterEndpoint, false)
		if endpt == "" {
			return nil, errors.Errorf("endpoint %q is missing or blank", common.ImporterEndpoint)
		}
	}
	return url.Parse(endpt)
}

// CleanAll deletes all files at specified paths (recursively)
func CleanAll(paths ...string) error {
	for _, p := range paths {
		isDevice, err := util.IsDevice(p)
		if err != nil {
			return err
		}

		if !isDevice {
			// Remove handles p not existing
			if err := os.RemoveAll(p); err != nil {
				return err
			}
		}
	}
	return nil
}

// GetTerminationChannel returns a channel that listens for SIGTERM
func GetTerminationChannel() <-chan os.Signal {
	terminationChannel := make(chan os.Signal, 1)
	signal.Notify(terminationChannel, os.Interrupt, syscall.SIGTERM)
	return terminationChannel
}

func envsToLabels(envs []string) map[string]string {
	labels := map[string]string{}
	for _, env := range envs {
		k, v, found := strings.Cut(env, "=")
		if !found || !strings.Contains(k, kubevirtEnvPrefix) {
			continue
		}
		labels[envToLabel(k)] = v
	}

	return labels
}

func envToLabel(env string) string {
	label := ""
	before, after, _ := strings.Cut(env, kubevirtEnvPrefix)
	if elems := strings.Split(strings.TrimSuffix(before, "_"), "_"); len(elems) > 0 && elems[0] != "" {
		label += strings.Join(elems, ".") + "."
	}
	label += kubevirtLabelPrefix
	label += strings.Join(strings.Split(after, "_"), "-")

	return strings.ToLower(label)
}

// streamDataToFile provides a function to stream the specified io.Reader to the specified local file
func streamDataToFile(r io.Reader, fileName string) error {
	outFile, err := util.OpenFileOrBlockDevice(fileName)
	if err != nil {
		return err
	}
	defer outFile.Close()
	klog.V(1).Infof("Writing data...\n")
	start := time.Now()
	klog.V(1).Infof("Copy to %s started at %s (block size %d bytes)", fileName, start.Format(time.RFC3339Nano), copyBufferSize)
	if err = copyBuffered(zeroAwareFile{outFile}, r); err != nil {
		klog.Errorf("Unable to write file from dataReader: %v\n", err)
		_ = os.Remove(outFile.Name())
		if strings.Contains(err.Error(), "no space left on device") {
			return errors.Wrapf(err, "unable to write to file")
		}
		return NewImagePullFailedError(err)
	}
	end := time.Now()
	klog.V(1).Infof("Copy to %s finished at %s (duration %s)", fileName, end.Format(time.RFC3339Nano), end.Sub(start))
	err = outFile.Sync()
	klog.V(1).Infof("Sync of %s took %s", fileName, time.Since(end))
	return err
}
