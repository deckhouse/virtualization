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

// In-package test for the conntrack package of the KubeVirt fork: it lives here
// and the virt-artifact-fuzz image places it into the cloned fork tree. Keep it
// in package conntrack and import nothing the fork does not vendor.
package conntrack

import (
	"bytes"
	"encoding/binary"
	"runtime/metrics"
	"testing"
)

const (
	// Version byte plus a four byte big-endian length.
	headerLen = 5

	fuzzMaxInput = 1 << 20

	// Ceiling on the buffer coming off the wire: proportional to the bytes that
	// arrived, plus the fixed read chunk the decoder allocates before the first
	// payload byte. Loose because the Go allocation counter is process-wide.
	fuzzAllocFactor   = 64
	fuzzAllocOverhead = 8 << 20
)

// FuzzDecodeSyncMessage drives the conntrack synchronization message decoder.
// The message arrives over the migration connection from the source
// virt-handler, and its declared length is a uint32 the sender picks freely.
//
// Checked: no panic; allocation proportional to the bytes that arrived rather
// than to the length the header claims; a message that ends early fails; an
// accepted message re-encodes to the bytes it read. The 512 MiB limit on the
// declared length is the fork's, and the seeds carry lengths on both sides of
// it, up to the whole uint32 range.
func FuzzDecodeSyncMessage(f *testing.F) {
	// Well-formed messages, from an empty payload upwards.
	f.Add(syncMessage(1, nil))
	f.Add(syncMessage(1, []byte("x")))
	f.Add(syncMessage(1, bytes.Repeat([]byte("a"), 64)))
	f.Add(syncMessage(1, bytes.Repeat([]byte{0x00}, 4096)))
	f.Add(syncMessage(0, []byte("version zero")))
	f.Add(syncMessage(0xff, []byte("version max")))
	// Truncated at every boundary of the header.
	f.Add([]byte{})
	f.Add([]byte{0x01})
	f.Add([]byte{0x01, 0x00})
	f.Add([]byte{0x01, 0x00, 0x00})
	f.Add([]byte{0x01, 0x00, 0x00, 0x00})
	// A header promising payload the message does not carry.
	f.Add([]byte{0x01, 0x00, 0x00, 0x00, 0x01})
	f.Add([]byte{0x01, 0x00, 0x00, 0x00, 0x10, 0x61, 0x62})
	f.Add(syncMessage(1, bytes.Repeat([]byte("a"), 64))[:headerLen+32])
	// Extra bytes past the declared length, which must stay on the reader.
	f.Add(append(syncMessage(1, []byte("ab")), bytes.Repeat([]byte("c"), 64)...))
	f.Add(append(syncMessage(1, nil), bytes.Repeat([]byte{0xff}, 4096)...))
	// Declared lengths costing far more than the message weighs: under the
	// 512 MiB limit, at it, one byte past it, and the uint32 maximum.
	f.Add([]byte{0x01, 0x00, 0x10, 0x00, 0x00})
	f.Add([]byte{0x01, 0x00, 0x40, 0x00, 0x00})
	f.Add([]byte{0x01, 0x20, 0x00, 0x00, 0x00})
	f.Add([]byte{0x01, 0x20, 0x00, 0x00, 0x01})
	f.Add([]byte{0x01, 0xff, 0xff, 0xff, 0xff})
	// Two messages back to back, the way a stream carries them.
	f.Add(append(syncMessage(1, []byte("first")), syncMessage(1, []byte("second"))...))

	f.Fuzz(func(t *testing.T, data []byte) {
		if len(data) > fuzzMaxInput {
			t.Skip()
		}

		before := allocatedBytes()
		message, err := DecodeSyncMessage(bytes.NewReader(data))
		allocated := allocatedBytes() - before

		if limit := uint64(len(data))*fuzzAllocFactor + fuzzAllocOverhead; allocated > limit {
			declared := uint32(0)
			if len(data) >= headerLen {
				declared = binary.BigEndian.Uint32(data[1:headerLen])
			}

			t.Fatalf(
				"decoding a %d byte message allocated %d bytes, past the ceiling of %d: the message declared a payload of %d bytes and the decoder sized its buffer from that number before reading any payload",
				len(data), allocated, limit, declared,
			)
		}

		if err != nil {
			if message != nil {
				t.Fatalf("DecodeSyncMessage returned a message together with the error %v", err)
			}

			return
		}

		if message == nil {
			t.Fatal("DecodeSyncMessage reported success and returned no message")
		}

		requireWholeMessage(t, data, message)
	})
}

// requireWholeMessage checks that an accepted message is the one the bytes
// carried, so a truncated update cannot pass for a complete one.
func requireWholeMessage(t *testing.T, data []byte, message *SyncMessage) {
	t.Helper()

	if len(data) < headerLen {
		t.Fatalf("a %d byte message was accepted, which is shorter than the header", len(data))
	}

	var (
		version  = data[0]
		declared = binary.BigEndian.Uint32(data[1:headerLen])
	)

	if message.Version != version {
		t.Fatalf("decoded version %#02x from a message declaring %#02x", message.Version, version)
	}

	if uint32(len(message.Data)) != declared {
		t.Fatalf("decoded %d payload bytes from a message declaring %d", len(message.Data), declared)
	}

	available := uint64(len(data) - headerLen)
	if uint64(declared) > available {
		t.Fatalf("a message declaring %d payload bytes was accepted while carrying only %d", declared, available)
	}

	if !bytes.Equal(message.Data, data[headerLen:headerLen+int(declared)]) {
		t.Fatal("the decoded payload is not the bytes the message carried")
	}

	// The source side builds what this decoder reads with Encode, so an
	// accepted message has to be one the sender could have produced.
	encoded := message.Encode()
	if want := data[:headerLen+int(declared)]; !bytes.Equal(encoded, want) {
		t.Fatalf("re-encoding the accepted message gave %x, the bytes it read were %x", encoded, want)
	}
}

// allocatedBytes reports total bytes allocated by the process, so the
// difference across a call is that call's allocation cost.
var allocsSample = []metrics.Sample{{Name: "/gc/heap/allocs:bytes"}}

func allocatedBytes() uint64 {
	metrics.Read(allocsSample)
	return allocsSample[0].Value.Uint64()
}

// syncMessage builds a well-formed message the way the source side does.
func syncMessage(version byte, data []byte) []byte {
	return (&SyncMessage{Version: version, Data: data}).Encode()
}
