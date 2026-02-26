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
)

type sysfsAttr interface {
	Complete() string
}

func writeSysfsAttr(attrPath string, value sysfsAttr) error {
	f, err := os.OpenFile(attrPath, os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()

	_, err = f.WriteString(value.Complete())
	return err
}

const (
	bindAttrPathTmpl       = "/sys/bus/usb/drivers/%s/bind"
	unbindAttrPathTmpl     = "/sys/bus/usb/drivers/%s/unbind"
	rebindAttrPathTmpl     = "/sys/bus/usb/drivers/%s/rebind"
	matchBusIDAttrPathTmpl = "/sys/bus/usb/drivers/%s/match_busid"

	usbDevicesTmpl = "/sys/bus/usb/devices/%s"

	usbipStatusPathTmpl = "/sys/bus/usb/devices/%s/usbip_status"
	usbipSockFdPathTmpl = "/sys/bus/usb/devices/%s/usbip_sockfd"

	usbipHostDriverName = "usbip-host"
)

func getUSBDevicePath(busID string) string {
	return fmt.Sprintf(usbDevicesTmpl, busID)
}

func bindAttrPath(driver string) string {
	return fmt.Sprintf(bindAttrPathTmpl, driver)
}

func unbindAttrPath(driver string) string {
	return fmt.Sprintf(unbindAttrPathTmpl, driver)
}

func rebindAttrPath(driver string) string {
	return fmt.Sprintf(rebindAttrPathTmpl, driver)
}

func matchBusIDAttrPath(driver string) string {
	return fmt.Sprintf(matchBusIDAttrPathTmpl, driver)
}

func usbipStatusPath(busID string) string {
	return fmt.Sprintf(usbipStatusPathTmpl, busID)
}

func usbipSockFdPath(busID string) string {
	return fmt.Sprintf(usbipSockFdPathTmpl, busID)
}
