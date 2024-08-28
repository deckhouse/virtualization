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
	dataChan   chan []byte
	errChan    chan error
	doneChan   chan struct{}
}

func NewBlockDeviceDataSource() (*BlockDeviceDataSource, error) {
	devicePath := "/dev/xvda"

	file, err := os.Open(devicePath)
	if err != nil {
		return nil, fmt.Errorf("failed to open block device: %v", err)
	}

	bd := &BlockDeviceDataSource{
		devicePath: devicePath,
		file:       file,
		dataChan:   make(chan []byte, 1024),
		errChan:    make(chan error, 1),
		doneChan:   make(chan struct{}),
	}

	go bd.readDataInBackground()

	return bd, nil
}

// readDataInBackground читает данные из блочного устройства и отправляет их в канал
func (bd *BlockDeviceDataSource) readDataInBackground() {
	buf := make([]byte, 1024)
	for {
		n, err := bd.file.Read(buf)
		if err != nil {
			if err != io.EOF {
				bd.errChan <- err
			}
			close(bd.dataChan)
			close(bd.doneChan)
			return
		}
		if n > 0 {
			bd.dataChan <- buf[:n]
		}
	}
}

// Filename возвращает путь к блочному устройству
func (bd *BlockDeviceDataSource) Filename() (string, error) {
	return bd.devicePath, nil
}

// Length возвращает размер блочного устройства
func (bd *BlockDeviceDataSource) Length() (int, error) {
	var stat syscall.Stat_t
	if err := syscall.Stat(bd.devicePath, &stat); err != nil {
		return 0, fmt.Errorf("failed to stat block device: %v", err)
	}
	return int(stat.Size), nil
}

// ReadCloser возвращает io.ReadCloser для чтения данных с блочного устройства
func (bd *BlockDeviceDataSource) ReadCloser() (io.ReadCloser, error) {
	pipeReader, pipeWriter := io.Pipe()

	go func() {
		defer pipeWriter.Close()
		for data := range bd.dataChan {
			_, err := pipeWriter.Write(data)
			if err != nil {
				bd.errChan <- err
				return
			}
		}
	}()

	return pipeReader, nil
}

// Close закрывает файл блочного устройства и останавливает фоновое чтение
func (bd *BlockDeviceDataSource) Close() error {
	if bd.file != nil {
		close(bd.dataChan)
		<-bd.doneChan
		return bd.file.Close()
	}
	return nil
}
