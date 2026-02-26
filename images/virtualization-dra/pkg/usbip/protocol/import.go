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
	"fmt"
	"io"
)

func NewImportRequest(busID string) *ImportRequest {
	return &ImportRequest{
		busID: ToBusID(busID),
	}
}

type ImportRequest struct {
	busID [sysfsBusIdMax]byte
}

func (i *ImportRequest) BusID() string {
	return fromCString(i.busID[:])
}

func (i *ImportRequest) Encode(w io.Writer) error {
	_, err := w.Write(i.busID[:])
	return err
}

func (i *ImportRequest) Decode(r io.Reader) error {
	buf := make([]byte, sysfsBusIdMax)
	_, err := io.ReadFull(r, buf)
	if err != nil {
		return fmt.Errorf("failed to read ImportRequest from reader: %w", err)
	}

	copy(i.busID[:], buf)
	return nil
}

type ImportReply struct {
	OpCommon
	USBDevice
}

func NewImportReply(status OpStatus, device USBDevice) *ImportReply {
	return &ImportReply{
		OpCommon: OpCommon{
			Version: Version,
			Code:    OpRepImport,
			Status:  status,
		},
		USBDevice: device,
	}
}

func (i *ImportReply) Encode(w io.Writer) error {
	if err := i.OpCommon.Encode(w); err != nil {
		return fmt.Errorf("failed to encode OpCommon: %w", err)
	}
	if err := i.USBDevice.Encode(w); err != nil {
		return fmt.Errorf("failed to encode USBDevice: %w", err)
	}
	return nil
}

func (i *ImportReply) Decode(r io.Reader) error {
	if err := i.OpCommon.Decode(r); err != nil {
		return fmt.Errorf("failed to decode OpCommon: %w", err)
	}
	if err := i.USBDevice.Decode(r); err != nil {
		return fmt.Errorf("failed to decode USBDevice: %w", err)
	}
	return nil
}
