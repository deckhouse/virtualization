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

This file was initially copied from the Containerized Data Importer (CDI)
project (https://github.com/kubevirt/containerized-data-importer),
Copyright 2018 The CDI Authors, and adapted for the virtualization module.
*/

package image

import (
	"bytes"
	"encoding/hex"
	"strconv"

	"github.com/pkg/errors"
	"k8s.io/klog/v2"
)

// MaxExpectedHdrSize defines the Size of buffer used to read file headers.
// Note: this is the size of tar's header. If a larger number is used the tar unarchive operation
//
//	creates the destination file too large, by the difference between this const and 512.
const MaxExpectedHdrSize = 512

// Headers provides a map for header info, key is file format, eg. "gz" or "tar", value is metadata describing the layout for this hdr
type Headers map[string]Header

var knownHeaders = Headers{
	"gz": Header{
		Format:      "gz",
		magicNumber: []byte{0x1F, 0x8B},
		// TODO: size not in hdr
		SizeOff: 0,
		SizeLen: 0,
	},
	"zst": Header{
		Format:      "zst",
		magicNumber: []byte{0x28, 0xb5, 0x2f, 0xfd},
		SizeOff:     0,
		SizeLen:     0,
	},
	"qcow2": Header{
		Format:      "qcow2",
		magicNumber: []byte{'Q', 'F', 'I', 0xfb},
		mgOffset:    0,
		SizeOff:     24,
		SizeLen:     8,
	},
	"tar": Header{
		Format:      "tar",
		magicNumber: []byte{0x75, 0x73, 0x74, 0x61, 0x72},
		mgOffset:    0x101,
		SizeOff:     124,
		SizeLen:     8,
	},
	"xz": Header{
		Format:      "xz",
		magicNumber: []byte{0xFD, 0x37, 0x7A, 0x58, 0x5A, 0x00},
		// TODO: size not in hdr
		SizeOff: 0,
		SizeLen: 0,
	},
	"vmdk": Header{
		Format:      "vmdk",
		magicNumber: []byte("KDMV"),
		SizeOff:     0,
		SizeLen:     0,
	},
	"vdi": Header{
		Format:      "vdi",
		magicNumber: []byte{0x7F, 0x10, 0xDA, 0xBE},
		mgOffset:    0x40,
		SizeOff:     0,
		SizeLen:     0,
	},
	"vhd": Header{
		Format:      "vhd",
		magicNumber: []byte("conectix"),
		SizeOff:     0,
		SizeLen:     0,
	},
	"vhdx": Header{
		Format:      "vhdx",
		magicNumber: []byte("vhdxfile"),
		SizeOff:     0,
		SizeLen:     0,
	},
}

// Header represents our parameters for a file format header
type Header struct {
	Format      string
	magicNumber []byte
	mgOffset    int
	SizeOff     int // in bytes
	SizeLen     int // in bytes
}

// CopyKnownHdrs performs a simple map copy since := assignment copies the reference to the map, not contents.
func CopyKnownHdrs() Headers {
	m := make(Headers)
	for k, v := range knownHeaders {
		m[k] = v
	}
	return m
}

// Match performs a check to see if the provided byte slice matches the bytes in our header data
func (h Header) Match(b []byte) bool {
	return bytes.Equal(b[h.mgOffset:h.mgOffset+len(h.magicNumber)], h.magicNumber)
}

// Size uses the Header receiver offset and length fields to extract, from the passed-in file header slice (b),
// the size of the original file. It is not guaranteed that the header is known to cdi and thus 0 may be returned as the size.
func (h Header) Size(b []byte) (int64, error) {
	if h.SizeLen == 0 { // no size is supported in this format's header
		return 0, nil
	}
	s := hex.EncodeToString(b[h.SizeOff : h.SizeOff+h.SizeLen])
	size, err := strconv.ParseInt(s, 16, 64)
	if err != nil {
		return 0, errors.Wrapf(err, "unable to determine original file size from %+v", s)
	}
	klog.V(3).Infof("Size: %q size in bytes (at off %d:%d): %d", h.Format, h.SizeOff, h.SizeOff+h.SizeLen, size)
	return size, nil
}
