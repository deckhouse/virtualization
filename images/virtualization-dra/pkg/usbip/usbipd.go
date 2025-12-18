/*
Copyright 2025 Flant JSC

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

package usbip

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"os"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/deckhouse/virtualization-dra/pkg/libusb"
	"github.com/deckhouse/virtualization-dra/pkg/usbip/protocol"
)

const (
	defaultMaxTCPConnection        = 100
	defaultGracefulShutdownTimeout = 30 * time.Second
)

type Option func(usbipd *USBIPD)

func WithGracefulShutdownTimeout(timeout time.Duration) Option {
	return func(u *USBIPD) {
		u.gracefulShutdownTimeout = timeout
	}
}

func WithMaxTCPConnection(maxTCPConnection int) Option {
	return func(u *USBIPD) {
		u.maxTCPConnection = maxTCPConnection
	}
}

func WithExport(enabled bool) Option {
	return func(usbipd *USBIPD) {
		usbipd.exportEnabled = enabled
	}
}

func NewUSBIPD(addr string, monitor libusb.Monitor, opts ...Option) *USBIPD {
	usbipd := &USBIPD{
		addr:                    addr,
		monitor:                 monitor,
		gracefulShutdownTimeout: defaultGracefulShutdownTimeout,
		maxTCPConnection:        defaultMaxTCPConnection,
		logger:                  slog.Default().With(slog.String("component", "usbipd")),
		quit:                    make(chan struct{}),
	}

	for _, opt := range opts {
		opt(usbipd)
	}

	if usbipd.exportEnabled {
		usbipd.usbBinder = NewUSBBinder()
	}

	return usbipd
}

type USBIPD struct {
	addr                    string
	monitor                 libusb.Monitor
	gracefulShutdownTimeout time.Duration
	maxTCPConnection        int
	logger                  *slog.Logger
	exportEnabled           bool
	usbBinder               USBBinder

	listener  net.Listener
	connWg    sync.WaitGroup
	connCount atomic.Int64
	quit      chan struct{}
}

func (u *USBIPD) Start(ctx context.Context) error {
	if err := u.setup(); err != nil {
		return err
	}

	go func() {
		<-ctx.Done()
		close(u.quit)
		if u.listener != nil {
			if err := u.listener.Close(); err != nil {
				u.logger.Error("failed to close listener", slog.Any("error", err))
			}
		}
	}()

	u.connWg.Add(1)
	go u.run(ctx)

	return nil
}

func (u *USBIPD) Run(ctx context.Context) error {
	if err := u.setup(); err != nil {
		return err
	}

	go func() {
		<-ctx.Done()
		close(u.quit)
		if u.listener != nil {
			if err := u.listener.Close(); err != nil {
				u.logger.Error("failed to close listener", slog.Any("error", err))
			}
		}
	}()

	u.connWg.Add(1)
	u.run(ctx)

	if waitWithTimeout(&u.connWg, u.gracefulShutdownTimeout) {
		u.logger.Info("all connections closed")
	} else {
		u.logger.Warn("graceful shutdown timeout, some connections may be left open")
	}

	return nil
}

// waitWithTimeout waits for wg to complete; returns true if done before timeout.
func waitWithTimeout(wg *sync.WaitGroup, timeout time.Duration) bool {
	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()
	select {
	case <-done:
		return true
	case <-time.After(timeout):
		return false
	}
}

func (u *USBIPD) setup() (err error) {
	u.listener, err = net.Listen("tcp", u.addr)
	return err
}

func (u *USBIPD) run(ctx context.Context) {
	defer u.connWg.Done()
	for {
		conn, err := u.listener.Accept()
		// Error occurred when
		// 1. Connection error
		// 2. The listener is closed (e.g. on context cancellation)
		if err != nil {
			select {
			case <-u.quit:
				return
			default:
				u.logger.Error("unable to accept request", slog.String("address", u.addr), slog.Any("err", err))
			}
			continue
		}
		{
			// Check if TCP connection reached the limit specified in given config
			count := u.connCount.Load()
			if count+1 > int64(u.maxTCPConnection) {
				u.logger.Error("maximum TCP connection reached, drop the connection", slog.Int64("count", count))
				if err := conn.Close(); err != nil {
					u.logger.Error("failed to close connection", slog.String("error", err.Error()))
				}
				continue
			}

			// TCP connection handler
			u.connWg.Add(1)
			u.connCount.Add(1)
			go func() {
				defer u.connCount.Add(-1)
				defer u.connWg.Done()
				defer func() {
					if err := conn.Close(); err != nil {
						u.logger.Error("failed to close connection", slog.String("error", err.Error()))
					}
				}()

				u.logger.Info("new connection established", slog.String("addr", conn.RemoteAddr().String()))
				keepConn, err := u.handleConnection(conn)
				if err != nil {
					if !errors.Is(err, io.EOF) {
						u.logger.Error("failed to handle connection", slog.Any("err", err), slog.String("addr", conn.RemoteAddr().String()))
					} else {
						u.logger.Info("connection EOF", slog.String("addr", conn.RemoteAddr().String()))
					}
				}
				if keepConn {
					// don't handle and read from the socket. other work doing a kernel module
					<-ctx.Done()
				}
				u.logger.Info("connection closed", slog.String("addr", conn.RemoteAddr().String()))
			}()
		}
	}
}

// https://docs.kernel.org/usb/usbip_protocol.html
// https://github.com/torvalds/linux/blob/9448598b22c50c8a5bb77a9103e2d49f134c9578/tools/usb/usbip/src/usbipd.c#L251
func (u *USBIPD) handleConnection(conn net.Conn) (bool, error) {
	opCommon := &protocol.OpCommon{}
	if err := opCommon.Decode(conn); err != nil {
		return false, fmt.Errorf("failed to decode OpCommon: %w", err)
	}

	if opCommon.Version != protocol.Version {
		return false, fmt.Errorf("unsupported USBIP version: %d", opCommon.Version)
	}

	if opCommon.Status != protocol.OpStatusOk {
		return false, fmt.Errorf("request failed: %s", opCommon.Status.String())
	}

	switch opCommon.Code {
	case protocol.OpReqDevList:
		if err := u.handleDeviceList(conn); err != nil {
			return false, fmt.Errorf("failed to handle OpReqDevList: %w", err)
		}
	case protocol.OpReqImport:
		if err := u.handleImportRequest(conn); err != nil {
			return false, fmt.Errorf("failed to handle OpReqImport: %w", err)
		}
		return true, nil
	case protocol.OpReqExport:
		if err := u.handleExportRequest(conn); err != nil {
			return false, fmt.Errorf("failed to handle OpRepExport: %w", err)
		}
	case protocol.OpReqUnexport:
		if err := u.handleUnexportRequest(conn); err != nil {
			return false, fmt.Errorf("failed to handle OpReqUnexport: %w", err)
		}
	case protocol.OpReqDevInfo, protocol.OpReqCrypkey:
		// nothing to do
	default:
		return false, fmt.Errorf("unsupported OpCommon.Code: %d", opCommon.Code)
	}

	return false, nil
}

// https://github.com/torvalds/linux/blob/9448598b22c50c8a5bb77a9103e2d49f134c9578/tools/usb/usbip/src/usbipd.c#L229
func (u *USBIPD) handleDeviceList(conn net.Conn) error {
	info := u.getUSBDeviceInfo()
	if len(info) == 0 {
		slog.Info("no USB devices found")
	}
	devList := protocol.NewDeviceList(protocol.OpStatusOk, info)
	return devList.Encode(conn)
}

// https://github.com/torvalds/linux/blob/9448598b22c50c8a5bb77a9103e2d49f134c9578/tools/usb/usbip/src/usbipd.c#L91
func (u *USBIPD) handleImportRequest(conn net.Conn) error {
	importReq := &protocol.ImportRequest{}
	if err := importReq.Decode(conn); err != nil {
		return fmt.Errorf("failed to decode ImportRequest: %w", err)
	}

	busID := importReq.BusID()
	log := u.logger.With(slog.String("busID", busID))
	log.Info("import request")

	bindDevice, exists := u.monitor.GetDeviceByBusID(busID)
	if !exists {
		log.Info("USB device is not found")
		return protocol.NewImportReply(protocol.OpStatusNoDev, protocol.USBDevice{}).Encode(conn)
	}

	// should set TCP_NODELAY for usbip
	u.setNoDelay(conn)

	status := u.exportDevice(conn, bindDevice)
	if status != protocol.OpStatusOk {
		log.Error("failed to export device", slog.String("status", status.String()))
	} else {
		u.logger.Info("device exported", slog.Any("device", bindDevice))
	}

	usbDevice := toUSBDeviceInfo(bindDevice).USBDevice

	return protocol.NewImportReply(status, usbDevice).Encode(conn)
}

// https://github.com/torvalds/linux/blob/9448598b22c50c8a5bb77a9103e2d49f134c9578/tools/usb/usbip/libsrc/usbip_host_common.c#L212
func (u *USBIPD) exportDevice(conn net.Conn, device *libusb.USBDevice) protocol.OpStatus {
	log := u.logger.With(slog.String("busID", device.BusID))
	log.Info("export request")

	usbIpStatus, err := u.getUSBIPStatus(device)
	if err != nil {
		log.Error("failed to get USBIP status", slog.Any("error", err))
		return protocol.OpStatusError
	}

	if usbIpStatus != protocol.DeviceStatusAvailable {
		log.Info("USBIP status is not available")
		switch usbIpStatus {
		case protocol.DeviceStatusError:
			log.Debug("USBIP status is error")
			return protocol.OpStatusDevErr
		case protocol.DeviceStatusUsed:
			log.Debug("USBIP status is used")
			return protocol.OpStatusDevBusy
		default:
			log.Debug("USBIP status unknown")
			return protocol.OpStatusNA
		}
	}

	syscallConn, ok := conn.(syscall.Conn)
	if !ok {
		log.Error("conn does not implement syscall.Conn")
		return protocol.OpStatusNA
	}

	var sockFd int
	rawConn, err := syscallConn.SyscallConn()
	if err != nil {
		log.Error("failed to get raw connection", slog.Any("error", err))
		return protocol.OpStatusNA
	}
	err = rawConn.Control(func(fd uintptr) {
		sockFd = int(fd)
	})
	if err != nil {
		log.Error("failed to get socket fd", slog.Any("error", err))
		return protocol.OpStatusNA
	}

	err = writeSysfsAttr(usbipSockFdPath(device.BusID), sockFdAttr{sockFd: sockFd})
	if err != nil {
		log.Error("failed to write usbip_sockfd", slog.Any("error", err))
		return protocol.OpStatusNA
	}

	log.Info("Connect")

	return protocol.OpStatusOk
}

func (u *USBIPD) handleExportRequest(conn net.Conn) error {
	if !u.exportEnabled {
		u.logger.Info("USBIPD export is disabled, skip handle export request")
		return nil
	}

	exportRequest := &protocol.ExportRequest{}
	if err := exportRequest.Decode(conn); err != nil {
		return fmt.Errorf("failed to decode ExportRequest: %w", err)
	}

	busID := exportRequest.BusID()
	log := u.logger.With(slog.String("busID", busID))
	log.Info("export request")

	_, exists := u.monitor.GetDeviceByBusID(busID)
	if !exists {
		log.Info("USB device is not found")
		return protocol.NewExportReply(protocol.OpStatusNoDev).Encode(conn)
	}

	bound, err := u.usbBinder.IsBound(busID)
	if err != nil {
		log.Error("failed to check if USB device is bound", slog.Any("error", err))
		return protocol.NewExportReply(protocol.OpStatusError).Encode(conn)
	}

	if bound {
		log.Info("USB device is already bound")
		return protocol.NewExportReply(protocol.OpStatusOk).Encode(conn)
	}

	err = u.usbBinder.Bind(busID)
	if err != nil {
		log.Error("failed to bind USB device", slog.Any("error", err))
		return protocol.NewExportReply(protocol.OpStatusError).Encode(conn)
	}

	log.Info("USB device bound")
	return protocol.NewExportReply(protocol.OpStatusOk).Encode(conn)
}

func (u *USBIPD) handleUnexportRequest(conn net.Conn) error {
	if !u.exportEnabled {
		u.logger.Info("USBIPD export is disabled, skip handle unexport request")
		return nil
	}

	unexportRequest := &protocol.UnExportRequest{}
	if err := unexportRequest.Decode(conn); err != nil {
		return fmt.Errorf("failed to decode UnExportRequest: %w", err)
	}

	busID := unexportRequest.BusID()
	log := u.logger.With(slog.String("busID", busID))
	log.Info("unexport request")

	_, exists := u.monitor.GetDeviceByBusID(busID)
	if !exists {
		log.Info("USB device is not found")
		return protocol.NewUnExportReply(protocol.OpStatusNoDev).Encode(conn)
	}

	bound, err := u.usbBinder.IsBound(busID)
	if err != nil {
		log.Error("failed to check if USB device is bound", slog.Any("error", err))
		return protocol.NewUnExportReply(protocol.OpStatusError).Encode(conn)
	}

	if !bound {
		log.Info("USB device already unbound")
		return protocol.NewUnExportReply(protocol.OpStatusOk).Encode(conn)
	}

	err = u.usbBinder.Unbind(busID)
	if err != nil {
		log.Error("failed to unbind USB device", slog.Any("error", err))
		return protocol.NewUnExportReply(protocol.OpStatusError).Encode(conn)
	}

	log.Info("USB device unbound")
	return protocol.NewUnExportReply(protocol.OpStatusOk).Encode(conn)
}

type sockFdAttr struct {
	sockFd int
}

func (a sockFdAttr) Complete() string {
	return fmt.Sprintf("%d\n", a.sockFd)
}

func (u *USBIPD) getUSBIPStatus(device *libusb.USBDevice) (protocol.DeviceStatus, error) {
	statusPath := usbipStatusPath(device.BusID)

	data, err := os.ReadFile(statusPath)
	if err != nil {
		return 0, fmt.Errorf("failed to read %s: %w", statusPath, err)
	}

	statusStr := strings.TrimSpace(string(data))

	value, err := strconv.ParseUint(statusStr, 10, 32)
	if err != nil {
		return 0, fmt.Errorf("invalid status value %q: %w", statusStr, err)
	}

	status := protocol.DeviceStatus(value)

	return status, nil
}

func (u *USBIPD) setNoDelay(conn net.Conn) {
	tcpConn, ok := conn.(*net.TCPConn)
	if ok {
		err := tcpConn.SetNoDelay(true)
		if err != nil {
			u.logger.Error("failed to set TCP_NODELAY", slog.String("error", err.Error()))
		}
		return
	}
	u.logger.Error("failed to cast connection to TCPConn")
}

// TODO: check already used devices
func (u *USBIPD) getUSBDeviceInfo() []protocol.USBDeviceInfo {
	devices := u.monitor.GetDevices()

	var bindDevices []protocol.USBDeviceInfo

	for _, device := range devices {
		if device.Driver == usbipHostDriverName {
			bindDevice := toUSBDeviceInfo(&device)
			bindDevices = append(bindDevices, bindDevice)
		}
	}

	return bindDevices
}

func toUSBDeviceInfo(device *libusb.USBDevice) protocol.USBDeviceInfo {
	if device == nil {
		return protocol.USBDeviceInfo{}
	}
	return protocol.USBDeviceInfo{
		USBDevice: protocol.USBDevice{
			Path:                protocol.ToDevicePath(device.DevicePath),
			BusID:               protocol.ToBusID(device.BusID),
			Busnum:              device.Bus,
			Devnum:              device.DeviceNumber,
			Speed:               toSpeed(device.Speed),
			IDVendor:            device.VendorID,
			IDProduct:           device.ProductID,
			BcdDevice:           device.BCD,
			BDeviceClass:        device.BDeviceClass,
			BDeviceSubClass:     device.BDeviceSubClass,
			BDeviceProtocol:     device.BDeviceProtocol,
			BConfigurationValue: device.BConfigurationValue,
			BNumConfigurations:  device.BNumConfigurations,
			BNumInterfaces:      device.BNumInterfaces,
		},
		Interfaces: toInterfaces(device.Interfaces),
	}
}

func toInterfaces(interfaces []libusb.USBDeviceInterface) []protocol.USBDeviceInterface {
	result := make([]protocol.USBDeviceInterface, len(interfaces))
	for i, iface := range interfaces {
		result[i] = protocol.USBDeviceInterface{
			BInterfaceClass:    iface.BInterfaceClass,
			BInterfaceSubClass: iface.BInterfaceSubClass,
			BInterfaceProtocol: iface.BInterfaceProtocol,
		}
	}
	return result
}

func toSpeed(speed uint32) uint32 {
	return uint32(libusb.ResolveDeviceSpeed(speed))
}
