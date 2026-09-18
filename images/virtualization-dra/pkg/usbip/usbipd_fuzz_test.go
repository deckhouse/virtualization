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
	"bytes"
	"encoding/binary"
	"log/slog"
	"net"
	"testing"
	"time"

	"github.com/deckhouse/virtualization-dra/pkg/libusb"
	"github.com/deckhouse/virtualization-dra/pkg/usbip/protocol"
)

const (
	// Past the longest request: an eight byte header plus a 32 byte bus ID.
	fuzzMaxInput = 8 << 10

	fuzzMaxReply = 64 << 10

	// The only device the stub monitor holds. Not a bus ID the kernel could
	// produce, so the sysfs path built from it names nothing on the machine
	// running the fuzzer and export stops before anything is written. Do not
	// make it plausible, like 1-1.
	fuzzKnownBusID = "fuzz-no-such-device"
)

// FuzzUSBIPDHandleConnection drives the request handling of the USB/IP daemon:
// the header dispatch and the import, export, unexport and device list handlers
// behind it. Checked: no panic; a version it does not speak, a status other
// than OK and an unimplemented operation code are refused; no device the
// monitor does not hold is ever bound or unbound, and the binder is never
// touched while export is disabled; the reply stays bounded.
//
// The connection is a buffer pair, not a *net.TCPConn, so export stops at the
// socket handover and nothing is written to the host running the fuzzer.
func FuzzUSBIPDHandleConnection(f *testing.F) {
	// The daemon logs on the default logger as well as its own; discard both.
	slog.SetDefault(slog.New(slog.DiscardHandler))

	// Every request the dispatch has a branch for, at the device the stub holds.
	f.Add(true, request(protocol.OpReqDevList, nil))
	f.Add(true, request(protocol.OpReqImport, busIDBody(fuzzKnownBusID)))
	f.Add(true, request(protocol.OpReqExport, busIDBody(fuzzKnownBusID)))
	f.Add(true, request(protocol.OpReqUnexport, busIDBody(fuzzKnownBusID)))
	f.Add(true, request(protocol.OpReqDevInfo, nil))
	f.Add(true, request(protocol.OpReqCrypkey, nil))
	// The same requests with export off, which must leave the binder untouched.
	f.Add(false, request(protocol.OpReqExport, busIDBody(fuzzKnownBusID)))
	f.Add(false, request(protocol.OpReqUnexport, busIDBody(fuzzKnownBusID)))
	f.Add(false, request(protocol.OpReqImport, busIDBody(fuzzKnownBusID)))
	// Requests for a device the monitor does not hold.
	f.Add(true, request(protocol.OpReqImport, busIDBody("1-1")))
	f.Add(true, request(protocol.OpReqExport, busIDBody("1-1")))
	f.Add(true, request(protocol.OpReqUnexport, busIDBody("2-4.1")))
	f.Add(true, request(protocol.OpReqExport, busIDBody("")))
	// Headers the daemon has to turn away before it reads a body.
	f.Add(true, header(0, protocol.OpReqImport, protocol.OpStatusOk))
	f.Add(true, header(0xffff, protocol.OpReqImport, protocol.OpStatusOk))
	f.Add(true, header(protocol.Version, protocol.OpReqImport, protocol.OpStatusError))
	f.Add(true, header(protocol.Version, protocol.OpReqImport, protocol.OpStatus(0xffffffff)))
	f.Add(true, header(protocol.Version, protocol.Op(0xffff), protocol.OpStatusOk))
	f.Add(true, header(protocol.Version, protocol.Op(0x0000), protocol.OpStatusOk))
	f.Add(true, header(protocol.Version, protocol.Op(0x8000), protocol.OpStatusOk))
	// A stream that ends before the daemon has what it needs.
	f.Add(true, []byte{})
	f.Add(true, []byte{0x01})
	f.Add(true, header(protocol.Version, protocol.OpReqImport, protocol.OpStatusOk)[:7])
	f.Add(true, request(protocol.OpReqImport, []byte("1-1")))
	f.Add(true, request(protocol.OpReqExport, bytes.Repeat([]byte("A"), 31)))
	// Bus IDs built to escape the sysfs directory the daemon pastes them into.
	f.Add(true, request(protocol.OpReqImport, busIDBody("../../../../etc/passwd")))
	f.Add(true, request(protocol.OpReqExport, busIDBody("../../../drivers/usbip-host/bind")))
	f.Add(true, request(protocol.OpReqUnexport, busIDBody("/etc/shadow")))
	f.Add(true, request(protocol.OpReqImport, busIDBody("..")))
	f.Add(true, request(protocol.OpReqImport, busIDBody(".")))
	f.Add(true, request(protocol.OpReqImport, busIDBody("1-1/../..")))
	// Bus IDs that are not text at all.
	f.Add(true, request(protocol.OpReqImport, busIDBody("\x01\x02\x03\x7f")))
	f.Add(true, request(protocol.OpReqImport, busIDBody("\xff\xfe\xfd")))
	f.Add(true, request(protocol.OpReqExport, busIDBody("1-1\n1-2")))
	f.Add(true, request(protocol.OpReqExport, busIDBody("%s%n")))
	f.Add(true, request(protocol.OpReqImport, bytes.Repeat([]byte{0x00}, 32)))
	f.Add(true, request(protocol.OpReqImport, bytes.Repeat([]byte("A"), 32)))
	// More bytes than the request needs: the daemon must stop at the end of the
	// message it understands.
	f.Add(true, append(request(protocol.OpReqDevList, nil), bytes.Repeat([]byte{0xff}, 4<<10)...))
	f.Add(true, append(request(protocol.OpReqImport, busIDBody("1-1")), bytes.Repeat([]byte{0xff}, 4<<10)...))
	f.Add(true, bytes.Repeat(request(protocol.OpReqDevList, nil), 128))
	f.Add(true, bytes.Repeat([]byte{0x00}, 4<<10))
	f.Add(true, bytes.Repeat([]byte{0xff}, 4<<10))

	f.Fuzz(func(t *testing.T, exportEnabled bool, data []byte) {
		if len(data) > fuzzMaxInput {
			t.Skip()
		}

		monitor := &fuzzMonitor{busID: fuzzKnownBusID}
		binder := &fuzzBinder{}

		daemon := NewUSBIPD("", monitor, WithExport(exportEnabled))
		daemon.logger = slog.New(slog.DiscardHandler)
		// The real binder writes to sysfs, and NewUSBIPD builds none while
		// export is disabled. Replace it either way, so a binder call made with
		// export disabled is recorded instead of panicking on a nil pointer.
		daemon.usbBinder = binder

		reply, err := handleRequest(daemon, data)

		requireHeaderRefused(t, data, reply, err)
		requireBoundedReply(t, reply)
		requireNoStrayDeviceOperation(t, exportEnabled, binder)
	})
}

// requireHeaderRefused checks the three header conditions handleConnection
// rejects before it reads a body. The verdict is recomputed from the first
// eight bytes rather than read back out of the daemon. A refused request gets
// no reply.
func requireHeaderRefused(t *testing.T, data, reply []byte, err error) {
	t.Helper()

	refused := func(format string, args ...any) {
		if err == nil {
			t.Fatalf(format+" was accepted", args...)
		}
		if len(reply) != 0 {
			t.Fatalf(format+" was refused but answered with %d bytes: %x", append(args, len(reply), reply)...)
		}
	}

	if len(data) < 8 {
		refused("a %d byte request, shorter than the header,", len(data))
		return
	}

	var (
		version = protocol.USBVersion(binary.BigEndian.Uint16(data[0:2]))
		code    = protocol.Op(binary.BigEndian.Uint16(data[2:4]))
		status  = protocol.OpStatus(binary.BigEndian.Uint32(data[4:8]))
	)

	switch {
	case version != protocol.Version:
		refused("a request declaring USB/IP version %#04x", uint16(version))
	case status != protocol.OpStatusOk:
		refused("a request declaring status %s", status.String())
	case !implementedOps[code]:
		refused("a request with the unimplemented operation code %#04x", uint16(code))
	}
}

// implementedOps are the operation codes handleConnection has a branch for;
// anything else has to come back as an error.
var implementedOps = map[protocol.Op]bool{
	protocol.OpReqDevList:  true,
	protocol.OpReqImport:   true,
	protocol.OpReqExport:   true,
	protocol.OpReqUnexport: true,
	protocol.OpReqDevInfo:  true,
	protocol.OpReqCrypkey:  true,
}

// requireBoundedReply checks that whatever the daemon wrote back is bounded and
// starts with the common header carrying the protocol version.
func requireBoundedReply(t *testing.T, reply []byte) {
	t.Helper()

	if len(reply) == 0 {
		return
	}

	if len(reply) > fuzzMaxReply {
		t.Fatalf("the daemon replied with %d bytes, past the bound of %d", len(reply), fuzzMaxReply)
	}

	if len(reply) < 8 {
		t.Fatalf("the daemon replied with %d bytes, which is shorter than the common header: %x", len(reply), reply)
	}

	if version := protocol.USBVersion(binary.BigEndian.Uint16(reply[0:2])); version != protocol.Version {
		t.Fatalf("the daemon replied with USB/IP version %#04x instead of %#04x", uint16(version), uint16(protocol.Version))
	}
}

// requireNoStrayDeviceOperation checks that nothing reaches the binder while
// export is disabled, and that no bus ID other than the one the monitor holds
// reaches it at all. That second check guards the sysfs path: the helpers in
// sysfs.go paste the bus ID into a format string and normalize nothing, so the
// path's safety rests on every handler answering OpStatusNoDev first, and this
// is the check that fails if that early return is ever dropped.
func requireNoStrayDeviceOperation(t *testing.T, exportEnabled bool, binder *fuzzBinder) {
	t.Helper()

	if !exportEnabled && len(binder.calls) > 0 {
		t.Fatalf("the binder was called with export disabled: %v", binder.calls)
	}

	for _, call := range binder.calls {
		if call.busID != fuzzKnownBusID {
			t.Fatalf("%s was called for bus ID %q, which the monitor does not hold", call.method, call.busID)
		}
	}
}

// handleRequest hands the request to the daemon over an in-memory connection
// and returns what the daemon wrote back. A buffer pair rather than net.Pipe:
// net.Pipe cannot half-close, so a request the daemon reads past deadlocks
// io.ReadFull, while a buffer returns io.EOF like a client that hung up.
func handleRequest(daemon *USBIPD, request []byte) ([]byte, error) {
	conn := &memConn{read: bytes.NewReader(request)}

	err := daemon.handleConnection(conn)

	return conn.written.Bytes(), err
}

// memConn is a net.Conn over a request buffer and a reply buffer. The daemon
// asserts the connection to *net.TCPConn and to syscall.Conn to hand the socket
// to the kernel; neither assertion holds here, so both paths take their failure
// branch and nothing is written to sysfs.
type memConn struct {
	read    *bytes.Reader
	written bytes.Buffer
}

func (c *memConn) Read(p []byte) (int, error)  { return c.read.Read(p) }
func (c *memConn) Write(p []byte) (int, error) { return c.written.Write(p) }
func (c *memConn) Close() error                { return nil }
func (c *memConn) LocalAddr() net.Addr         { return memAddr{} }
func (c *memConn) RemoteAddr() net.Addr        { return memAddr{} }

func (c *memConn) SetDeadline(time.Time) error      { return nil }
func (c *memConn) SetReadDeadline(time.Time) error  { return nil }
func (c *memConn) SetWriteDeadline(time.Time) error { return nil }

type memAddr struct{}

func (memAddr) Network() string { return "mem" }
func (memAddr) String() string  { return "mem" }

// fuzzMonitor stands in for the libusb monitor. It holds exactly one device, so
// every other bus ID a request can name is a device the node does not export.
type fuzzMonitor struct {
	busID string
}

func (m *fuzzMonitor) GetDevices() []libusb.USBDevice {
	return []libusb.USBDevice{{
		Path:               "/sys/bus/usb/devices/" + m.busID,
		BusID:              m.busID,
		DevicePath:         "/sys/devices/" + m.busID,
		Driver:             usbipHostDriverName,
		Bus:                1,
		DeviceNumber:       2,
		VendorID:           0x1d6b,
		ProductID:          0x0002,
		BNumConfigurations: 1,
	}}
}

func (m *fuzzMonitor) GetDevice(string) (*libusb.USBDevice, bool) {
	return nil, false
}

func (m *fuzzMonitor) GetDeviceByBusID(busID string) (*libusb.USBDevice, bool) {
	if busID != m.busID {
		return nil, false
	}

	device := m.GetDevices()[0]

	return &device, true
}

func (m *fuzzMonitor) DeviceChanges() <-chan struct{} {
	return nil
}

// fuzzBinder stands in for the binder, which in production writes to the sysfs
// attributes of the usbip-host driver. It records calls and does nothing.
type fuzzBinder struct {
	calls []binderCall
}

type binderCall struct {
	method string
	busID  string
}

func (b *fuzzBinder) Bind(busID string) error {
	b.calls = append(b.calls, binderCall{method: "Bind", busID: busID})
	return nil
}

func (b *fuzzBinder) Unbind(busID string) error {
	b.calls = append(b.calls, binderCall{method: "Unbind", busID: busID})
	return nil
}

func (b *fuzzBinder) IsBound(busID string) (bool, error) {
	b.calls = append(b.calls, binderCall{method: "IsBound", busID: busID})
	return false, nil
}

func (b *fuzzBinder) GetBindInfo() ([]BindInfo, error) {
	b.calls = append(b.calls, binderCall{method: "GetBindInfo"})
	return nil, nil
}

func header(version protocol.USBVersion, code protocol.Op, status protocol.OpStatus) []byte {
	buf := make([]byte, 8)

	binary.BigEndian.PutUint16(buf[0:2], uint16(version))
	binary.BigEndian.PutUint16(buf[2:4], uint16(code))
	binary.BigEndian.PutUint32(buf[4:8], uint32(status))

	return buf
}

func request(code protocol.Op, body []byte) []byte {
	return append(header(protocol.Version, code, protocol.OpStatusOk), body...)
}

func busIDBody(busID string) []byte {
	var body bytes.Buffer

	if err := protocol.NewImportRequest(busID).Encode(&body); err != nil {
		panic(err)
	}

	return body.Bytes()
}
