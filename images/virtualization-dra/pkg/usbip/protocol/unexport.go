/*
Copyright 2026 Flant JSC

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

func NewUnExportRequest(busID string) *UnExportRequest {
	return &UnExportRequest{
		busID: ToBusID(busID),
	}
}

type UnExportRequest struct {
	busID [sysfsBusIdMax]byte
}

func (i *UnExportRequest) BusID() string {
	return fromCString(i.busID[:])
}

func (i *UnExportRequest) Encode(w io.Writer) error {
	_, err := w.Write(i.busID[:])
	return err
}

func (i *UnExportRequest) Decode(r io.Reader) error {
	buf := make([]byte, sysfsBusIdMax)
	_, err := io.ReadFull(r, buf)
	if err != nil {
		return fmt.Errorf("failed to read UnExportRequest from reader: %w", err)
	}

	copy(i.busID[:], buf)
	return nil
}

type UnExportReply struct {
	OpCommon
}

func NewUnExportReply(status OpStatus) *UnExportReply {
	return &UnExportReply{
		OpCommon: OpCommon{
			Version: Version,
			Code:    OpRepExport,
			Status:  status,
		},
	}
}

func (i *UnExportReply) Encode(w io.Writer) error {
	if err := i.OpCommon.Encode(w); err != nil {
		return fmt.Errorf("failed to encode OpCommon: %w", err)
	}
	return nil
}

func (i *UnExportReply) Decode(r io.Reader) error {
	if err := i.OpCommon.Decode(r); err != nil {
		return fmt.Errorf("failed to decode OpCommon: %w", err)
	}
	return nil
}
