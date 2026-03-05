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

// https://github.com/torvalds/linux/blob/master/tools/usb/usbip/src/usbip_network.h
package protocol

import (
	"encoding/binary"
	"fmt"
	"io"
)

type USBVersion uint16

const (
	Version USBVersion = 0x0111
)

type Op uint16

// Common header for all the kinds of PDUs.
const (
	OpRequest Op = 0x80 << 8
	OpReply   Op = 0x00 << 8
)

// Dummy Code
const (
	OpUnspec    Op = 0x00
	OpReqUnspec Op = OpRequest | OpUnspec
	OpRepUnspec Op = OpReply | OpUnspec
)

// Retrieve USB device information. (still not used)
const (
	OpDevInfo    Op = 0x02
	OpReqDevInfo Op = OpRequest | OpDevInfo
	OpRepDevInfo Op = OpReply | OpDevInfo
)

// Import a remote USB device.
const (
	OpImport    Op = 0x03
	OpReqImport Op = OpRequest | OpImport
	OpRepImport Op = OpReply | OpImport
)

// Negotiate IPSec encryption key. (still not used)
const (
	OpCrypkey    Op = 0x04
	OpReqCrypkey Op = OpRequest | OpCrypkey
	OpRepCrypkey Op = OpReply | OpCrypkey
)

// Retrieve the list of exported USB devices.
const (
	OpDevList    Op = 0x05
	OpReqDevList Op = OpRequest | OpDevList
	OpRepDevList Op = OpReply | OpDevList
)

// Export a USB device to a remote host.
const (
	OpExport    Op = 0x06
	OpReqExport Op = OpRequest | OpExport
	OpRepExport Op = OpReply | OpExport
)

// un-Export a USB device from a remote host.
const (
	OpUnexport    Op = 0x07
	OpReqUnexport Op = OpRequest | OpUnexport
	OpRepUnexport Op = OpReply | OpUnexport
)

type OpStatus uint32

const (
	OpStatusOk      OpStatus = 0x00
	OpStatusNA      OpStatus = 0x01
	OpStatusDevBusy OpStatus = 0x02
	OpStatusDevErr  OpStatus = 0x03
	OpStatusNoDev   OpStatus = 0x04
	OpStatusError   OpStatus = 0x05
)

func (o OpStatus) String() string {
	switch o {
	case OpStatusOk:
		return "OK"
	case OpStatusNA:
		return "NA"
	case OpStatusDevBusy:
		return "DevBusy"
	case OpStatusDevErr:
		return "DevErr"
	case OpStatusNoDev:
		return "NoDev"
	case OpStatusError:
		return "Error"
	default:
		return "Unknown"
	}
}

type DeviceStatus uint32

const (
	DeviceStatusAvailable DeviceStatus = iota + 0x01
	DeviceStatusUsed
	DeviceStatusError
	VDeviceStatusNull
	VDeviceStatusNotAssigned
	VDeviceStatusUsed
	VDeviceStatusError
)

func NewOpCommon(code Op, status OpStatus) *OpCommon {
	return &OpCommon{
		Version: Version,
		Code:    code,
		Status:  status,
	}
}

type OpCommon struct {
	Version USBVersion
	Code    Op
	Status  OpStatus
}

func (op *OpCommon) Decode(r io.Reader) error {
	buf := make([]byte, 8)
	_, err := io.ReadFull(r, buf)
	if err != nil {
		return fmt.Errorf("failed to read OpCommon: %w", err)
	}

	op.Version = USBVersion(binary.BigEndian.Uint16(buf[0:2]))
	op.Code = Op(binary.BigEndian.Uint16(buf[2:4]))
	op.Status = OpStatus(binary.BigEndian.Uint32(buf[4:8]))
	return nil
}

func (op *OpCommon) Encode(w io.Writer) error {
	buf := make([]byte, 8)
	binary.BigEndian.PutUint16(buf[0:2], uint16(op.Version))
	binary.BigEndian.PutUint16(buf[2:4], uint16(op.Code))
	binary.BigEndian.PutUint32(buf[4:8], uint32(op.Status))
	_, err := w.Write(buf)
	return err
}
