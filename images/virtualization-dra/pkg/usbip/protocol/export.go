package protocol

import (
	"fmt"
	"io"
)

func NewExportRequest(busID string) *ExportRequest {
	return &ExportRequest{
		busID: ToBusID(busID),
	}
}

type ExportRequest struct {
	busID [sysfsBusIdMax]byte
}

func (i *ExportRequest) BusID() string {
	return fromCString(i.busID[:])
}

func (i *ExportRequest) Encode(w io.Writer) error {
	_, err := w.Write(i.busID[:])
	return err
}

func (i *ExportRequest) Decode(r io.Reader) error {
	buf := make([]byte, sysfsBusIdMax)
	_, err := io.ReadFull(r, buf)
	if err != nil {
		return fmt.Errorf("failed to read ExportRequest from reader: %w", err)
	}

	copy(i.busID[:], buf)
	return nil
}

type ExportReply struct {
	OpCommon
}

func NewExportReply(status OpStatus) *ExportReply {
	return &ExportReply{
		OpCommon: OpCommon{
			Version: Version,
			Code:    OpRepExport,
			Status:  status,
		},
	}
}

func (i *ExportReply) Encode(w io.Writer) error {
	if err := i.OpCommon.Encode(w); err != nil {
		return fmt.Errorf("failed to encode OpCommon: %w", err)
	}
	return nil
}

func (i *ExportReply) Decode(r io.Reader) error {
	if err := i.OpCommon.Decode(r); err != nil {
		return fmt.Errorf("failed to decode OpCommon: %w", err)
	}
	return nil
}
