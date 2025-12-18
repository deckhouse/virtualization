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
	"fmt"
	"log/slog"
	"net"
	"os"
	"strconv"
	"sync"
	"syscall"

	"github.com/deckhouse/virtualization-dra/pkg/libusb"
	"github.com/deckhouse/virtualization-dra/pkg/usbip/protocol"
)

func NewUSBAttacher() USBAttacher {
	return &usbAttacher{}
}

type usbAttacher struct {
	mu sync.Mutex
}

// https://github.com/torvalds/linux/blob/b927546677c876e26eba308550207c2ddf812a43/tools/usb/usbip/src/usbip_attach.c#L174
func (a *usbAttacher) Attach(host, busID string, port int) (int, error) {
	a.mu.Lock()
	defer a.mu.Unlock()

	conn, err := a.usbipNetTCPConnect(host, strconv.Itoa(port))
	if err != nil {
		return -1, fmt.Errorf("failed to connect to usbipd: %w", err)
	}

	rhport, err := a.queryImportDevice(conn, busID)
	if err != nil {
		return -1, fmt.Errorf("failed to query import device: %w", err)
	}

	err = a.recordConnection(host, strconv.Itoa(port), busID, rhport)
	if err != nil {
		return -1, fmt.Errorf("failed to record connection: %w", err)
	}

	return rhport, nil
}

// https://github.com/torvalds/linux/blob/b927546677c876e26eba308550207c2ddf812a43/tools/usb/usbip/src/usbip_detach.c#L32
func (a *usbAttacher) Detach(rhport int) error {
	a.mu.Lock()
	defer a.mu.Unlock()

	driver, err := newVhciDriver()
	if err != nil {
		return fmt.Errorf("failed to get vhci driver: %w", err)
	}

	found := false
	for i := 0; i < driver.nports; i++ {
		idev := &driver.idevs[i]

		if idev.port == rhport {
			found = true
			vstatus := protocol.DeviceStatus(idev.status)
			if vstatus == protocol.VDeviceStatusNull {
				slog.Info("Port is already detached", slog.Int("rhport", rhport))
				return fmt.Errorf("port is already detached")
			}

			break
		}
	}

	if !found {
		slog.Error("Invalid port > maxports", slog.Int("rhport", rhport), slog.Int("maxports", driver.nports))
		return fmt.Errorf("rhport %d not found", rhport)
	}

	path := vhciStatePortPath(rhport)

	if err = os.Remove(path); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to remove vhci state port file: %w", err)
	}

	if err = os.RemoveAll(vhciStatePath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to remove vhci state path: %w", err)
	}

	if err = writeSysfsAttr(vhciHcdDetach, detachAttr{port: rhport}); err != nil {
		return fmt.Errorf("failed to write detach attribute: %w", err)
	}

	slog.Info("Port detached", slog.Int("rhport", rhport))
	return nil
}

func (a *usbAttacher) GetAttachInfo() ([]AttachInfo, error) {
	driver, err := newVhciDriver()
	if err != nil {
		return nil, fmt.Errorf("failed to get vhci driver: %w", err)
	}

	var usedInfos []AttachInfo

	for i := 0; i < driver.nports; i++ {
		idev := &driver.idevs[i]

		vstatus := protocol.DeviceStatus(idev.status)
		if vstatus == protocol.VDeviceStatusUsed {
			usedInfos = append(usedInfos, AttachInfo{
				Port:       idev.port,
				Busnum:     idev.busnum,
				Devnum:     idev.devnum,
				LocalBusID: idev.localBusID,
			})
		}
	}

	return usedInfos, nil
}

// https://github.com/torvalds/linux/blob/b927546677c876e26eba308550207c2ddf812a43/tools/usb/usbip/src/usbip_network.c#L261
func (a *usbAttacher) usbipNetTCPConnect(host, port string) (*net.TCPConn, error) {
	tcpAddr, err := net.ResolveTCPAddr("tcp", net.JoinHostPort(host, port))
	if err != nil {
		return nil, fmt.Errorf("resolve TCP address: %w", err)
	}

	conn, err := net.DialTCP("tcp", nil, tcpAddr)
	if err != nil {
		return nil, fmt.Errorf("dial TCP: %w", err)
	}

	if err := conn.SetNoDelay(true); err != nil {
		if conErr := conn.Close(); conErr != nil {
			slog.Error("failed to close connection", slog.String("error", conErr.Error()))
		}
		return nil, fmt.Errorf("set TCP_NODELAY: %w", err)
	}

	if err := conn.SetKeepAlive(true); err != nil {
		if conErr := conn.Close(); conErr != nil {
			slog.Error("failed to close connection", slog.String("error", conErr.Error()))
		}
		return nil, fmt.Errorf("set keepalive: %w", err)
	}

	return conn, nil
}

// https://github.com/torvalds/linux/blob/b927546677c876e26eba308550207c2ddf812a43/tools/usb/usbip/src/usbip_attach.c#L120
func (a *usbAttacher) queryImportDevice(conn *net.TCPConn, busID string) (int, error) {
	opCommon := protocol.NewOpCommon(protocol.OpReqImport, protocol.OpStatusOk)
	importReq := protocol.NewImportRequest(busID)

	if err := opCommon.Encode(conn); err != nil {
		return -1, fmt.Errorf("failed to encode OpCommon: %w", err)
	}

	if err := importReq.Encode(conn); err != nil {
		return -1, fmt.Errorf("failed to encode ImportRequest: %w", err)
	}

	importReply := &protocol.ImportReply{}
	if err := importReply.Decode(conn); err != nil {
		return -1, fmt.Errorf("failed to decode ImportReply: %w", err)
	}

	if importReply.Version != protocol.Version {
		return -1, fmt.Errorf("unsupported USBIP version: %d", importReply.Version)
	}

	if importReply.Status != protocol.OpStatusOk {
		return -1, fmt.Errorf("reply failed: %s", importReply.Status.String())
	}

	if importReply.GetBusID() != busID {
		return -1, fmt.Errorf("busID mismatch: %s != %s", importReply.GetBusID(), busID)
	}

	return a.importDevice(conn, importReply.USBDevice)
}

// https://github.com/torvalds/linux/blob/b927546677c876e26eba308550207c2ddf812a43/tools/usb/usbip/src/usbip_attach.c#L81
func (a *usbAttacher) importDevice(conn *net.TCPConn, usbDevice protocol.USBDevice) (int, error) {
	port, err := a.getFreePort(usbDevice.Speed)
	if err != nil {
		return -1, fmt.Errorf("failed to get free port: %w", err)
	}

	sockFd, err := a.getSockFd(conn)
	if err != nil {
		return -1, fmt.Errorf("failed to get socket fd: %w", err)
	}

	devID := getDevId(usbDevice.Busnum, usbDevice.Devnum)

	attr := attachAttr{
		port:   port,
		sockFd: sockFd,
		devId:  devID,
		speed:  usbDevice.Speed,
	}

	err = writeSysfsAttr(vhciHcdAttach, attr)
	if err != nil {
		return -1, fmt.Errorf("failed to write attach attribute: %w", err)
	}

	return port, nil
}

// https://github.com/torvalds/linux/blob/b927546677c876e26eba308550207c2ddf812a43/tools/usb/usbip/libsrc/vhci_driver.c#L334
func (a *usbAttacher) getFreePort(speed uint32) (int, error) {
	driver, err := newVhciDriver()
	if err != nil {
		return -1, err
	}

	deviceSpeed := libusb.USBDeviceSpeed(speed)

	for i := 0; i < driver.nports; i++ {
		switch deviceSpeed {
		case libusb.USBDeviceSpeedSuper:
			if driver.idevs[i].hub != hubSpeedSuper {
				continue
			}
		default:
			if driver.idevs[i].hub != hubSpeedHigh {
				continue
			}
		}
		vstatus := protocol.DeviceStatus(driver.idevs[i].status)
		if vstatus == protocol.VDeviceStatusNull {
			return driver.idevs[i].port, nil
		}
	}

	return -1, nil
}

func (a *usbAttacher) getSockFd(conn *net.TCPConn) (int, error) {
	file, err := conn.File()
	if err != nil {
		return -1, err
	}
	defer file.Close()

	fd := int(file.Fd())

	newFd, err := syscall.Dup(fd)
	if err != nil {
		return -1, err
	}

	return newFd, nil
}

// https://github.com/torvalds/linux/blob/b927546677c876e26eba308550207c2ddf812a43/tools/usb/usbip/src/usbip_attach.c#L39
func (a *usbAttacher) recordConnection(host, port, busID string, rhport int) error {
	err := os.MkdirAll(vhciStatePath, 0o700)
	if err != nil {
		return fmt.Errorf("failed to create vhci state path: %w", err)
	}

	path := vhciStatePortPath(rhport)

	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o700)
	if err != nil {
		return fmt.Errorf("failed to open vhci state port file: %w", err)
	}
	defer file.Close()

	value := fmt.Sprintf("%s %s %s", host, port, busID)

	_, err = file.WriteString(value)
	if err != nil {
		return fmt.Errorf("failed to write vhci state port file: %w", err)
	}

	return nil
}

type attachAttr struct {
	port   int
	sockFd int
	devId  int
	speed  uint32
}

func (a attachAttr) Complete() string {
	return fmt.Sprintf("%d %d %d %d", a.port, a.sockFd, a.devId, a.speed)
}

type detachAttr struct {
	port int
}

func (a detachAttr) Complete() string {
	return fmt.Sprintf("%d", a.port)
}
