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

package image

import (
	"errors"
	"net/url"
	"strings"
	"testing"

	metrics "github.com/deckhouse/virtualization/images/pvc-artifact/pkg/monitoring/metrics/pvc-importer"
	"github.com/deckhouse/virtualization/images/pvc-artifact/pkg/system"
)

// Seeds carrying non-ASCII text are written as \x byte escapes rather than as
// literal characters, so this file stays ASCII-only and the CI linters that
// reject non-ASCII source have nothing to flag. The escapes encode the same
// UTF-8 bytes; do not "simplify" them back into literals.

const (
	// fuzzMaxInput is a technical cap only. The parsing under test works on a
	// few hundred bytes of tool output; larger inputs only cost time.
	fuzzMaxInput = 64 << 10

	// fuzzImageName stands in for the URL the image was read from. It only ends
	// up in error messages.
	fuzzImageName = "nbd+unix:///?socket=/tmp/fuzz.sock"
)

// FuzzQemuImgInfo drives the parsing and the validation of the JSON that
// qemu-img info prints. The importer runs qemu-img against the imported image,
// so the numbers and strings in that output are derived from the image itself:
// the format, the virtual size and the backing file reference are all chosen by
// whoever crafted the disk. Rejecting the output is the normal case; accepting
// an image that does not fit the target, or one in a format the import cannot
// handle, is the finding.
func FuzzQemuImgInfo(f *testing.F) {
	const gigabyte = int64(1) << 30

	// The shapes qemu-img actually prints, for every format the import accepts.
	f.Add([]byte(`{"virtual-size":1048576,"filename":"disk.img","format":"qcow2","actual-size":200704,"dirty-flag":false}`), gigabyte)
	f.Add([]byte(`{"virtual-size":1048576,"filename":"disk.img","format":"raw","actual-size":1048576}`), gigabyte)
	f.Add([]byte(`{"format":"vmdk","virtual-size":1048576,"actual-size":8192}`), gigabyte)
	f.Add([]byte(`{"format":"vdi","virtual-size":1048576,"actual-size":8192}`), gigabyte)
	f.Add([]byte(`{"format":"vpc","virtual-size":1048576,"actual-size":8192}`), gigabyte)
	f.Add([]byte(`{"format":"vhdx","virtual-size":1048576,"actual-size":8192}`), gigabyte)
	// Formats the import must refuse, including the ones qemu-img knows but this
	// module does not.
	f.Add([]byte(`{"format":"iso","virtual-size":1048576}`), gigabyte)
	f.Add([]byte(`{"format":"luks","virtual-size":1048576}`), gigabyte)
	f.Add([]byte(`{"format":"nbd","virtual-size":1048576}`), gigabyte)
	f.Add([]byte(`{"format":"","virtual-size":1048576}`), gigabyte)
	f.Add([]byte(`{"format":"QCOW2","virtual-size":1048576}`), gigabyte)
	f.Add([]byte("{\"format\":\"\xd1\x81\xd1\x8b\xd1\x80\xd0\xbe\xd0\xb9\",\"virtual-size\":1048576}"), gigabyte)
	// Sizes at and beyond the boundary of the destination.
	f.Add([]byte(`{"format":"qcow2","virtual-size":1073741824}`), gigabyte)
	f.Add([]byte(`{"format":"qcow2","virtual-size":1073741825}`), gigabyte)
	f.Add([]byte(`{"format":"raw","virtual-size":9223372036854775807}`), int64(0))
	f.Add([]byte(`{"format":"raw","virtual-size":-1}`), int64(0))
	f.Add([]byte(`{"format":"raw","virtual-size":0}`), int64(-1))
	f.Add([]byte(`{"format":"raw","virtual-size":9223372036854775808}`), gigabyte)
	f.Add([]byte(`{"format":"raw","virtual-size":1e999}`), gigabyte)
	f.Add([]byte(`{"format":"raw","virtual-size":1.5}`), gigabyte)
	f.Add([]byte(`{"format":"raw","virtual-size":"1048576"}`), gigabyte)
	// Backing file references, which the validation resolves on the local
	// filesystem of the importer pod.
	f.Add([]byte(`{"format":"qcow2","virtual-size":1048576,"backing-filename":"/etc/shadow"}`), gigabyte)
	f.Add([]byte(`{"format":"qcow2","virtual-size":1048576,"backing-filename":"../../../etc/passwd"}`), gigabyte)
	f.Add([]byte(`{"format":"qcow2","virtual-size":1048576,"backing-filename":""}`), gigabyte)
	f.Add([]byte(`{"format":"qcow2","virtual-size":1048576,"backing-filename":"/dev/pvc-importer-block-volume"}`), gigabyte)
	f.Add([]byte(`{"format":"qcow2","virtual-size":1048576,"backing-filename":"`+strings.Repeat("a", 8192)+`"}`), gigabyte)
	// Output that is not the expected JSON object at all. The first one is what
	// qemu-img prints when it cannot open the image, and the importer feeds it
	// to the same parser.
	f.Add([]byte(`qemu-img: Could not open 'disk.img': Image is not in qcow2 format`), gigabyte)
	f.Add([]byte(``), gigabyte)
	f.Add([]byte(`null`), gigabyte)
	f.Add([]byte(`[]`), gigabyte)
	f.Add([]byte(`[{"format":"raw","virtual-size":1048576}]`), gigabyte)
	f.Add([]byte(`{`), gigabyte)
	f.Add([]byte(`{"format":123}`), gigabyte)
	f.Add([]byte(`{"format":null,"virtual-size":null}`), gigabyte)
	f.Add([]byte(`{"format":{"name":"raw"}}`), gigabyte)
	f.Add([]byte(`{"format":"raw","format":"qcow2","virtual-size":1048576}`), gigabyte)
	f.Add([]byte(`{"format":"raw","virtual-size":NaN}`), gigabyte)
	f.Add([]byte(`{"format":"ra\xffw","virtual-size":1}`), gigabyte)
	f.Add([]byte("{\"format\":\"raw\\u0000\",\"virtual-size\":1}"), gigabyte)
	f.Add([]byte(strings.Repeat(`{"a":`, 1024)+`1`+strings.Repeat(`}`, 1024)), gigabyte)
	f.Add([]byte(`{"format":"`+strings.Repeat("r", 32<<10)+`"}`), gigabyte)

	f.Fuzz(func(t *testing.T, output []byte, availableSize int64) {
		if len(output) > fuzzMaxInput {
			t.Skip()
		}

		info, err := checkOutputQemuImgInfo(output, fuzzImageName)
		if err != nil {
			if info != nil {
				t.Fatalf("rejected output still produced the image info %+v", *info)
			}

			return
		}
		if info == nil {
			t.Fatalf("accepted output produced no image info")
		}

		if err := checkIfURLIsValid(info, availableSize, fuzzImageName); err != nil {
			return
		}

		// An accepted image is converted next, so its format has to be one
		// qemu-img convert is driven with, and it has to fit the destination.
		if !isSupportedFormat(info.Format) {
			t.Fatalf("format %q was accepted for conversion", info.Format)
		}
		if info.VirtualSize > availableSize {
			t.Fatalf("virtual size %d was accepted for %d bytes of available space", info.VirtualSize, availableSize)
		}
	})
}

// FuzzQemuInfo drives qemuOperations.Info, the wrapper that asks qemu-img about
// an image. The URL it is given is derived from the imported data - for a
// registry source it is built from the name of the file found inside the image
// layer - and the output it parses comes from a tool run against attacker
// controlled bytes. qemu-img itself is not executed: the exec seam is replaced,
// so one iteration stays fast and the invariants are about how the wrapper
// invokes the tool and how it treats the answer.
func FuzzQemuInfo(f *testing.F) {
	// The two shapes the importer builds: an nbd+unix socket for the streaming
	// path and a plain path for a file in scratch space.
	f.Add("nbd+unix:///?socket=/tmp/nbd.sock", false)
	f.Add("nbd+unix:///?socket=/tmp/nbd.sock", true)
	f.Add("nbd+unix:///?socket=/tmp/nbd.sock&debug=1", false)
	f.Add("nbd+unix:///?socket=/tmp/nbd.sock#fragment", true)
	f.Add("file:///data/disk.img", false)
	f.Add("file:///data/disk.img", true)
	f.Add("/data/disk.img", false)
	f.Add("/scratch/disk/disk.img", false)
	f.Add("/data/disk.img", true)
	f.Add("", false)
	// Schemes the wrapper has to refuse before running anything.
	f.Add("http://example.com/disk.qcow2", false)
	f.Add("https://example.com/disk.qcow2", true)
	f.Add("ftp://example.com/disk.qcow2", false)
	f.Add("nbd://127.0.0.1:10809/", false)
	f.Add("javascript:alert(1)", false)
	f.Add("data:application/octet-stream;base64,QUFBQQ==", false)
	// Scheme spellings url.Parse normalizes, and one it rejects outright.
	f.Add("NBD+UNIX:///?socket=/tmp/nbd.sock", false)
	f.Add("File:///data/disk.img", false)
	f.Add("nbd+unix", false)
	// Names that look like options rather than files, which is what the tool
	// would see if the URL ever started with a dash.
	f.Add("--image-opts", false)
	f.Add("-f raw /data/disk.img", false)
	f.Add("--output=json", true)
	// Traversal, hosts, separators and spacing inside the reference.
	f.Add("file:///data/../../etc/shadow", false)
	f.Add("file://localhost/data/disk.img", false)
	f.Add("//data/disk.img", false)
	f.Add("./disk.img", false)
	f.Add(`\\server\share\disk.img`, false)
	f.Add("/data/disk with spaces.img", false)
	// Percent encoding, control bytes, non-ASCII and oversized references.
	f.Add("/data/disk%00.img", false)
	f.Add("/data/disk\x00.img", false)
	f.Add("%%", false)
	f.Add("/data/\xd0\xb4\xd0\xb8\xd1\x81\xd0\xba.img", false)
	f.Add("nbd+unix:///?socket=/tmp/"+strings.Repeat("a", 8192), false)

	f.Fuzz(func(t *testing.T, rawURL string, execFails bool) {
		if len(rawURL) > fuzzMaxInput {
			t.Skip()
		}

		parsed, err := url.Parse(rawURL)
		if err != nil {
			// The importer parses the reference before it gets here, so an
			// unparseable string never reaches Info.
			return
		}

		var (
			calls       int
			gotLimits   *system.ProcessLimitValues
			gotCommand  string
			gotArgs     []string
			execFailure = errors.New("qemu-img failed")
		)

		restore := qemuExecFunction
		defer func() { qemuExecFunction = restore }()

		qemuExecFunction = func(limits *system.ProcessLimitValues, _ func(string), command string, args ...string) ([]byte, error) {
			calls++
			gotLimits, gotCommand, gotArgs = limits, command, args

			if execFails {
				return []byte("qemu-img: Could not open image"), execFailure
			}

			return []byte(`{"format":"qcow2","virtual-size":1048576,"actual-size":200704}`), nil
		}

		info, err := NewQEMUOperations().Info(parsed)

		if calls > 1 {
			t.Fatalf("URL %q made qemu-img run %d times", rawURL, calls)
		}

		if calls == 0 {
			// The only reason not to run the tool is the scheme guard.
			if err == nil {
				t.Fatalf("URL %q was accepted without asking qemu-img", rawURL)
			}
			if info != nil {
				t.Fatalf("URL %q produced image info %+v without asking qemu-img", rawURL, *info)
			}
			if scheme := parsed.Scheme; scheme == "" || scheme == "nbd+unix" || scheme == "file" {
				t.Fatalf("URL %q with the allowed scheme %q never reached qemu-img", rawURL, scheme)
			}

			return
		}

		// The resource limits on the parsing are the mitigation the threat model
		// credits this path with, so they have to be applied on every run.
		if gotLimits == nil {
			t.Fatalf("qemu-img ran without resource limits for %q", rawURL)
		}
		if gotLimits.AddressSpaceLimit != maxMemory {
			t.Fatalf("address space limit is %d, want %d", gotLimits.AddressSpaceLimit, uint64(maxMemory))
		}
		if gotLimits.CPUTimeLimit != maxCPUSecs {
			t.Fatalf("CPU time limit is %d, want %d", gotLimits.CPUTimeLimit, uint64(maxCPUSecs))
		}

		// The reference is passed as a single argument to a fixed command line:
		// no shell, no extra flags, and the answer is asked for as JSON.
		if gotCommand != "qemu-img" {
			t.Fatalf("ran %q instead of qemu-img", gotCommand)
		}
		if len(gotArgs) != 3 || gotArgs[0] != "info" || gotArgs[1] != "--output=json" {
			t.Fatalf("qemu-img ran with the arguments %q", gotArgs)
		}
		if gotArgs[2] != parsed.String() {
			t.Fatalf("qemu-img was given %q instead of the reference %q", gotArgs[2], parsed.String())
		}

		switch {
		case execFails:
			if err == nil {
				t.Fatalf("a failed qemu-img run was reported as success")
			}
			if info != nil {
				t.Fatalf("a failed qemu-img run produced image info %+v", *info)
			}
		case err != nil:
			t.Fatalf("a successful qemu-img run was rejected: %v", err)
		case info == nil:
			t.Fatalf("a successful qemu-img run produced no image info")
		case info.Format != "qcow2" || info.VirtualSize != 1<<20:
			t.Fatalf("the parsed image info %+v does not match the output", *info)
		}
	})
}

// FuzzQemuProgress drives the parsing of the progress output of qemu-img, which
// the importer reads line by line while the conversion runs and publishes as the
// import progress of the disk. A misparsed line is reported to Prometheus as the
// progress of a real import, so the value has to stay inside the phase it
// belongs to and it must never move backwards.
func FuzzQemuProgress(f *testing.F) {
	// The shape qemu-img convert -p writes.
	f.Add("    (0.00/100%)")
	f.Add("    (0.01/100%)")
	f.Add("    (1.00/100%)")
	f.Add("    (9.99/100%)")
	f.Add("    (50.00/100%)")
	f.Add("    (75.50/100%)")
	f.Add("    (99.99/100%)")
	f.Add("(100.00/100%)")
	f.Add("(50.00/100%)\r")
	f.Add("\t(50.00/100%)\n")
	// Two matches in one line: only the first one may be taken.
	f.Add("(50.00/100%) (75.00/100%)")
	f.Add(strings.Repeat("(50.00/100%)", 512))
	f.Add(strings.Repeat(" ", 32<<10) + "(50.00/100%)")
	// Shapes the parser has to ignore, including the nbdcopy one.
	f.Add("")
	f.Add("\n")
	f.Add("(")
	f.Add("()")
	f.Add("100/100")
	f.Add("(50.0/100%)")
	f.Add("(.00/100%)")
	f.Add("(50.000/100%)")
	f.Add("(50.00/100)")
	f.Add("(50.00/1000%)")
	f.Add("(50,00/100%)")
	f.Add("50.00/100%")
	f.Add("(-1.00/100%)")
	f.Add("(1e2.00/100%)")
	f.Add("qemu-img: error while writing at byte 0: No space left on device")
	f.Add("nbdkit: curl: error: HTTP 404")
	// Non-ASCII, invalid UTF-8 and control bytes around a valid match.
	f.Add("(50.00/100%) \xd0\xbf\xd1\x80\xd0\xbe\xd0\xb3\xd1\x80\xd0\xb5\xd1\x81\xd1\x81")
	f.Add("\xd0\xbf\xd1\x80\xd0\xbe\xd0\xb3\xd1\x80\xd0\xb5\xd1\x81\xd1\x81 (50.00/100%)")
	f.Add("\xff\xfe(50.00/100%)")
	f.Add("(50.00/100%)\x00")
	f.Add("(\xc2\xbd.00/100%)")

	f.Fuzz(func(t *testing.T, line string) {
		if len(line) > fuzzMaxInput {
			t.Skip()
		}

		// The convert phase fills the upper half of the counter, so a match has
		// to land in [convertProgressBase, 100].
		requireProgressInRange(t, "fuzz-qemu-progress-convert", line, reportProgress, convertProgressBase, 100)

		// The direct nbdkit path owns the whole counter.
		requireProgressInRange(t, "fuzz-qemu-progress-full", line, reportProgressFull, 0, 100)
	})
}

// requireProgressInRange runs one of the progress reporters against a fresh
// counter and pins the contract of the value it publishes: it stays inside the
// phase bounds, only a line carrying the qemu-img progress marker moves it, and
// replaying the same line does not move it again.
func requireProgressInRange(t *testing.T, uid, line string, report func(string), low, high float64) {
	t.Helper()

	// Each check starts from a counter of its own, otherwise the verdict would
	// depend on the previous input. ownerUID is the global the reporters read.
	ownerUID = uid
	metrics.Progress(uid).Delete()

	report(line)

	progress, err := metrics.Progress(uid).Get()
	if err != nil {
		t.Fatalf("reading the progress counter failed: %v", err)
	}
	if progress != 0 && (progress < low || progress > high) {
		t.Fatalf("line %q reported progress %v outside [%v, %v]", line, progress, low, high)
	}
	if progress != 0 && !strings.Contains(line, "/100%)") {
		t.Fatalf("line %q was accepted as progress %v", line, progress)
	}

	report(line)

	again, err := metrics.Progress(uid).Get()
	if err != nil {
		t.Fatalf("reading the progress counter failed: %v", err)
	}
	if again != progress {
		t.Fatalf("replaying line %q moved the progress from %v to %v", line, progress, again)
	}

	// An empty owner UID means the importer runs without a progress metric,
	// which must stay a no-op whatever the line says.
	ownerUID = ""
	report(line)
}
