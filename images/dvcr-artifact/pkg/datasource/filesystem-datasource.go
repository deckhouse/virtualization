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
	"fmt"
	"io"
	"os"

	"github.com/google/uuid"
)

type FilesystemDataSource struct {
	readCloser          io.ReadCloser
	sourceImageSize     int64
	sourceImageFilename string
}

func NewFilesystemDataSource(ctx context.Context) (*FilesystemDataSource, error) {
	filesystemImagePath := "/tmp/fs/disk.img"

	file, err := os.Open(filesystemImagePath)
	if err != nil {
		return nil, fmt.Errorf("can not get open image %s: %w", filesystemImagePath, err)
	}

	sourceImageSize, err := file.Seek(0, io.SeekEnd)
	if err != nil {
		return nil, fmt.Errorf("error seeking to end: %w", err)
	}

	_, err = file.Seek(0, io.SeekStart)
	if err != nil {
		return nil, fmt.Errorf("error seeking to start: %w", err)
	}

	uuid, _ := uuid.NewUUID()
	sourceImageFilename := uuid.String() + ".img"

	return &FilesystemDataSource{
		readCloser:          file,
		sourceImageSize:     int64(sourceImageSize),
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
