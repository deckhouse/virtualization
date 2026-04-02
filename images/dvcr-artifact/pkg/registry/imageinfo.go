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
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"

	"k8s.io/klog/v2"
	"kubevirt.io/containerized-data-importer/pkg/image"
	"kubevirt.io/containerized-data-importer/pkg/importer"
)

const (
	syntheticHeadSize = 10 * 1024 * 1024
	syntheticTailSize = 50 * 1024 * 1024
)

const (
	imageInfoSize        = 64 * 1024 * 1024
	tempImageInfoPattern = "tempfile"
	isoImageType         = "iso"
)

func getImageInfo(ctx context.Context, sourceReader io.ReadCloser) (ImageInfo, error) {
	initialReadSize := syntheticHeadSize
	headerBuf := make([]byte, initialReadSize)
	n, err := io.ReadFull(sourceReader, headerBuf)
	if err != nil && err != io.ErrUnexpectedEOF {
		return ImageInfo{}, fmt.Errorf("error reading initial data: %w", err)
	}
	headerBuf = headerBuf[:n]

	combinedReader := io.MultiReader(
		bytes.NewReader(headerBuf),
		sourceReader,
	)
	combinedReadCloser := io.NopCloser(combinedReader)

	formatSourceReaders, err := importer.NewFormatReaders(combinedReadCloser, 0)
	if err != nil {
		return ImageInfo{}, fmt.Errorf("error creating format readers: %w", err)
	}

	knownHdrs := image.CopyKnownHdrs()
	vmdkHeader, exists := knownHdrs["vmdk"]

	checkSize := min(len(headerBuf), 512)
	if exists && vmdkHeader.Match(headerBuf[:checkSize]) {
		return getImageInfoVMDK(ctx, formatSourceReaders.TopReader(), headerBuf)
	}

	return getImageInfoStandard(ctx, formatSourceReaders, headerBuf)
}

// getImageInfoVMDK obtains information about the VMDK image using a synthetic file.
// This approach is necessary because VMDK stores metadata (footer, Grain Directory)
// at the end of the file, and qemu-img cannot work with partial VMDK.
func getImageInfoVMDK(ctx context.Context, sourceReader io.Reader, headerBuf []byte) (ImageInfo, error) {
	klog.Infoln("Get VMDK image info: prepare temp file with the first and last parts of the image data.")

	var headBuf []byte
	var totalBytesRead int64

	if headerBuf != nil && len(headerBuf) > 0 {
		headSize := syntheticHeadSize
		if len(headerBuf) < headSize {
			headSize = len(headerBuf)
		}
		headBuf = headerBuf[:headSize]
		totalBytesRead = int64(len(headerBuf))

		klog.Infof("Using %d bytes from header as head buffer", len(headBuf))
	} else {
		headBuf = make([]byte, syntheticHeadSize)
		n, err := io.ReadFull(sourceReader, headBuf)
		if err != nil && err != io.ErrUnexpectedEOF {
			return ImageInfo{}, fmt.Errorf("error reading head: %w", err)
		}
		headBuf = headBuf[:n]
		totalBytesRead = int64(n)

		klog.Infof("Read %d bytes as head buffer", n)
	}

	tailBuf := NewTailBuffer(syntheticTailSize)

	if headerBuf != nil && len(headerBuf) > len(headBuf) {
		remainingHeader := headerBuf[len(headBuf):]
		klog.Infof("Adding %d bytes from remaining header to tail buffer", len(remainingHeader))
		tailBuf.Write(remainingHeader)
	}

	klog.Infoln("Streaming remaining VMDK data through tail buffer...")
	written, err := io.Copy(tailBuf, sourceReader)
	if err != nil {
		return ImageInfo{}, fmt.Errorf("error streaming to tail buffer: %w", err)
	}

	totalSize := totalBytesRead + written
	klog.Infof("VMDK total size: %d bytes (%.2f GB)", totalSize, float64(totalSize)/(1024*1024*1024))

	syntheticPath, err := createSyntheticVMDK(headBuf, tailBuf, totalSize)
	if err != nil {
		return ImageInfo{}, fmt.Errorf("error creating synthetic VMDK: %w", err)
	}
	defer os.Remove(syntheticPath)

	klog.Infof("Created synthetic VMDK file: %s", syntheticPath)

	cmd := exec.CommandContext(ctx, "qemu-img", "info", "--output=json", syntheticPath)
	rawOut, err := cmd.CombinedOutput()
	if err != nil {
		klog.Errorf("qemu-img failed on synthetic VMDK: %s", string(rawOut))
		return ImageInfo{}, fmt.Errorf("qemu-img info failed on synthetic VMDK: %w, output: %s", err, string(rawOut))
	}

	klog.Infof("qemu-img output: %s", string(rawOut))

	var imageInfo ImageInfo
	if err = json.Unmarshal(rawOut, &imageInfo); err != nil {
		return ImageInfo{}, fmt.Errorf("error parsing qemu-img output: %w", err)
	}

	return imageInfo, nil
}

// getImageInfoStandard handles non-VMDK formats using the first 64MB of the file.
func getImageInfoStandard(ctx context.Context, formatSourceReaders *importer.FormatReaders, headerBuf []byte) (ImageInfo, error) {
	var tempImageInfoFile *os.File
	var err error
	var bytesWrittenToTemp int64

	klog.Infoln("Get image info: prepare temp file with the first 64Mi of the image data.")
	{
		tempImageInfoFile, err = os.CreateTemp("", tempImageInfoPattern)
		if err != nil {
			return ImageInfo{}, fmt.Errorf("error creating temp file: %w", err)
		}
		defer os.Remove(tempImageInfoFile.Name())

		n, err := tempImageInfoFile.Write(headerBuf)
		if err != nil {
			return ImageInfo{}, fmt.Errorf("error writing header to temp file: %w", err)
		}
		bytesWrittenToTemp = int64(n)

		remaining := imageInfoSize - int64(len(headerBuf))
		if remaining > 0 {
			n, err := io.CopyN(tempImageInfoFile, formatSourceReaders.TopReader(), remaining)
			if err != nil && !errors.Is(err, io.EOF) {
				return ImageInfo{}, fmt.Errorf("error writing remaining data to temp file: %w", err)
			}
			bytesWrittenToTemp += n
		}

		if err = tempImageInfoFile.Close(); err != nil {
			return ImageInfo{}, fmt.Errorf("error closing temp file: %w", err)
		}
	}

	klog.Infoln("Get image info from temp file")
	var imageInfo ImageInfo
	{
		cmd := exec.CommandContext(ctx, "qemu-img", "info", "--output=json", tempImageInfoFile.Name())
		rawOut, err := cmd.CombinedOutput()
		if err != nil {
			klog.Warningf("qemu-img info failed: %v", err)
			klog.Warningf("qemu-img output: %s", string(rawOut))
			return ImageInfo{}, fmt.Errorf("error running qemu-img info: %s: %w", string(rawOut), err)
		}

		klog.Infoln("Qemu-img command output:", string(rawOut))

		if err = json.Unmarshal(rawOut, &imageInfo); err != nil {
			return ImageInfo{}, fmt.Errorf("error parsing qemu-img info output: %w", err)
		}

		if imageInfo.Format != "raw" {
			// It's necessary to read everything from the original image to avoid blocking.
			_, err = io.Copy(&EmptyWriter{}, formatSourceReaders.TopReader())
			if err != nil {
				return ImageInfo{}, fmt.Errorf("error copying to nowhere: %w", err)
			}

			return imageInfo, nil
		}
	}

	// `qemu-img` command does not support getting information about iso files.
	// It is necessary to obtain this information in another way (using the `file` command).
	klog.Infoln("Check the image as it may be an iso")
	{
		cmd := exec.CommandContext(ctx, "file", "-b", tempImageInfoFile.Name())
		rawOut, err := cmd.Output()
		if err != nil {
			return ImageInfo{}, fmt.Errorf("error running file info: %w", err)
		}

		out := string(rawOut)

		klog.Infoln("File command output:", out)

		if strings.HasPrefix(strings.ToLower(out), isoImageType) {
			imageInfo.Format = isoImageType
		}

		// Count uncompressed size of source image.
		n, err := io.Copy(&EmptyWriter{}, formatSourceReaders.TopReader())
		if err != nil {
			return ImageInfo{}, fmt.Errorf("error copying to nowhere: %w", err)
		}

		imageInfo.VirtualSize = uint64(bytesWrittenToTemp + n)

		return imageInfo, nil
	}
}

func createSyntheticVMDK(headBuf []byte, tailBuf *TailBuffer, totalSize int64) (string, error) {
	tmpFile, err := os.CreateTemp("", "synthetic-*.vmdk")
	if err != nil {
		return "", fmt.Errorf("error creating temp file: %w", err)
	}
	defer tmpFile.Close()

	_, err = tmpFile.Write(headBuf)
	if err != nil {
		os.Remove(tmpFile.Name())
		return "", fmt.Errorf("error writing head: %w", err)
	}

	tailData := tailBuf.Bytes()
	tailOffset := totalSize - int64(len(tailData))

	if tailOffset > int64(len(headBuf)) {
		_, err = tmpFile.Seek(tailOffset, io.SeekStart)
		if err != nil {
			os.Remove(tmpFile.Name())
			return "", fmt.Errorf("error seeking: %w", err)
		}
	}

	_, err = tmpFile.Write(tailData)
	if err != nil {
		os.Remove(tmpFile.Name())
		return "", fmt.Errorf("error writing tail: %w", err)
	}

	return tmpFile.Name(), nil
}
