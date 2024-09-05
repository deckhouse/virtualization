package datasource

import (
	"fmt"
	"io"
	"os"

	"github.com/google/uuid"
)

type BlockDeviceDataSource struct {
	readCloser          io.ReadCloser
	sourceImageSize     int64
	sourceImageFilename string
}

func NewBlockDeviceDataSource() (*BlockDeviceDataSource, error) {
	blockDevicePath := "/dev/xvda"

	readCloser, err := os.Open(blockDevicePath)
	if err != nil {
		return nil, fmt.Errorf("can not get open device %s: %w", blockDevicePath, err)
	}

	sourceImageSize, err := readCloser.Seek(0, io.SeekEnd)
	if err != nil {
		return nil, fmt.Errorf("error seeking to end: %w", err)
	}

	_, err = readCloser.Seek(0, io.SeekStart)
	if err != nil {
		return nil, fmt.Errorf("error seeking to start: %w", err)
	}

	uuid, _ := uuid.NewUUID()
	sourceImageFilename := uuid.String() + ".raw"

	return &BlockDeviceDataSource{
		readCloser:          readCloser,
		sourceImageSize:     sourceImageSize,
		sourceImageFilename: sourceImageFilename,
	}, nil
}

func (ds *BlockDeviceDataSource) ReadCloser() (io.ReadCloser, error) {
	return ds.readCloser, nil
}

func (ds *BlockDeviceDataSource) Length() (int, error) {
	return int(ds.sourceImageSize), nil
}

func (ds *BlockDeviceDataSource) Filename() (string, error) {
	return ds.sourceImageFilename, nil
}

func (ds *BlockDeviceDataSource) Close() error {
	return ds.readCloser.Close()
}
