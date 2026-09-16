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

package cloudinit

import (
	"bytes"
	"compress/gzip"
	"runtime/metrics"
	"strconv"
	"strings"
	"testing"
	"unicode/utf8"
)

const (
	// Twice the deepest seed.
	fuzzMaxInput = 128 << 10

	// Far above any warning the validation builds.
	fuzzMaxWarningLen = 4 << 10

	// Catches amplification by orders of magnitude, not a tight budget.
	fuzzAllocFactor   = 1024
	fuzzAllocOverhead = 8 << 20
)

// FuzzValidateUserData drives the cloud-init user data validation over raw
// bytes written by a namespace user and never parsed by kube-apiserver.
//
// Checked: no panic; allocation proportional to the payload; every warning
// non-empty, valid UTF-8 and within its bound, since it goes into a condition
// message and an event. A warning is the normal outcome, never a finding.
func FuzzValidateUserData(f *testing.F) {
	// Payloads cloud-init understands, one per recognized header.
	f.Add([]byte("#cloud-config\nusers:\n  - name: cloud\n"))
	f.Add([]byte("#cloud-config"))
	f.Add([]byte("#cloud-config\n---\npackages:\n  - nginx\n"))
	f.Add([]byte("#cloud-config\r\nhostname: vm\r\n"))
	f.Add([]byte("\xef\xbb\xbf#cloud-config\nhostname: vm\n"))
	f.Add([]byte("#cloud-config-archive\n- type: text/cloud-boothook\n  content: |\n    #!/bin/sh\n"))
	f.Add([]byte("#cloud-config-jsonp\n[{\"op\": \"add\", \"path\": \"/hostname\", \"value\": \"vm\"}]\n"))
	f.Add([]byte("## template: jinja\n#cloud-config\nhostname: {{ ds.meta_data.hostname }}\n"))
	f.Add([]byte("#!/bin/bash\necho hello\n"))
	f.Add([]byte("#include\nhttps://example.com/config\n"))
	f.Add([]byte("#include-once\nhttps://example.com/config\n"))
	f.Add([]byte("#cloud-boothook\n#!/bin/sh\necho early\n"))
	f.Add([]byte("#part-handler\ndef list_types():\n    pass\n"))
	f.Add([]byte("#upstart-job\nstart on stopped rc\nexec /bin/true\n"))
	f.Add([]byte("MIME-Version: 1.0\nContent-Type: multipart/mixed; boundary=\"==B==\"\n"))
	f.Add([]byte("Content-Type: multipart/mixed; boundary=\"==B==\"\nMIME-Version: 1.0\n"))
	// Header spelling, case, and the longest-header-wins order.
	f.Add([]byte("#CLOUD-CONFIG\nhostname: vm\n"))
	f.Add([]byte("#Cloud-Config-Archive\n- \"\"\n"))
	f.Add([]byte("mime-version: 1.0\n"))
	f.Add([]byte("#cloud-configuration\nhostname: vm\n"))
	// Near-miss template headers.
	f.Add([]byte("##template: jinja\n#cloud-config\n"))
	f.Add([]byte("## Template:Jinja\n"))
	f.Add([]byte("##  template  :  jinja2  \n"))
	// Empty, whitespace only, and only the bytes trimmed from the front.
	f.Add([]byte{})
	f.Add([]byte(" \t\r\n"))
	f.Add([]byte("\xef\xbb\xbf"))
	f.Add(bytes.Repeat([]byte("\xef\xbb\xbf \t\r\n"), 64))
	// Compressed payloads.
	f.Add(gzipped([]byte("#cloud-config\nhostname: vm\n")))
	f.Add([]byte{0x1f, 0x8b})
	f.Add([]byte{0x1f, 0x8b, 0x08, 0xff, 0xff, 0xff})
	f.Add([]byte{0x1f})
	// A cloud-config that is not a mapping, not YAML at all, or a document the
	// conversion rejects.
	f.Add([]byte("#cloud-config\n- a\n- b\n"))
	f.Add([]byte("#cloud-config\njust a string\n"))
	f.Add([]byte("#cloud-config\n\tindented with a tab\n"))
	f.Add([]byte("#cloud-config\nkey: [unclosed\n"))
	f.Add([]byte("#cloud-config\n!!binary not-base64\n"))
	f.Add([]byte("#cloud-config\na: &x\nb: *y\n"))
	f.Add([]byte("#cloud-config\n\x00\x01\x02\n"))
	// The list-valued formats given something that is not a list.
	f.Add([]byte("#cloud-config-archive\nhostname: vm\n"))
	f.Add([]byte("#cloud-config-jsonp\n{\"op\": \"add\"}\n"))
	f.Add([]byte("#cloud-config-archive\n- - - - - -\n"))
	// The MIME header just inside and just past mimeSearchLimit.
	f.Add(append(bytes.Repeat([]byte("#"), mimeSearchLimit-len("MIME-Version:")), []byte("MIME-Version: 1.0\n")...))
	f.Add(append(bytes.Repeat([]byte("#"), mimeSearchLimit), []byte("MIME-Version: 1.0\n")...))
	// A long first line, including one of multi-byte runes.
	f.Add([]byte(strings.Repeat("a", 4096)))
	f.Add([]byte(strings.Repeat("\xd0\xb0", 4096)))
	f.Add([]byte("aaaa\xd0\xb0rest"))
	// Bytes that are not UTF-8 at all.
	f.Add([]byte("\xff\xfe\xfd"))
	f.Add([]byte("aaaa\xffb"))
	// Payloads that cost the parser far more than they weigh.
	f.Add(nestedCloudConfig(360))
	f.Add(wideCloudConfig(4096))
	f.Add([]byte("#cloud-config\nkey: " + strings.Repeat("v", 32<<10) + "\n"))
	f.Add([]byte("#cloud-config\n" + strings.Repeat("[", 4096)))
	f.Add([]byte("#cloud-config-archive\n" + strings.Repeat("- ", 8192) + "x\n"))
	// A YAML alias graph, the billion-laughs shape.
	f.Add([]byte("#cloud-config\na: &a [x, x]\nb: &b [*a, *a]\nc: [*b, *b]\n"))

	f.Fuzz(func(t *testing.T, data []byte) {
		if len(data) > fuzzMaxInput {
			t.Skip()
		}

		requireBoundedValidation(t, data)
	})
}

// requireBoundedValidation checks bounded allocation and warnings safe to put
// in front of a user.
func requireBoundedValidation(t *testing.T, data []byte) {
	t.Helper()

	before := allocatedBytes()
	warnings := ValidateUserData(data)
	allocated := allocatedBytes() - before

	if limit := uint64(len(data))*fuzzAllocFactor + fuzzAllocOverhead; allocated > limit {
		t.Fatalf(
			"validating %d bytes allocated %d bytes, which is past the ceiling of %d: the payload amplifies by a factor of %d",
			len(data), allocated, limit, allocated/uint64(max(len(data), 1)),
		)
	}

	for i, warning := range warnings {
		if warning == "" {
			t.Fatalf("warning %d is empty", i)
		}
		if !utf8.ValidString(warning) {
			t.Fatalf("warning %d is not valid UTF-8: %q", i, warning)
		}
		if len(warning) > fuzzMaxWarningLen {
			t.Fatalf(
				"warning %d is %d bytes long, past the bound of %d, for a payload of %d bytes: %q...",
				i, len(warning), fuzzMaxWarningLen, len(data), warning[:fuzzMaxWarningLen],
			)
		}
	}
}

// allocatedBytes reports total bytes allocated by the process, so the
// difference across a call is its allocation cost. The counter is process-wide,
// hence the constant term in the ceiling. runtime/metrics rather than
// runtime.ReadMemStats, which stops the world.
var allocsSample = []metrics.Sample{{Name: "/gc/heap/allocs:bytes"}}

func allocatedBytes() uint64 {
	metrics.Read(allocsSample)
	return allocsSample[0].Value.Uint64()
}

func gzipped(data []byte) []byte {
	var buf bytes.Buffer

	w := gzip.NewWriter(&buf)
	if _, err := w.Write(data); err != nil {
		panic(err)
	}
	if err := w.Close(); err != nil {
		panic(err)
	}

	return buf.Bytes()
}

// nestedCloudConfig builds a cloud-config nested depth levels deep.
func nestedCloudConfig(depth int) []byte {
	var buf bytes.Buffer

	buf.WriteString("#cloud-config\n")
	for i := 0; i < depth; i++ {
		buf.WriteString(strings.Repeat(" ", i))
		buf.WriteString("k:\n")
	}
	buf.WriteString(strings.Repeat(" ", depth))
	buf.WriteString("v\n")

	return buf.Bytes()
}

// wideCloudConfig spends the same order of bytes on breadth instead of depth.
func wideCloudConfig(keys int) []byte {
	var buf bytes.Buffer

	buf.WriteString("#cloud-config\n")
	for i := 0; i < keys; i++ {
		buf.WriteString("k")
		buf.WriteString(strconv.Itoa(i))
		buf.WriteString(": v\n")
	}

	return buf.Bytes()
}
