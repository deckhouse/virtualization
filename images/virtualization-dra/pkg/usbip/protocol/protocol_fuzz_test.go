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
	"bytes"
	"encoding/binary"
	"go/ast"
	"go/parser"
	"go/token"
	"io"
	"path/filepath"
	"strings"
	"testing"
)

const (
	// Past the longest message: a 312 byte device plus 4 bytes per interface.
	fuzzMaxInput = 8 << 10

	// BNumInterfaces is a uint8 and drives the only allocation in the protocol
	// sized from a value off the wire.
	fuzzMaxInterfaces = 255

	fuzzDevicePath = "/sys/devices/pci0000:00/usb1/1-1"
	fuzzBusID      = "1-1"
)

type decoder interface {
	Decode(r io.Reader) error
}

type encoder interface {
	Encode(w io.Writer) error
}

// decoders are the ten Decode methods of this package, each behind a
// constructor so that every iteration starts from a zero value.
var decoders = []struct {
	name string
	new  func() decoder
}{
	{"OpCommon", func() decoder { return &OpCommon{} }},
	{"USBDevice", func() decoder { return &USBDevice{} }},
	{"USBDeviceInterface", func() decoder { return &USBDeviceInterface{} }},
	{"USBDeviceInfo", func() decoder { return &USBDeviceInfo{} }},
	{"ImportRequest", func() decoder { return &ImportRequest{} }},
	{"ImportReply", func() decoder { return &ImportReply{} }},
	{"ExportRequest", func() decoder { return &ExportRequest{} }},
	{"ExportReply", func() decoder { return &ExportReply{} }},
	{"UnExportRequest", func() decoder { return &UnExportRequest{} }},
	{"UnExportReply", func() decoder { return &UnExportReply{} }},
}

// FuzzProtocolDecoders drives every Decode method in this package over raw
// bytes, the selector picking the decoder. Findings: a panic; an allocation
// sized by a length off the wire; a decoder reporting success with a value
// inconsistent with the bytes; a bus ID or path coming back as anything but a
// bounded NUL-free string.
func FuzzProtocolDecoders(f *testing.F) {
	// Well-formed headers the server dispatches on, and replies a client reads.
	f.Add(uint8(0), opCommonBytes(Version, OpReqDevList, OpStatusOk))
	f.Add(uint8(0), opCommonBytes(Version, OpReqImport, OpStatusOk))
	f.Add(uint8(0), opCommonBytes(Version, OpReqExport, OpStatusOk))
	f.Add(uint8(0), opCommonBytes(Version, OpReqUnexport, OpStatusOk))
	f.Add(uint8(0), opCommonBytes(Version, OpReqDevInfo, OpStatusOk))
	f.Add(uint8(0), opCommonBytes(Version, OpReqCrypkey, OpStatusOk))
	f.Add(uint8(0), opCommonBytes(Version, OpRepImport, OpStatusOk))
	// Headers the server has to turn away.
	f.Add(uint8(0), opCommonBytes(0, OpReqImport, OpStatusOk))
	f.Add(uint8(0), opCommonBytes(0xffff, OpReqImport, OpStatusOk))
	f.Add(uint8(0), opCommonBytes(Version, OpReqImport, OpStatusError))
	f.Add(uint8(0), opCommonBytes(Version, OpReqImport, OpStatus(0xffffffff)))
	f.Add(uint8(0), opCommonBytes(Version, Op(0xffff), OpStatusOk))
	f.Add(uint8(0), opCommonBytes(Version, Op(0x0000), OpStatusOk))
	// A header cut short of eight bytes.
	f.Add(uint8(0), []byte{})
	f.Add(uint8(0), []byte{0x01})
	f.Add(uint8(0), opCommonBytes(Version, OpReqImport, OpStatusOk)[:7])
	// Bus IDs, the whole body of an import, export and unexport request.
	f.Add(uint8(4), busIDBytes("1-1"))
	f.Add(uint8(4), busIDBytes("1-1.4.2"))
	f.Add(uint8(4), bytes.Repeat([]byte("A"), sysfsBusIdMax))
	f.Add(uint8(4), append([]byte{0x00}, bytes.Repeat([]byte("A"), sysfsBusIdMax-1)...))
	f.Add(uint8(4), append([]byte("1-1\x00"), bytes.Repeat([]byte("B"), sysfsBusIdMax-4)...))
	f.Add(uint8(4), busIDBytes(""))
	// Bus IDs that would escape the sysfs directory the server pastes them into.
	f.Add(uint8(4), busIDBytes("../../../../etc/passwd"))
	f.Add(uint8(4), busIDBytes("..%2F..%2Fetc"))
	f.Add(uint8(4), busIDBytes("1-1/../../.."))
	f.Add(uint8(4), busIDBytes("/etc/shadow"))
	f.Add(uint8(4), busIDBytes("."))
	f.Add(uint8(4), busIDBytes(".."))
	// Bus IDs that are not text.
	f.Add(uint8(4), busIDBytes("1-1\n1-2"))
	f.Add(uint8(4), busIDBytes("\x01\x02\x03\x7f"))
	f.Add(uint8(4), busIDBytes("\xff\xfe\xfd"))
	f.Add(uint8(4), busIDBytes("%s%n%d"))
	// A request body cut inside the bus ID field.
	f.Add(uint8(4), []byte("1-1"))
	f.Add(uint8(4), bytes.Repeat([]byte("A"), sysfsBusIdMax-1))
	// A whole device: the fixed-size part of a list entry and of an import reply.
	f.Add(uint8(1), usbDeviceBytes("/sys/devices/pci0000:00/usb1/1-1", "1-1", 0))
	f.Add(uint8(1), usbDeviceBytes("", "", 0))
	f.Add(uint8(1), usbDeviceBytes(strings.Repeat("p", sysfsPathMax), strings.Repeat("b", sysfsBusIdMax), 0))
	f.Add(uint8(1), usbDeviceBytes("../../../../etc", "..", 0))
	f.Add(uint8(1), usbDeviceBytes("/sys/devices/\xff\xfe", "\x01\x02", 0))
	f.Add(uint8(1), usbDeviceBytes(fuzzDevicePath, fuzzBusID, 0)[:311])
	// Device list entries: walk the interface count, the one field that sizes an
	// allocation, with and without the interfaces it promises.
	f.Add(uint8(3), deviceInfoBytes(0))
	f.Add(uint8(3), deviceInfoBytes(1))
	f.Add(uint8(3), deviceInfoBytes(32))
	f.Add(uint8(3), deviceInfoBytes(255))
	f.Add(uint8(3), usbDeviceBytes(fuzzDevicePath, fuzzBusID, 255))
	f.Add(uint8(3), usbDeviceBytes(fuzzDevicePath, fuzzBusID, 1))
	f.Add(uint8(3), deviceInfoBytes(255)[:400])
	// A single interface descriptor and a truncated one.
	f.Add(uint8(2), []byte{0x03, 0x01, 0x02, 0x00})
	f.Add(uint8(2), []byte{0xff, 0xff, 0xff, 0xff})
	f.Add(uint8(2), []byte{0x03, 0x01, 0x02})
	// An import reply: a header followed by a device.
	f.Add(uint8(5), append(opCommonBytes(Version, OpRepImport, OpStatusOk), usbDeviceBytes(fuzzDevicePath, fuzzBusID, 0)...))
	f.Add(uint8(5), append(opCommonBytes(Version, OpRepImport, OpStatusNoDev), usbDeviceBytes("", "", 0)...))
	f.Add(uint8(5), opCommonBytes(Version, OpRepImport, OpStatusOk))
	// The remaining decoders, so the seed corpus alone covers all ten.
	f.Add(uint8(6), busIDBytes("1-1"))
	f.Add(uint8(7), opCommonBytes(Version, OpRepExport, OpStatusOk))
	f.Add(uint8(8), busIDBytes("1-1"))
	f.Add(uint8(9), opCommonBytes(Version, OpRepExport, OpStatusOk))
	// Long streams: a decoder must consume what its layout says and no more.
	f.Add(uint8(0), bytes.Repeat(opCommonBytes(Version, OpReqDevList, OpStatusOk), 64))
	f.Add(uint8(3), bytes.Repeat([]byte{0xff}, 4<<10))
	f.Add(uint8(4), bytes.Repeat([]byte{0x00}, 4<<10))

	f.Fuzz(func(t *testing.T, selector uint8, data []byte) {
		if len(data) > fuzzMaxInput {
			t.Skip()
		}

		target := decoders[int(selector)%len(decoders)]

		// Failing is the normal outcome; only a success has to be consistent.
		message := target.new()
		if err := message.Decode(bytes.NewReader(data)); err != nil {
			return
		}

		requireBoundedInterfaces(t, target.name, message)
		requireBoundedStrings(t, target.name, message)
		requireRoundTrip(t, target.name, target.new, message, data)
	})
}

// requireBoundedStrings checks that the bus ID and the device path come back
// bounded and NUL-free: the server pastes the bus ID into a sysfs path.
func requireBoundedStrings(t *testing.T, name string, message decoder) {
	t.Helper()

	type busIDGetter interface{ BusID() string }
	type pathGetter interface{ GetPath() string }
	type busIDFieldGetter interface{ GetBusID() string }

	check := func(what, value string, limit int) {
		if len(value) > limit {
			t.Fatalf("%s.%s returned %d bytes from a %d byte field: %q", name, what, len(value), limit, value)
		}
		// Pins that fromCString keeps cutting at the first NUL.
		if strings.ContainsRune(value, 0) {
			t.Fatalf("%s.%s returned a string with a NUL in it: %q", name, what, value)
		}
	}

	if m, ok := message.(busIDGetter); ok {
		check("BusID", m.BusID(), sysfsBusIdMax)
	}
	if m, ok := message.(busIDFieldGetter); ok {
		check("GetBusID", m.GetBusID(), sysfsBusIdMax)
	}
	if m, ok := message.(pathGetter); ok {
		check("GetPath", m.GetPath(), sysfsPathMax)
	}
}

// requireRoundTrip re-encodes an accepted message and decodes it again.
// Comparing the re-encoded bytes with the prefix they came from pins that the
// decoder reads fields where its encoder writes them.
func requireRoundTrip(t *testing.T, name string, newMessage func() decoder, message decoder, data []byte) {
	t.Helper()

	source, ok := message.(encoder)
	if !ok {
		t.Fatalf("%s decodes but does not encode, so nothing pins the offsets it reads", name)
	}

	var encoded bytes.Buffer
	if err := source.Encode(&encoded); err != nil {
		t.Fatalf("%s.Encode failed on a message its own Decode accepted: %v", name, err)
	}

	if encoded.Len() > len(data) {
		t.Fatalf("%s re-encoded to %d bytes from a %d byte message", name, encoded.Len(), len(data))
	}

	if !bytes.Equal(encoded.Bytes(), data[:encoded.Len()]) {
		t.Fatalf("%s re-encoded to bytes other than the ones it read:\n got: %x\nwant: %x", name, encoded.Bytes(), data[:encoded.Len()])
	}

	again := newMessage()
	if err := again.Decode(bytes.NewReader(encoded.Bytes())); err != nil {
		t.Fatalf("%s.Decode rejected the output of its own Encode: %v", name, err)
	}

	var reEncoded bytes.Buffer
	if err := again.(encoder).Encode(&reEncoded); err != nil {
		t.Fatalf("%s.Encode failed on the second pass: %v", name, err)
	}

	if !bytes.Equal(encoded.Bytes(), reEncoded.Bytes()) {
		t.Fatalf("%s is not stable across a decode/encode round trip:\nfirst:  %x\nsecond: %x", name, encoded.Bytes(), reEncoded.Bytes())
	}
}

// requireBoundedInterfaces pins that the one allocation sized from the message,
// the USBDeviceInfo interface slice, stays capped by the uint8 BNumInterfaces
// and matches the declared count.
func requireBoundedInterfaces(t *testing.T, name string, message decoder) {
	t.Helper()

	info, ok := message.(*USBDeviceInfo)
	if !ok {
		return
	}

	if len(info.Interfaces) > fuzzMaxInterfaces {
		t.Fatalf("%s decoded %d interfaces, past the ceiling of %d", name, len(info.Interfaces), fuzzMaxInterfaces)
	}

	if len(info.Interfaces) != int(info.BNumInterfaces) {
		t.Fatalf("%s decoded %d interfaces for a declared count of %d", name, len(info.Interfaces), info.BNumInterfaces)
	}
}

func opCommonBytes(version USBVersion, code Op, status OpStatus) []byte {
	buf := make([]byte, 8)

	binary.BigEndian.PutUint16(buf[0:2], uint16(version))
	binary.BigEndian.PutUint16(buf[2:4], uint16(code))
	binary.BigEndian.PutUint32(buf[4:8], uint32(status))

	return buf
}

func busIDBytes(busID string) []byte {
	field := ToBusID(busID)
	return field[:]
}

// usbDeviceBytes encodes the fixed-size device descriptor with the interface
// count set independently of any interface that follows.
func usbDeviceBytes(path, busID string, interfaces uint8) []byte {
	device := USBDevice{
		Path:                ToDevicePath(path),
		BusID:               ToBusID(busID),
		Busnum:              1,
		Devnum:              2,
		Speed:               3,
		IDVendor:            0x1d6b,
		IDProduct:           0x0002,
		BcdDevice:           0x0200,
		BDeviceClass:        0x09,
		BDeviceSubClass:     0x00,
		BDeviceProtocol:     0x01,
		BConfigurationValue: 1,
		BNumConfigurations:  1,
		BNumInterfaces:      interfaces,
	}

	var buf bytes.Buffer
	if err := device.Encode(&buf); err != nil {
		panic(err)
	}

	return buf.Bytes()
}

// deviceInfoBytes is usbDeviceBytes followed by the interface descriptors the
// count promises, which is a well-formed device list entry.
func deviceInfoBytes(interfaces uint8) []byte {
	buf := usbDeviceBytes(fuzzDevicePath, fuzzBusID, interfaces)

	for i := 0; i < int(interfaces); i++ {
		buf = append(buf, byte(i), byte(i+1), byte(i+2), 0x00)
	}

	return buf
}

// TestDecodersListIsComplete reads the Decode methods out of the package
// sources, so an eleventh decoder fails this test by existing until it is added
// to the decoders list.
func TestDecodersListIsComplete(t *testing.T) {
	covered := make(map[string]bool, len(decoders))
	for _, target := range decoders {
		covered[target.name] = true
	}

	inPackage := decodeMethodReceivers(t)

	for name := range inPackage {
		if !covered[name] {
			t.Errorf("%s has a Decode method that no fuzz target drives; add it to the decoders list", name)
		}
	}

	for name := range covered {
		if !inPackage[name] {
			t.Errorf("the decoders list names %s, which has no Decode method in this package", name)
		}
	}
}

// decodeMethodReceivers returns the receiver of every Decode method in the
// package sources.
func decodeMethodReceivers(t *testing.T) map[string]bool {
	t.Helper()

	sources, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatalf("failed to list the package sources: %v", err)
	}

	receivers := map[string]bool{}

	for _, source := range sources {
		if strings.HasSuffix(source, "_test.go") {
			continue
		}

		parsed, err := parser.ParseFile(token.NewFileSet(), source, nil, parser.SkipObjectResolution)
		if err != nil {
			t.Fatalf("failed to parse %s: %v", source, err)
		}

		for _, decl := range parsed.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || fn.Name.Name != "Decode" || fn.Recv == nil || len(fn.Recv.List) != 1 {
				continue
			}

			receiver := fn.Recv.List[0].Type
			if star, ok := receiver.(*ast.StarExpr); ok {
				receiver = star.X
			}

			if ident, ok := receiver.(*ast.Ident); ok {
				receivers[ident.Name] = true
			}
		}
	}

	if len(receivers) == 0 {
		t.Fatal("no Decode method was found in the package sources, so this test checks nothing")
	}

	return receivers
}
