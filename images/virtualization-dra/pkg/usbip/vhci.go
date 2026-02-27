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
	"bytes"
	"fmt"
	"os"
	"strconv"
	"strings"
)

const (
	vhciStatePath          = "/var/run/vhci_hcd"
	platformPath           = "/sys/devices/platform"
	usbipVhciHcdNPortsPath = "/sys/devices/platform/vhci_hcd.0/nports"

	vhciHcdAttach              = "/sys/devices/platform/vhci_hcd.0/attach"
	vhciHcdDetach              = "/sys/devices/platform/vhci_hcd.0/detach"
	vhciHcdStatus              = "/sys/devices/platform/vhci_hcd.0/status"
	secondaryVhciHcdStatusTmpl = "/sys/devices/platform/vhci_hcd.%d/status.%d"

	vhciStatePortTmpl = "/var/run/vhci_hcd/port%d"
)

func vhciStatePortPath(port int) string {
	return fmt.Sprintf(vhciStatePortTmpl, port)
}

func secondaryVhciHcdStatusPath(count int) string {
	return fmt.Sprintf(secondaryVhciHcdStatusTmpl, count, count)
}

type vhciDriver struct {
	nports       int
	ncontrollers int
	idevs        []importDevice
}

type importDevice struct {
	hub                                 hubSpeed
	port, status, devID, busnum, devnum int
	localBusID                          string
}

type hubSpeed int

const (
	hubSpeedHigh hubSpeed = iota
	hubSpeedSuper
)

// https://github.com/torvalds/linux/blob/b927546677c876e26eba308550207c2ddf812a43/tools/usb/usbip/libsrc/vhci_driver.c#L243
func newVhciDriver() (*vhciDriver, error) {
	nports, err := getNPorts()
	if err != nil {
		return nil, err
	}
	ncontrollers, err := getNControllers()
	if err != nil {
		return nil, err
	}

	driver := &vhciDriver{
		nports:       nports,
		ncontrollers: ncontrollers,
	}

	err = driver.refreshImportDeviceList()
	if err != nil {
		return nil, fmt.Errorf("failed to refresh import device list: %w", err)
	}

	return driver, nil
}

func getNPorts() (int, error) {
	data, err := os.ReadFile(usbipVhciHcdNPortsPath)
	if err != nil {
		return -1, err
	}

	nports, err := strconv.Atoi(strings.TrimSpace(string(data)))
	if err != nil {
		return -1, err
	}

	return nports, nil
}

func getNControllers() (int, error) {
	entries, err := os.ReadDir(platformPath)
	if err != nil {
		return -1, err
	}
	count := 0
	for _, entry := range entries {
		if entry.IsDir() && strings.HasPrefix(entry.Name(), "vhci_hcd") {
			count++
		}
	}

	return count, nil
}

// https://github.com/torvalds/linux/blob/b927546677c876e26eba308550207c2ddf812a43/tools/usb/usbip/libsrc/vhci_driver.c#L111
func (d *vhciDriver) refreshImportDeviceList() error {
	status := vhciHcdStatus

	for i := 0; i < d.ncontrollers; i++ {
		if i > 0 {
			status = secondaryVhciHcdStatusPath(i)
		}

		attrStatus, err := os.ReadFile(status)
		if err != nil {
			return fmt.Errorf("failed to read %s: %w", status, err)
		}

		err = d.parseStatus(attrStatus)
		if err != nil {
			return fmt.Errorf("failed to parse attr status %s: %w", status, err)
		}
	}

	return nil
}

// https://github.com/torvalds/linux/blob/b927546677c876e26eba308550207c2ddf812a43/tools/usb/usbip/libsrc/vhci_driver.c#L40
func (d *vhciDriver) parseStatus(statusBytes []byte) error {
	lines := strings.Split(string(statusBytes), "\n")

	// hub port sta spd dev      sockfd local_busid
	// hs  0000 004 000 00000000 000000 0-0
	// hs  0001 004 000 00000000 000000 0-0
	// hs  0002 004 000 00000000 000000 0-0

	head := true
	for _, line := range lines {
		if head {
			// skip header
			head = false
			continue
		}

		if strings.TrimSpace(line) == "" {
			continue
		}

		var (
			hub                                string
			port, status, speed, devID, sockFd int
			localBusID                         string
		)

		buf := bytes.NewBufferString(line)
		_, err := fmt.Fscanf(buf, "%2s %d %d %d %x %d %31s", &hub, &port, &status, &speed, &devID, &sockFd, &localBusID)
		if err != nil {
			return fmt.Errorf("failed to parse status: %w", err)
		}

		if len(d.idevs) <= port {
			idevs := make([]importDevice, port+1)
			copy(idevs, d.idevs)
			d.idevs = idevs
		}

		busnum, devnum := getBusNumDevNum(devID)

		idev := &d.idevs[port]

		idev.port = port
		idev.status = status
		idev.devID = devID
		idev.busnum = busnum
		idev.devnum = devnum
		idev.localBusID = localBusID

		switch hub {
		case "hs":
			idev.hub = hubSpeedHigh
		case "ss":
			idev.hub = hubSpeedSuper
		}
	}

	return nil
}

func getDevId(busnum, devnum uint32) int {
	return int((busnum << 16) | devnum)
}

func getBusNumDevNum(devID int) (int, int) {
	busnum := devID >> 16
	devnum := devID & 0x0000ffff

	return busnum, devnum
}
