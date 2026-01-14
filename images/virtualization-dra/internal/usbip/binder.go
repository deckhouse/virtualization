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
	"os"
	"path/filepath"
	"strings"
)

func NewUSBBinder() USBBinder {
	return &usbBinder{}
}

type usbBinder struct{}

// Bind binds the USB device to the USBIP server.
// https://github.com/torvalds/linux/blob/40fbbd64bba6c6e7a72885d2f59b6a3be9991eeb/tools/usb/usbip/src/usbip_bind.c#L130
func (b *usbBinder) Bind(busID string) error {
	devInfo, err := b.getUSBDeviceInfo(busID)
	if err != nil {
		return fmt.Errorf("device with bus ID %s does not exist: %w", busID, err)
	}

	if strings.Contains(devInfo.DevPath, "vhci_hcd") {
		return fmt.Errorf("bind loop detected: device %s is already attached to vhci_hcd", busID)
	}

	err = b.unbindOther(devInfo)
	if err != nil {
		return fmt.Errorf("failed to unbind other devices: %w", err)
	}

	if err = b.modifyMatchBusID(busID, true); err != nil {
		return err
	}

	if err = b.bindUsbip(busID); err != nil {
		return fmt.Errorf("failed to bind usb device: %w: %w", err, b.modifyMatchBusID(busID, false))
	}

	return nil
	// return b.storeBind(busID, true)
}

// Unbind unbinds the USB device from the USBIP server.
// https://github.com/torvalds/linux/blob/40fbbd64bba6c6e7a72885d2f59b6a3be9991eeb/tools/usb/usbip/src/usbip_unbind.c#L30
func (b *usbBinder) Unbind(busID string) error {
	devInfo, err := b.getUSBDeviceInfo(busID)
	if err != nil {
		return fmt.Errorf("device with bus ID %s does not exist: %w", busID, err)
	}

	if b.isBound(devInfo) {
		return fmt.Errorf("device %s is not bound to %s driver", devInfo.BusID, usbipHostDriverName)
	}

	if err = b.unbindUsbip(busID); err != nil {
		return fmt.Errorf("failed to unbind usb device %s: %w", busID, err)
	}

	// notify driver of unbind
	if err = b.modifyMatchBusID(busID, false); err != nil {
		return fmt.Errorf("failed to modify match bus ID %s: %w", busID, err)
	}

	// Trigger new probing
	if err = b.rebindUsbip(busID); err != nil {
		return fmt.Errorf("failed to rebind usb device %s: %w", busID, err)
	}

	return nil
	// return b.storeBind(busID, false)
}

func (b *usbBinder) IsBound(busID string) (bool, error) {
	devInfo, err := b.getUSBDeviceInfo(busID)
	if err != nil {
		return false, fmt.Errorf("device with bus ID %s does not exist: %w", busID, err)
	}
	return b.isBound(devInfo), nil
}

type usbDeviceInfo struct {
	BusID   string
	Driver  string
	DevPath string
	IsHub   bool
}

func (b *usbBinder) getUSBDeviceInfo(busID string) (*usbDeviceInfo, error) {
	path := getUSBDevicePath(busID)

	if _, err := os.Stat(path); err != nil {
		return nil, err
	}

	info := &usbDeviceInfo{
		BusID: busID,
	}

	bDevClassPath := filepath.Join(path, "bDeviceClass")
	if data, err := os.ReadFile(bDevClassPath); err == nil {
		info.IsHub = strings.TrimSpace(string(data)) == "09" // 09 = USB Hub class
	}

	driverLink := filepath.Join(path, "driver")
	if link, err := os.Readlink(driverLink); err == nil {
		info.Driver = filepath.Base(link)
	}

	ueventPath := filepath.Join(path, "uevent")
	if data, err := os.ReadFile(ueventPath); err == nil {
		lines := strings.Split(string(data), "\n")
		for _, line := range lines {
			if strings.HasPrefix(line, "DEVNAME=") {
				info.DevPath = filepath.Join("/dev", strings.TrimPrefix(line, "DEVNAME="))
				break
			}
		}
	}

	return info, nil
}

func (b *usbBinder) isBound(devInfo *usbDeviceInfo) bool {
	return devInfo.Driver == usbipHostDriverName
}

func (b *usbBinder) unbindOther(devInfo *usbDeviceInfo) error {
	if devInfo.IsHub {
		return fmt.Errorf("skip unbinding of hub %s", devInfo.BusID)
	}

	if devInfo.Driver == "" {
		// no driver bound to the device
		return nil
	}

	if b.isBound(devInfo) {
		return fmt.Errorf("device %s is already bound to %s", devInfo.BusID, usbipHostDriverName)
	}

	unbindPath := unbindAttrPath(devInfo.Driver)

	if err := writeSysfsAttr(unbindPath, busIDAttr{busID: devInfo.BusID}); err != nil {
		return fmt.Errorf("error unbinding device %s from driver %s: %w", devInfo.BusID, devInfo.Driver, err)
	}

	return nil
}

func (b *usbBinder) bindUsbip(busID string) error {
	return writeSysfsAttr(bindAttrPath(usbipHostDriverName), busIDAttr{busID: busID})
}

func (b *usbBinder) unbindUsbip(busID string) error {
	return writeSysfsAttr(unbindAttrPath(usbipHostDriverName), busIDAttr{busID: busID})
}

func (b *usbBinder) rebindUsbip(busID string) error {
	return writeSysfsAttr(rebindAttrPath(usbipHostDriverName), busIDAttr{busID: busID})
}

func (b *usbBinder) modifyMatchBusID(busID string, add bool) error {
	return writeSysfsAttr(matchBusIDAttrPath(usbipHostDriverName), modifyMatchBusIDAttr{busID: busID, add: add})
}

type modifyMatchBusIDAttr struct {
	busID string
	add   bool
}

func (a modifyMatchBusIDAttr) Complete() string {
	if a.add {
		return fmt.Sprintf("add %s", a.busID)
	}
	return fmt.Sprintf("del %s", a.busID)
}

type busIDAttr struct {
	busID string
}

func (a busIDAttr) Complete() string {
	return a.busID
}
