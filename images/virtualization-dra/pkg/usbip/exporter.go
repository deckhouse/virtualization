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

package usbip

import (
	"fmt"
	"log/slog"
	"net"
	"strconv"

	"github.com/deckhouse/virtualization-dra/pkg/usbip/protocol"
)

func NewUSBExporter() USBExporter {
	return &usbExporter{}
}

type usbExporter struct{}

func (e *usbExporter) Export(host, busID string, port int) error {
	conn, err := e.usbipNetTCPConnect(host, port)
	if err != nil {
		return fmt.Errorf("failed to connect to usbipd: %w", err)
	}
	defer func() {
		if err := conn.Close(); err != nil {
			slog.Error("failed to close connection", slog.String("error", err.Error()))
		}
	}()

	opCommon := protocol.NewOpCommon(protocol.OpReqExport, protocol.OpStatusOk)
	if err = opCommon.Encode(conn); err != nil {
		return fmt.Errorf("failed to encode OpCommon: %w", err)
	}

	exportReq := protocol.NewExportRequest(busID)
	if err = exportReq.Encode(conn); err != nil {
		return fmt.Errorf("failed to encode ExportRequest: %w", err)
	}

	exportReply := &protocol.ExportReply{}
	if err = exportReply.Decode(conn); err != nil {
		return fmt.Errorf("failed to decode ExportReply: %w", err)
	}

	if exportReply.Version != protocol.Version {
		return fmt.Errorf("unsupported USBIP version: %d", exportReply.Version)
	}

	if exportReply.Status != protocol.OpStatusOk {
		return fmt.Errorf("reply failed: %s", exportReply.Status.String())
	}

	return nil
}

func (e *usbExporter) Unexport(host, busID string, port int) error {
	conn, err := e.usbipNetTCPConnect(host, port)
	if err != nil {
		return fmt.Errorf("failed to connect to usbipd: %w", err)
	}
	defer func() {
		if err := conn.Close(); err != nil {
			slog.Error("failed to close connection", slog.String("error", err.Error()))
		}
	}()

	opCommon := protocol.NewOpCommon(protocol.OpReqUnexport, protocol.OpStatusOk)
	if err = opCommon.Encode(conn); err != nil {
		return fmt.Errorf("failed to encode OpCommon: %w", err)
	}

	unExportReq := protocol.NewUnExportRequest(busID)
	if err = unExportReq.Encode(conn); err != nil {
		return fmt.Errorf("failed to encode UnExportRequest: %w", err)
	}

	unExportReply := &protocol.UnExportReply{}
	if err = unExportReply.Decode(conn); err != nil {
		return fmt.Errorf("failed to decode UnExportReply: %w", err)
	}

	if unExportReply.Version != protocol.Version {
		return fmt.Errorf("unsupported USBIP version: %d", unExportReply.Version)
	}

	if unExportReply.Status != protocol.OpStatusOk {
		return fmt.Errorf("reply failed: %s", unExportReply.Status.String())
	}

	return nil
}

func (e *usbExporter) usbipNetTCPConnect(host string, port int) (*net.TCPConn, error) {
	tcpAddr, err := net.ResolveTCPAddr("tcp", net.JoinHostPort(host, strconv.Itoa(port)))
	if err != nil {
		return nil, fmt.Errorf("resolve TCP address: %w", err)
	}

	conn, err := net.DialTCP("tcp", nil, tcpAddr)
	if err != nil {
		return nil, fmt.Errorf("dial TCP: %w", err)
	}

	return conn, nil
}
