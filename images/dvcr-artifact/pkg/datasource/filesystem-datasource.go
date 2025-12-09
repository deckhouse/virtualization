/*
Copyright 2024 Flant JSC

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

package datasource

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"time"

	"github.com/google/uuid"
)

type FilesystemDataSource struct {
	readCloser          io.ReadCloser
	sourceImageSize     int64
	sourceImageFilename string
}

func NewFilesystemDataSource() (*FilesystemDataSource, error) {
	ctx := context.Background()
	filesystemImagePath := "/tmp/fs/disk.img"

	for {
		cmd := exec.CommandContext(ctx, "qemu-img", "info", "--output=json", filesystemImagePath)
		rawOut, err := cmd.Output()
		if err != nil {
			rawOut, err = cmd.CombinedOutput()
			if err != nil {
				fmt.Printf("inner error running qemu-img info: %w\n", err)
			}
			fmt.Printf("qemu-img command output: %s\n", string(rawOut))

			fmt.Printf("error running qemu-img info: %w\n", err)
		}

		fmt.Printf("qemu-img command output: %s\n", string(rawOut))

		time.Sleep(time.Second)
	}

	file, err := os.OpenFile(filesystemImagePath, os.O_RDONLY, 0)
	if err != nil {
		return nil, fmt.Errorf("can not get open image %s: %w", filesystemImagePath, err)
	}


	type ImageInfo struct {
		VirtualSize uint64 `json:"virtual-size"`
		Format      string `json:"format"`
	}
	var imageInfo ImageInfo


	cmd := exec.CommandContext(ctx, "qemu-img", "info", "--output=json", filesystemImagePath)
	rawOut, err := cmd.Output()
	if err != nil {
		rawOut, err = cmd.CombinedOutput()
		if err != nil {
			return nil, fmt.Errorf("error running qemu-img info: %w", err)
		}
		fmt.Printf("qemu-img command output: %s\n", string(rawOut))

		return nil, fmt.Errorf("error running qemu-img info: %w", err)
	}

	if err = json.Unmarshal(rawOut, &imageInfo); err != nil {
		return nil, fmt.Errorf("error parsing qemu-img info output: %w", err)
	}

	uuid, _ := uuid.NewUUID()
	sourceImageFilename := uuid.String() + ".img"

	return &FilesystemDataSource{
		readCloser:          file,
		sourceImageSize:     int64(imageInfo.VirtualSize),
		sourceImageFilename: sourceImageFilename,
	}, nil
}

func (ds *FilesystemDataSource) ReadCloser() (io.ReadCloser, error) {
	return ds.readCloser, nil
}

func (ds *FilesystemDataSource) Length() (int, error) {
	return int(ds.sourceImageSize), nil
}

func (ds *FilesystemDataSource) Filename() (string, error) {
	return ds.sourceImageFilename, nil
}

func (ds *FilesystemDataSource) Close() error {
	return ds.readCloser.Close()
}
