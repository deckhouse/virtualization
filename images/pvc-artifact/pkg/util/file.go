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

package util

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"unsafe"

	"github.com/pkg/errors"
	"golang.org/x/sys/unix"
	"k8s.io/klog/v2"
)

// OpenFileOrBlockDevice opens the destination data file, whether it is a block device or regular file
func OpenFileOrBlockDevice(fileName string) (*os.File, error) {
	var outFile *os.File
	blockSize, err := GetAvailableSpaceBlock(fileName)
	if err != nil {
		return nil, errors.Wrapf(err, "error determining if block device exists")
	}
	if blockSize >= 0 {
		// Block device found and size determined.
		outFile, err = os.OpenFile(fileName, os.O_EXCL|os.O_WRONLY, os.ModePerm)
	} else {
		// Truncate the leftover of an interrupted attempt instead of failing on it: the
		// importer pod restarts on the same volume (RestartPolicy=OnFailure), so a
		// half-written file must not turn every retry into an endless CrashLoop.
		// Truncating also keeps the tail of a longer previous image out of the new one and
		// leaves the file zero-length, as the zero-appending writers below expect.
		outFile, err = os.OpenFile(fileName, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, os.ModePerm)
	}
	if err != nil {
		return nil, errors.Wrapf(err, "could not open file %q", fileName)
	}
	return outFile, nil
}

// CopyFile copies a file from one location to another.
func CopyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer out.Close()

	_, err = io.Copy(out, in)
	if err != nil {
		return err
	}
	return out.Close()
}

// CopyDir copies a dir from one location to another.
func CopyDir(source, dest string) error {
	// get properties of source dir
	sourceinfo, err := os.Stat(source)
	if err != nil {
		return err
	}

	// create dest dir
	err = os.MkdirAll(dest, sourceinfo.Mode())
	if err != nil {
		return err
	}

	directory, _ := os.Open(source)
	objects, err := directory.Readdir(-1)

	for _, obj := range objects {
		src := filepath.Join(source, obj.Name())
		dst := filepath.Join(dest, obj.Name())

		if obj.IsDir() {
			// create sub-directories - recursively
			err = CopyDir(src, dst)
			if err != nil {
				fmt.Println(err)
			}
		} else {
			// perform copy
			err = CopyFile(src, dst)
			if err != nil {
				fmt.Println(err)
			}
		}
	}
	return err
}

// GetAvailableSpace gets the amount of available space at the path specified.
func GetAvailableSpace(path string) (int64, error) {
	var stat syscall.Statfs_t
	err := syscall.Statfs(path, &stat)
	if err != nil {
		return int64(-1), err
	}
	return int64(stat.Bavail) * stat.Bsize, nil
}

// GetAvailableSpaceBlock gets the amount of available space at the block device path specified.
func GetAvailableSpaceBlock(deviceName string) (int64, error) {
	// Check if the file exists and is a device file.
	if ok, err := IsDevice(deviceName); !ok || err != nil {
		return int64(-1), err
	}

	// Device exists, attempt to get size.
	cmd := exec.Command(blockdevFileName, "--getsize64", deviceName)
	var out bytes.Buffer
	var errBuf bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &errBuf
	err := cmd.Run()
	if err != nil {
		return int64(-1), errors.Errorf("%v, %s", err, errBuf.String())
	}
	i, err := strconv.ParseInt(strings.TrimSpace(out.String()), 10, 64)
	if err != nil {
		return int64(-1), err
	}
	return i, nil
}

// IsDevice returns true if it's a device file
func IsDevice(deviceName string) (bool, error) {
	info, err := os.Stat(deviceName)
	if err == nil {
		return (info.Mode() & os.ModeDevice) != 0, nil
	}

	if os.IsNotExist(err) {
		return false, nil
	}

	return false, err
}

// Three functions for zeroing a range in the destination file:

// ZeroRange asks the kernel to zero [start, start+length) instead of writing the
// zeroes byte by byte, and leaves the file position right after the range so a
// following write continues where the caller expects.
//
// On a block device this is BLKZEROOUT, which the driver may turn into a single
// WRITE_ZEROES command instead of moving the data: on DRBD (write_zeroes_max_bytes
// 128 MiB) zeroing 4 GiB takes 3 s against 49 s for a direct write of the same
// range. On a regular file it is fallocate(ZERO_RANGE), which extends the file when
// the range goes past its end.
//
// Unlike skipping the write, this really zeroes the range, so it stays correct on a
// thick or a reused volume whose blocks do not already read as zero. It does not
// keep the volume thin: whether the range stays unallocated is up to the backend.
//
// Returns the syscall error unchanged, so the caller can fall back to writing
// zeroes when the backend does not implement the operation (ENOTTY, EOPNOTSUPP,
// EINVAL are all seen in the wild).
func ZeroRange(outFile *os.File, start, length int64) error {
	klog.V(4).Infof("Zeroing %d bytes at offset %d", length, start)

	isDevice, err := IsDevice(outFile.Name())
	if err != nil {
		return err
	}

	if isDevice {
		rng := [2]uint64{uint64(start), uint64(length)}
		if _, _, errno := unix.Syscall(
			unix.SYS_IOCTL,
			outFile.Fd(),
			uintptr(unix.BLKZEROOUT),
			uintptr(unsafe.Pointer(&rng[0])),
		); errno != 0 {
			return errno
		}
	} else if err := syscall.Fallocate(int(outFile.Fd()), unix.FALLOC_FL_ZERO_RANGE, start, length); err != nil {
		return err
	}

	// Neither ioctl nor fallocate moves the file position.
	_, err = outFile.Seek(start+length, io.SeekStart)
	return err
}

// PunchHole attempts to zero a range in a file with fallocate, for block devices and pre-allocated files.
func PunchHole(outFile *os.File, start, length int64) error {
	klog.V(4).Infof("Punching %d-byte hole at offset %d", length, start)
	flags := uint32(unix.FALLOC_FL_PUNCH_HOLE | unix.FALLOC_FL_KEEP_SIZE)
	err := syscall.Fallocate(int(outFile.Fd()), flags, start, length)
	if err == nil {
		_, err = outFile.Seek(length, io.SeekCurrent) // Just to move current file position
	}
	return err
}

// AppendZeroWithTruncate resizes the file to append zeroes, meant only for newly-created (empty and zero-length) regular files.
func AppendZeroWithTruncate(outFile *os.File, start, length int64) error {
	klog.V(4).Infof("Truncating %d-bytes from offset %d", length, start)
	end, err := outFile.Seek(0, io.SeekEnd)
	if err != nil {
		return err
	}
	if start != end {
		return errors.Errorf("starting offset %d does not match previous ending offset %d, cannot safely append zeroes to this file using truncate", start, end)
	}
	err = outFile.Truncate(start + length)
	if err != nil {
		return err
	}
	_, err = outFile.Seek(0, io.SeekEnd)
	return err
}

var zeroBuffer []byte

// AppendZeroWithWrite just does normal file writes to the destination, a slow but reliable fallback option.
func AppendZeroWithWrite(outFile *os.File, start, length int64) error {
	klog.Infof("Writing %d zero bytes at offset %d", length, start)
	offset, err := outFile.Seek(0, io.SeekCurrent)
	if err != nil {
		return err
	}
	if start != offset {
		return errors.Errorf("starting offset %d does not match previous ending offset %d, cannot safely append zeroes to this file using write", start, offset)
	}
	if zeroBuffer == nil { // No need to re-allocate this on every write
		zeroBuffer = bytes.Repeat([]byte{0}, 32<<20)
	}
	count := int64(0)
	for count < length {
		blockSize := int64(len(zeroBuffer))
		remaining := length - count
		if remaining < blockSize {
			blockSize = remaining
		}
		written, err := outFile.Write(zeroBuffer[:blockSize])
		if err != nil {
			return errors.Wrapf(err, "unable to write %d zeroes at offset %d: %v", length, start+count, err)
		}
		count += int64(written)
	}
	return nil
}

func StreamDataToFile(r io.Reader, fileName string, preallocate bool) (int64, int64, error) {
	var outFile *os.File
	var bytesRead, bytesWritten int64
	outFile, err := OpenFileOrBlockDevice(fileName)
	if err != nil {
		return 0, 0, err
	}
	defer outFile.Close()

	if !preallocate {
		var isDevice bool
		zeroWriter := AppendZeroWithTruncate
		isDevice, err = IsDevice(fileName)
		if err != nil {
			return 0, 0, err
		}

		if isDevice {
			zeroWriter = PunchHole
		}

		bytesRead, bytesWritten, err = copyWithSparseCheck(outFile, r, zeroWriter)
	} else {
		bytesRead, err = io.Copy(outFile, r)
		bytesWritten = bytesRead
	}

	if err != nil {
		_ = os.Remove(outFile.Name())
		if strings.Contains(err.Error(), "no space left on device") {
			err = errors.Wrapf(err, "unable to write to file")
		}
		return bytesRead, bytesWritten, err
	}

	klog.Infof("Read %d bytes, wrote %d bytes to %s", bytesRead, bytesWritten, outFile.Name())

	err = outFile.Sync()

	return bytesRead, bytesWritten, err
}

type zeroWriterFunc func(*os.File, int64, int64) error

func copyWithSparseCheck(dst *os.File, src io.Reader, zeroWriter zeroWriterFunc) (int64, int64, error) {
	klog.Infof("copyWithSparseCheck to %s", dst.Name())
	const buffSize = 32 * 1024
	var bytesRead, bytesWritten int64
	zeroBuf := make([]byte, buffSize)
	writeBuf := make([]byte, buffSize)
	var writeOffset int64
	for {
		nr, er := src.Read(writeBuf)
		if nr > 0 {
			var nw int
			var ew error
			if bytes.Equal(writeBuf[0:nr], zeroBuf[0:nr]) {
				bytesRead += int64(nr)
			} else {
				if bytesRead > writeOffset {
					// zeroWriter func should seek to bytesRead before returning
					ew = zeroWriter(dst, writeOffset, bytesRead-writeOffset)
					if ew != nil {
						klog.Errorf("Error zeroing range in destination file: %v", ew)
						return bytesRead, bytesWritten, ew
					}
				}
				nw, ew = dst.Write(writeBuf[0:nr])
				if nw < 0 || nr < nw {
					nw = 0
					if ew == nil {
						ew = fmt.Errorf("invalid write result")
					}
				}
				bytesRead += int64(nr)
				bytesWritten += int64(nw)
				writeOffset = bytesRead
				if ew != nil {
					return bytesRead, bytesWritten, ew
				}
				if nr != nw {
					return bytesRead, bytesWritten, io.ErrShortWrite
				}
			}
		}
		if er != nil {
			if er != io.EOF {
				return bytesRead, bytesWritten, er
			}
			break
		}
	}
	if bytesRead > writeOffset {
		if err := zeroWriter(dst, writeOffset, bytesRead-writeOffset); err != nil {
			klog.Errorf("Error zeroing range in destination file: %v", err)
			return bytesRead, bytesWritten, err
		}
	}
	return bytesRead, bytesWritten, nil
}
