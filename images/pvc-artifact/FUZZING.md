# Fuzzing

Native Go fuzzing (`testing.F`) over the untrusted-input paths of `pvc-artifact`, the module
behind `pvc-importer` and `pvc-target-importer`: disk image format detection, the wrapper
around `qemu-img`, the unpacking of a container image layer, the progress output of the
external copy tools, the image size the controller hands over, and the metrics listener the
importer Pod opens.

All targets live in this module and none of them spawns a subprocess or touches a real
Kubernetes API. One, `FuzzPrometheusEndpoint`, opens a loopback listener of its own.

Contents:

- [Targets](#targets)
- [Traceability](#traceability)
- [Seed corpus convention](#seed-corpus-convention)
- [Running a target](#running-a-target)
- [Where the corpus lives](#where-the-corpus-lives)
- [What the targets do not cover](#what-the-targets-do-not-cover)
- [Findings and observations](#findings-and-observations)

## Targets

Nine targets. The seed counts were read off `task fuzz:local:seeds`, counting the
`FuzzX/seed#…` subtests.

| Target | Package | Seeds | Covers |
| --- | --- | --- | --- |
| `FuzzFormatReaders` | `pkg/importer` | 49 | image format detection and the compression unwrapping of `format-readers.go`, including `image.Header.Match`/`Size` and the qcow2 size parse |
| `FuzzSafeJoinPaths` | `pkg/importer` | 35 | the zip-slip guard applied to the tar entry names of a container image layer |
| `FuzzEnvsToLabels` | `pkg/importer` | 31 | conversion of the image's environment variables into the labels reported through the termination message |
| `FuzzNbdcopyProgress` | `pkg/importer` | 35 | parsing of the progress output `nbdcopy` writes to the pipe the target importer reads |
| `FuzzParseImageSize` | `pkg/importer` | 36 | parsing of the image size the controller passes through the environment: a rejected value is named in the error, an accepted one survives its own canonical form |
| `FuzzQemuImgInfo` | `pkg/image` | 41 | `checkOutputQemuImgInfo` and `checkIfURLIsValid`: the JSON `qemu-img info` prints and the format/size validation applied to it |
| `FuzzQemuInfo` | `pkg/image` | 33 | `qemuOperations.Info`: the URL scheme guard, the command line `qemu-img` is invoked with, and the resource limits applied to it |
| `FuzzQemuProgress` | `pkg/image` | 34 | parsing of the progress lines `qemu-img convert -p` writes, in both the convert-phase and the full-range projection |
| `FuzzPrometheusEndpoint` | `pkg/util/prometheus` | 45 | the TLS metrics listener of the importer Pod: an arbitrary request must not panic it, the response stays bounded, the port keeps answering a scrape afterwards, and the metric families stay within the expected prefixes |

Seven of them are the files of `main` — `pkg/importer/importer_fuzz_test.go` and
`pkg/image/qemu_fuzz_test.go` — with the import paths of this module, which is still named
`kubevirt.io/containerized-data-importer` here. The code under them is the same on both
branches with one exception: `FormatReaders` has no `ImageFormat` field on this branch, so
`FuzzFormatReaders` does not check the detected format and asserts instead that the reader
stack handed to the caller is usable, that no payload panics or hangs it, and that the stream
drains up to the limit. The `matchHeader` it exercises is the older CDI edition: a short read
is an error, and the header buffer is re-offered to the next round without a copy.

## Traceability

The threat model (`virtualization-threat-model.md`, the `Fuzzing` row of the security
test plan) lists these nine targets as the coverage of `pvc-importer`: image format parsing,
the `qemu-img` wrapper and the resource limits placed on it, the entry names of a layer, and
the metrics port. The targets of `main` that are not here — `FuzzChecksums` and the cloud-init
validation — stand on code this branch does not carry, and the same row records them as
outstanding.

## Seed corpus convention

**Every target carries at least 20 seeds, spelled out in the test file.** This is a
certification requirement, not a style preference. The lowest count today is 31.

- Seeds must be visible in `*_fuzz_test.go`, enumerated rather than generated. Every target
  here writes one `f.Add` per seed.
- Seeds should be *interesting*, not filler: format magic numbers at their real offsets,
  truncated and corrupted variants of each, boundary and overflowing sizes, path traversal,
  invalid UTF-8, non-ASCII, control bytes, oversized inputs and empty input.
- **Non-ASCII and Cyrillic seeds are allowed.** `task validation:no-cyrillic` skips any file
  matching `_fuzz_test.go$` and reports the skip reason as `fuzz seed corpus`. The exemption
  covers the test files only - this document, like every other `.md` outside `doc-ru-*`, is
  still checked and is therefore English only.
- Seed corpus entries run as ordinary subtests under `go test`, so a broken seed fails
  wherever the package is tested. In CI that place is the fuzz image build: the `-fuzz` image
  replays every target's corpus as its last build step, and a broken seed fails the build. The
  ordinary jobs leave this module alone — the pipeline runs unit tests for
  `images/virtualization-artifact` and the hooks only.

Every input is capped at 64 KiB with `t.Skip`. That is a technical limit against pointless
memory and time, never a filter on malformed data - malformed data is what these paths exist
to reject.

## Running a target

The whole module is Linux-only: `pkg/util` uses `syscall.Fallocate` and
`unix.FALLOC_FL_PUNCH_HOLE` / `FALLOC_FL_KEEP_SIZE`, `pkg/system` uses `SYS_PRLIMIT64`, and
`GetAvailableSpace` relies on the Linux field types of `syscall.Statfs_t`. It does not build on
macOS, so neither `go test` nor `golangci-lint` works there without `GOOS=linux`.

**No external binary is required.** `qemu-img`, `nbdkit`, `nbdcopy` and `file` are runtime
dependencies of the importer, not of these targets: `FuzzQemuInfo` replaces the exec seam
(`qemuExecFunction`) with a stub, and every other target works on bytes and strings or on a
listener it opens itself. A run in a bare `debian:bookworm-slim` exercises the same code as a
run in the importer image - nothing is silently short-circuited by a missing tool.

### Through the Taskfile (the platform ABI)

The repository-root `Taskfile.fuzz.yml` exposes `fuzz:list`, `fuzz:run`, `fuzz:coverage` and
`fuzz:replay`, driven by `FUZZ_PKG`, `FUZZ_TARGET`, `FUZZ_WORKERS` and `FUZZ_COVERAGE_FILE`.
There is deliberately no duration variable: the stopping policy belongs to the platform. This
module's own `Taskfile.yaml` does not duplicate them; from `images/pvc-artifact` on a Linux host
they are reachable as `task -t ../../Taskfile.fuzz.yml fuzz:list` and so on.

These are the tasks the external fuzzing platform calls, and it reaches them through the
`-fuzz` image: `werf.inc.yaml` ends with `{{- include "fuzz image" . }}`, which
[`.werf/defines/fuzz.tmpl`](../../.werf/defines/fuzz.tmpl) turns into
`{ModuleNamePrefix}pvc-artifact-fuzz`, built from the regular `pvc-artifact` image with
`CGO_ENABLED=0`. Inside it `Taskfile.fuzz.yml` is copied into the workdir as `Taskfile.yml`. The
workdir is `/src`: the src-artifact on this branch adds `images/pvc-artifact` itself rather than
the whole repository, so the module root has no repository-shaped tree above it. The image is
`final: false` and the whole template is behind `WERF_BUILD_FUZZ_IMAGES=true`, which only
`build_fuzz_dev` and `build_fuzz_release` set (`.gitlab/ci/jobs/build-fuzz.yml`) — a test-only
image must not be able to break the module build. Its install stage restores the corpus overlay
from S3, discovers the targets with `task fuzz:list` and replays each one; mutation runs stay
the platform's call.

`fuzz:local:*` below runs the same tasks on the architecture the component ships as.

### Native (Linux host)

```bash
cd images/pvc-artifact
go test ./pkg/... -run '^Fuzz' -count=1 -v            # replay the seed corpus
go test ./pkg/image -run '^$' -fuzz='^FuzzQemuInfo$' -fuzztime=60s
```

Anchor `-fuzz`: `-fuzz=FuzzQemu` matches three targets and Go then refuses to fuzz at all.
Pass `-run '^$'` as well, or the package's unit tests and every other target's seed corpus run
first.

### The local loop (linux/amd64 container)

`fuzz.Dockerfile` builds the image, and `Taskfile.yaml` wraps the docker commands as
`fuzz:local:*`, so no docker line is typed by hand. From `images/pvc-artifact`:

```bash
task fuzz:local:image                                # build the image, once
task fuzz:local:list                                 # the nine targets
task fuzz:local:seeds                                # replay every seed corpus

FUZZ_PKG=./pkg/image FUZZ_TARGET=FuzzQemuImgInfo FUZZ_TIME=60s FUZZ_WORKERS=4 \
  task fuzz:local:run
```

Inside the container the ABI tasks of `Taskfile.fuzz.yml` run unchanged, picked with
`task -t /src/Taskfile.fuzz.yml -d /src/images/pvc-artifact` - `fuzz:local:*` only provides the
container and the environment. The one addition is `FUZZ_TIME`, which `fuzz:local:run` hands to
`go test` through `GOFLAGS=-fuzztime=…`, because `fuzz:run` itself takes no duration by
contract.

**The image is linux/amd64, and on an arm64 host it is emulated.** The build takes about two
and a half minutes, and a fuzzing run stalls at `0/sec` for seconds at a time while still making
progress: `FuzzQemuImgInfo` with 2 workers made 106224 execs and 16 new interesting inputs in
21 s on an arm64 macOS host. Read those numbers as a local sanity check, never as a throughput
figure for a native amd64 machine.

**Dependencies are mounted, not baked in.** The module resolves on its own on this branch: there
is no `go.work`, and the `github.com/docker/docker` replace pointing at `staging/` lives in
`go.mod`. The host module cache is still mounted read-only, with `GOPROXY=off` in the image:
the host has the modules already, nothing has to be fetched from inside the container, and a
cold cache fails immediately and legibly (`module lookup disabled by GOPROXY=off`) instead of
downloading as root into the host cache. The precondition is that the host has built this
module once.

`$GOCACHE` is the named volume `pvc-artifact-fuzz-gocache`, so the discovered corpus survives
between runs; `docker volume rm pvc-artifact-fuzz-gocache` resets it.

## Where the corpus lives

| Location | Contents | Reaches git? |
| --- | --- | --- |
| `$(go env GOCACHE)/fuzz/<import path>/<FuzzFunc>/` | every coverage-expanding input the fuzzer finds | no |
| `<package>/testdata/fuzz/<FuzzFunc>/` | crash reproducers only, written when a target fails | yes, it is not gitignored |

There is no `testdata/fuzz` directory in this module today, because no target has failed.
Consequences worth knowing:

- The discovered corpus is not versioned. `go clean -fuzzcache` erases it, and nothing carries
  it into `testdata/fuzz`. Copy it out deliberately if it is worth keeping.
- In the local loop that directory is inside the `pvc-artifact-fuzz-gocache` volume, so it
  outlives the container but is invisible to the host filesystem.
- With coverage guidance the run prints `gathering baseline coverage: N/N` over seeds *plus*
  cached inputs, so `N` drifts above the seed count. To count seeds, replay the target as a
  plain test or run `go clean -fuzzcache` first.
- A container run with the repository mounted can write a reproducer into the repository as
  root. Check `git status` after any container run.
- The corpus the external platform accumulates is keyed by the package import path, which
  starts with the module name. This module is named differently from `main`, so the corpus of
  `main` never applies here; `build_fuzz_dev` reads the corpus of the MR target branch and
  `build_fuzz_release` that of `release-1.10`.

## What the targets do not cover

Rejected deliberately, so nobody re-derives the same dead ends:

- **`util.GetFormat` (`pkg/util/file_format.go`)** - no parsing at all. It stats a path and
  answers `raw` for a device and `qcow2` for anything else. A fuzzer would mutate a path
  string that the function only hands to `os.Stat`; there is no input-dependent branch worth
  exploring, and every iteration would touch the filesystem.
- **`WaitForNBDEndpoint` (`pkg/importer/nbd_wait.go`)** - with a real timeout it dials TCP and
  sleeps in a loop, which fuzzing must not do; with a zero timeout the loop body never runs
  and the only logic left is `url.Parse` plus two field checks, that is, the standard library's
  parser rather than ours. The existing unit tests already pin the four outcomes.
- **`Nbdkit.StartNbdkit` (`pkg/image/nbdkit.go`)** - building the argument list is
  deterministic string concatenation with no parsing, and reaching anything else means
  executing `nbdkit`, waiting on a PID file with a 15 second timeout and reading its log. The
  interesting half, the log the child writes, is consumed by `watchNbdLog`, which only
  reformats lines.
- **`image.Header.Match` and `image.Header.Size` fuzzed directly** - both index the buffer at
  the format's offset without a length check, so any buffer shorter than
  `mgOffset+len(magicNumber)` (262 bytes for tar) or `SizeOff+SizeLen` panics. Production
  cannot reach it: the only caller fills a 512 byte buffer with `io.ReadFull` and gives up on
  a short read. A direct target would report a panic nothing can trigger, so the two functions
  are exercised through `FuzzFormatReaders` instead. Worth remembering before either function
  gains a second caller.
- **`processLayer` / `CopyRegistryImage` (`pkg/importer/transport.go`)** - reaching them needs
  a `types.ImageSource`, and this module has no mocks for it. Their two interesting halves are
  covered separately: the layer's own bytes by `FuzzFormatReaders` and the entry names by
  `FuzzSafeJoinPaths`.
- **`DataProcessor.convert` / `resize` (`pkg/importer/data-processor.go`)** - every branch
  either runs `qemu-img` or writes to the target device.

## Findings and observations

Nothing crashed. Every target survives its seed corpus and the fuzzing runs made while porting:
30 s of `FuzzFormatReaders` (558359 execs) and 21 s of `FuzzQemuImgInfo` in the local loop.

The items below were found on `main` while choosing the targets and re-checked against this
branch's code. None of them is fixed here, and no production code was changed by this work.

| Observation | Where | Why it is not asserted |
| --- | --- | --- |
| The nbdcopy progress parser trusts the magnitude of the value: `1e300/100` sets the exported progress counter to `1e+300` and `+Inf/100` sets it to `+Inf`. Only the shape and the sign are checked. | `pkg/importer/nbdcopy.go:102` | The counter is fed by a same-pod child process, so this is robustness rather than a boundary crossing. `FuzzNbdcopyProgress` asserts the shape, the monotonicity and the absence of a negative delta, which is what a counter must not violate. The qemu-img parser is not affected: its regexp caps the value at 99.99. |
| Image environment variables become label keys that Kubernetes would reject: `KUBEVIRT_IO_ OS =x` becomes `kubevirt.io/ os `, `KUBEVIRT_IO_OS/NAME=x` becomes `kubevirt.io/os/name`, and a Cyrillic name becomes a Cyrillic key. The names come from the image config of the imported registry image. | `pkg/importer/util.go:76` and `:89` | Where these labels are applied is decided outside this module, so the consequence cannot be pinned from here. `FuzzEnvsToLabels` asserts the properties the conversion owns: the namespace, the lower casing and the verbatim values. |
| One oversized environment value makes the importer unable to report anything at all: an 8 KiB value produces a termination message above the kubelet's 4096 bytes, which `TerminationMessage.String` refuses to serialize. | `pkg/common/common.go:66` (`String`), fed from `pkg/importer/util.go:76` | The truncation policy belongs to the caller that writes the message, not to the parser under test. |
| A payload that matches several magic numbers at once is resolved in map iteration order, one magic per round of `constructReaders`, because `matchHeader` re-offers the header to the next round through a multi-reader. On this branch that header is the internal buffer itself, handed on without a copy, and the reader stack grows by one multi-reader per round. | `pkg/importer/format-readers.go:93`, `:130` and `:246` | Stable in outcome, and this branch has no `ImageFormat` to compare against; `FuzzFormatReaders` asserts that the resulting stack stays usable and drains. |
| `Info` accepts a URL with no scheme and passes it to `qemu-img` as the image argument, so a reference beginning with a dash would be read by the tool as an option rather than a file name. Not reachable today: the registry path builds the reference by joining the scratch directory with the file name found in the layer, which always yields an absolute path, and the streaming path builds an `nbd+unix` URL. | `pkg/image/qemu.go:210` | Asserting "the argument never starts with a dash" would fail on fuzzed input while nothing in production can produce it. `FuzzQemuInfo` asserts the scheme guard, the fixed argument list and the resource limits instead. |
| The format vocabularies differ between the two halves of the import: the header detection reports `vhd`, while the `qemu-img` allow-list spells the same format `vpc`. Harmless today, but the two lists have to be kept in step by hand. | `pkg/image/filefmt.go:85` and `pkg/image/qemu.go:227` | A naming mismatch, not a property of a single function. |
| A size above `math.MaxInt64` bytes is accepted by `parseImageSize`, and `resource.Quantity` then misbehaves: `1e21` prints as `1e21` but has `Value()` 0, the same number written out in digits prints as `1` and has a garbage `Value()`, and `10000000000000000000` has a negative one. `ResizeImage` and the target size computation consume the size through `Value()`. Found by `FuzzParseImageSize` during the port. | `pkg/importer/data-processor.go:145`, `:342` and `:400` | The value is `resource.Quantity` behaviour in apimachinery v0.30.2, not this module's parsing; whether the importer should refuse sizes beyond int64 is a production decision. The target asserts the round trip and a non-negative `Value()` for every size up to `math.MaxInt64` and leaves the range above it to this note. |
| The metrics listener serves the default Prometheus registry on any path without authentication, so `go_*` and `process_*` leave the Pod together with the import progress. | `pkg/util/prometheus/prometheus.go` | Recorded in the threat model as an unmet requirement; `FuzzPrometheusEndpoint` pins the family prefixes so a new family is a decision, not an accident. |
