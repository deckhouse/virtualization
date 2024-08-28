package datasource

import (
	"fmt"
	"io"
	"os"
	"syscall"
)

type BlockDeviceDataSource struct {
	devicePath string
	file       *os.File
}

func NewBlockDeviceDataSource() (*BlockDeviceDataSource, error) {
	devicePath := "/dev/xvda"
	file, err := os.Open(devicePath)
	if err != nil {
		return nil, fmt.Errorf("failed to open block device: %v", err)
	}
	return &BlockDeviceDataSource{
		devicePath: devicePath,
		file:       file,
	}, nil
}

func (bd *BlockDeviceDataSource) Filename() (string, error) {
	return "device", nil
}

func (bd *BlockDeviceDataSource) Length() (int, error) {
	var stat syscall.Stat_t
	if err := syscall.Stat(bd.devicePath, &stat); err != nil {
		return 0, fmt.Errorf("failed to stat block device: %v", err)
	}
	return int(stat.Size), nil
}

func (bd *BlockDeviceDataSource) ReadCloser() (io.ReadCloser, error) {
	return bd.file, nil
}

func (bd *BlockDeviceDataSource) Close() error {
	if bd.file != nil {
		return bd.file.Close()
	}
	return nil
}
