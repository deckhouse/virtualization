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

package protocol

import (
	"encoding/binary"
	"fmt"
	"io"
)

func NewDeviceList(status OpStatus, devices []USBDeviceInfo) *DeviceList {
	return &DeviceList{
		OpCommon: OpCommon{
			Version: Version,
			Code:    OpReqDevList,
			Status:  status,
		},
		Ndev:    uint32(len(devices)),
		Devices: devices,
	}
}

type DeviceList struct {
	OpCommon

	Ndev    uint32
	Devices []USBDeviceInfo
}

func (d *DeviceList) Encode(w io.Writer) error {
	if err := d.OpCommon.Encode(w); err != nil {
		return fmt.Errorf("failed to encode OpCommon: %w", err)
	}

	buf := make([]byte, 4)
	binary.BigEndian.PutUint32(buf[0:4], d.Ndev)

	if _, err := w.Write(buf); err != nil {
		return fmt.Errorf("failed to write Ndev to writer: %w", err)
	}

	for _, dev := range d.Devices {
		if err := dev.Encode(w); err != nil {
			return fmt.Errorf("failed to encode USBDeviceInfo: %w", err)
		}
	}

	return nil
}

const (
	sysfsPathMax  = 256
	sysfsBusIdMax = 32
)

type USBDeviceInfo struct {
	USBDevice
	Interfaces []USBDeviceInterface
}

func (d *USBDeviceInfo) Decode(r io.Reader) error {
	if err := d.USBDevice.Decode(r); err != nil {
		return fmt.Errorf("unable to decode USBDevice: %w", err)
	}

	d.Interfaces = make([]USBDeviceInterface, d.BNumInterfaces)
	for i := 0; i < int(d.BNumInterfaces); i++ {
		if err := d.Interfaces[i].Decode(r); err != nil {
			return fmt.Errorf("unable to decode USBDeviceInterface: %w", err)
		}
	}

	return nil
}

func (d *USBDeviceInfo) Encode(w io.Writer) error {
	if err := d.USBDevice.Encode(w); err != nil {
		return fmt.Errorf("unable to encode USBDevice: %w", err)
	}

	for _, iface := range d.Interfaces {
		if err := iface.Encode(w); err != nil {
			return fmt.Errorf("unable to encode USBDeviceInterface: %w", err)
		}
	}

	return nil
}

type USBDevice struct {
	Path  [sysfsPathMax]byte
	BusID [sysfsBusIdMax]byte

	Busnum uint32
	Devnum uint32
	Speed  uint32

	IDVendor  uint16
	IDProduct uint16
	BcdDevice uint16

	BDeviceClass        uint8
	BDeviceSubClass     uint8
	BDeviceProtocol     uint8
	BConfigurationValue uint8
	BNumConfigurations  uint8
	BNumInterfaces      uint8
}

func (u *USBDevice) GetPath() string {
	return fromCString(u.Path[:])
}

func (u *USBDevice) GetBusID() string {
	return fromCString(u.BusID[:])
}

func (u *USBDevice) Decode(r io.Reader) error {
	buf := make([]byte, sysfsPathMax+sysfsBusIdMax+12+6+6)
	_, err := io.ReadFull(r, buf)
	if err != nil {
		return fmt.Errorf("failed to read USBDevice from reader: %w", err)
	}

	copy(u.Path[:], buf[0:sysfsPathMax])
	copy(u.BusID[:], buf[sysfsPathMax:sysfsPathMax+sysfsBusIdMax])

	pass := sysfsPathMax + sysfsBusIdMax

	u.Busnum = binary.BigEndian.Uint32(buf[pass : pass+4])
	pass += 4
	u.Devnum = binary.BigEndian.Uint32(buf[pass : pass+4])
	pass += 4
	u.Speed = binary.BigEndian.Uint32(buf[pass : pass+4])
	pass += 4

	u.IDVendor = binary.BigEndian.Uint16(buf[pass : pass+2])
	pass += 2
	u.IDProduct = binary.BigEndian.Uint16(buf[pass : pass+2])
	pass += 2
	u.BcdDevice = binary.BigEndian.Uint16(buf[pass : pass+2])
	pass += 2

	u.BDeviceClass = buf[pass]
	pass += 1
	u.BDeviceSubClass = buf[pass]
	pass += 1
	u.BDeviceProtocol = buf[pass]
	pass += 1
	u.BConfigurationValue = buf[pass]
	pass += 1
	u.BNumConfigurations = buf[pass]
	pass += 1
	u.BNumInterfaces = buf[pass]

	return nil
}

func (u *USBDevice) Encode(w io.Writer) error {
	buf := make([]byte, sysfsPathMax+sysfsBusIdMax+12+6+6)

	copy(buf[0:sysfsPathMax], u.Path[:])
	copy(buf[sysfsPathMax:sysfsPathMax+sysfsBusIdMax], u.BusID[:])

	pass := sysfsPathMax + sysfsBusIdMax

	binary.BigEndian.PutUint32(buf[pass:pass+4], u.Busnum)
	pass += 4
	binary.BigEndian.PutUint32(buf[pass:pass+4], u.Devnum)
	pass += 4
	binary.BigEndian.PutUint32(buf[pass:pass+4], u.Speed)
	pass += 4

	binary.BigEndian.PutUint16(buf[pass:pass+2], u.IDVendor)
	pass += 2
	binary.BigEndian.PutUint16(buf[pass:pass+2], u.IDProduct)
	pass += 2
	binary.BigEndian.PutUint16(buf[pass:pass+2], u.BcdDevice)
	pass += 2

	buf[pass] = u.BDeviceClass
	pass += 1
	buf[pass] = u.BDeviceSubClass
	pass += 1
	buf[pass] = u.BDeviceProtocol
	pass += 1
	buf[pass] = u.BConfigurationValue
	pass += 1
	buf[pass] = u.BNumConfigurations
	pass += 1
	buf[pass] = u.BNumInterfaces

	_, err := w.Write(buf)
	if err != nil {
		return fmt.Errorf("failed to write USBDevice to writer: %w", err)
	}
	return nil
}

type USBDeviceInterface struct {
	BInterfaceClass    uint8
	BInterfaceSubClass uint8
	BInterfaceProtocol uint8
	padding            uint8
}

func (u *USBDeviceInterface) Decode(r io.Reader) error {
	buf := make([]byte, 4)
	_, err := io.ReadFull(r, buf)
	if err != nil {
		return fmt.Errorf("failed to read USBDeviceInterface from reader: %w", err)
	}

	u.BInterfaceClass = buf[0]
	u.BInterfaceSubClass = buf[1]
	u.BInterfaceProtocol = buf[2]
	u.padding = buf[3]

	return nil
}

func (u *USBDeviceInterface) Encode(w io.Writer) error {
	buf := make([]byte, 4)

	buf[0] = u.BInterfaceClass
	buf[1] = u.BInterfaceSubClass
	buf[2] = u.BInterfaceProtocol
	buf[3] = u.padding

	_, err := w.Write(buf)
	if err != nil {
		return fmt.Errorf("failed to write USBDeviceInterface to writer: %w", err)
	}
	return nil
}
