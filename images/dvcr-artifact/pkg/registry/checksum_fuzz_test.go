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

package registry

import (
	"bytes"
	"encoding/hex"
	"io"
	"strings"
	"testing"
)

// Sums of the "hello" payload, the same ones the unit tests pin.
const (
	helloMD5         = "5d41402abc4b2a76b9719d911017c592"
	helloSHA1        = "aaf4c61ddcc5e8a2dabede0f3b482cd9aea9434d"
	helloSHA256      = "2cf24dba5fb0a30e26e83b2ac5b9e29e1b161e5c1fa7425e73043362938b9824"
	helloSHA512      = "9b71d224bd62f3785d96d46ad3ea3d73319bfbc2890caadae2dff72519673ca72323c3d99ba5c11d7c7acc6e14b8c5da0c4663475c2e5c3adef46f73bcdec043"
	helloStreebog256 = "3fb0700a41ce6e41413ba764f98bf2135ba6ded516bea2fae8429cc5bdd46d6d"
	helloStreebog512 = "8df414260966beb7b34d920763079e15df1f63297eb3dd4311e8b585d4bf2f5923214f1dfed3fdee4aaf018330a12acde0efcc338eb52922f3e571212d42c8de"
)

// FuzzChecksums drives the whole optional checksum path of an import: the
// algorithm:sum pairs come straight from the checksum section of a
// VirtualImage spec, so both the spelling of the spec and the payload they are
// verified against are user controlled. A malformed spec and a mismatching sum
// are expected outcomes; only a panic or a contradiction between the parsing,
// the processor construction and the verdict is a finding.
func FuzzChecksums(f *testing.F) {
	hello := []byte("hello")

	f.Add("", hello)
	f.Add("md5:"+helloMD5, hello)
	// Hex digits are case insensitive, the parsing normalizes them.
	f.Add("md5:"+strings.ToUpper(helloMD5), hello)
	f.Add("sha1:"+helloSHA1, hello)
	f.Add("sha256:"+helloSHA256, hello)
	f.Add("sha512:"+helloSHA512, hello)
	// GOST R 34.11-2012, both digest lengths.
	f.Add("streebog256:"+helloStreebog256, hello)
	f.Add("streebog512:"+helloStreebog512, hello)
	f.Add("md5:"+helloMD5+",sha256:"+helloSHA256+",streebog512:"+helloStreebog512, hello)
	// A correct sum of the wrong payload, and the sum of an empty payload.
	f.Add("sha256:"+helloSHA256, []byte("hellp"))
	f.Add("sha256:"+helloSHA256, []byte{})
	f.Add("md5:d41d8cd98f00b204e9800998ecf8427e", []byte{})
	f.Add("sha256:"+strings.Repeat("0", 64), bytes.Repeat([]byte{0x00}, 4096))
	// Malformed pairs.
	f.Add("sha256", hello)
	f.Add("sha256:", hello)
	f.Add(":"+helloSHA256, hello)
	f.Add(",,,", hello)
	f.Add("md5:"+helloMD5+",", hello)
	f.Add(","+"md5:"+helloMD5, hello)
	f.Add(" md5 : "+helloMD5+" ", hello)
	f.Add("md5="+helloMD5, hello)
	// Algorithms the importer cannot calculate, and a name whose case does not
	// match the spec field.
	f.Add("gost341194:abc", hello)
	f.Add("MD5:"+helloMD5, hello)
	f.Add("crc32:00000000", hello)
	// Digests that are not hex, are the wrong length, or carry control bytes.
	f.Add("sha256:"+helloSHA256[:63], hello)
	f.Add("sha256:0x"+helloSHA256, hello)
	f.Add("md5:-1", hello)
	f.Add("streebog256:\xff\xfe\xfd", hello)
	f.Add("sha256:"+helloSHA256+"\n", hello)
	f.Add("md5:5d41402abc\x00b2a76b9719d911017c592", hello)
	// Oversized input: a 64 KiB digest, an oversized algorithm name and a spec
	// repeating the same algorithm a thousand times.
	f.Add("md5:"+strings.Repeat("f", 64<<10), hello)
	f.Add("streebog512:"+strings.Repeat("a", 4096), hello)
	f.Add(strings.Repeat("a", 4096)+":"+strings.Repeat("b", 4096), hello)
	f.Add(strings.Repeat("md5:0,", 1000)+"md5:"+helloMD5, hello)
	f.Add("md5:a,md5:"+helloMD5, hello)

	f.Fuzz(func(t *testing.T, spec string, data []byte) {
		if len(spec) > fuzzMaxInputSize || len(data) > fuzzMaxInputSize {
			t.Skip()
		}

		checksums, err := ParseChecksums(spec)
		if err != nil {
			if checksums != nil {
				t.Fatalf("ParseChecksums returned %d checksums along with the error %v", len(checksums), err)
			}

			return
		}

		// The importer parses the spec first and builds the processor second,
		// so whatever the parsing accepts the constructor has to accept too:
		// otherwise an import fails after the spec was already validated. The
		// constructor does not touch the data source.
		if _, err := NewDataProcessor(nil, DestinationRegistry{ImageName: fuzzDestImageName}, checksums); err != nil {
			t.Fatalf("NewDataProcessor rejected the checksums ParseChecksums accepted: %v", err)
		}

		writers, checks := newChecksumVerifiers(checksums)
		if len(writers) != len(checksums) || len(checks) != len(checksums) {
			t.Fatalf("got %d writers and %d checks for %d checksums", len(writers), len(checks), len(checksums))
		}

		if len(writers) > 0 {
			if _, err := io.MultiWriter(writers...).Write(data); err != nil {
				t.Fatalf("hashing %d bytes failed: %v", len(data), err)
			}
		}

		// A mismatch is the normal verdict for a fuzzed sum, so only the fact
		// that every check reaches a verdict at all matters here.
		for _, check := range checks {
			_ = check()
		}

		requireEveryAlgorithmAcceptsItsOwnSum(t, data)
	})
}

// requireEveryAlgorithmAcceptsItsOwnSum walks the same path once more with the
// sums of the payload itself: the pairs are rendered the way an importer Pod
// receives them, parsed back and verified against the very bytes they were
// computed from. Every supported algorithm has to agree, which pins the
// verification of arbitrary payloads for md5, sha1, sha256, sha512 and both
// GOST R 34.11-2012 digests.
func requireEveryAlgorithmAcceptsItsOwnSum(t *testing.T, data []byte) {
	t.Helper()

	pairs := make([]string, 0, len(checksumAlgorithms))
	for _, algorithm := range sortedChecksumAlgorithms(checksumAlgorithms) {
		hash := checksumAlgorithms[algorithm]()
		if _, err := hash.Write(data); err != nil {
			t.Fatalf("%s failed on %d bytes: %v", algorithm, len(data), err)
		}

		// Upper case on purpose: the parsing has to normalize it.
		pairs = append(pairs, algorithm+":"+strings.ToUpper(hex.EncodeToString(hash.Sum(nil))))
	}

	checksums, err := ParseChecksums(strings.Join(pairs, ","))
	if err != nil {
		t.Fatalf("the spec of the recomputed sums was rejected: %v", err)
	}

	writers, checks := newChecksumVerifiers(checksums)
	if _, err := io.MultiWriter(writers...).Write(data); err != nil {
		t.Fatalf("hashing %d bytes failed: %v", len(data), err)
	}

	for _, check := range checks {
		if err := check(); err != nil {
			t.Fatalf("a correct sum of %d bytes was rejected: %v", len(data), err)
		}
	}
}
