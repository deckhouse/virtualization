# Fuzzing

Native Go fuzzing (`testing.F`) over the untrusted-input paths of `pvc-artifact`, the module
behind `pvc-importer` and `pvc-target-importer`: disk image format detection, the wrapper
around `qemu-img`, the unpacking of a container image layer, and the progress output of the
external copy tools.

All targets live in this module and none of them spawns a subprocess, opens a socket or
touches a real Kubernetes API.

Contents:

- [Targets](#targets)
- [Traceability](#traceability)
- [Seed corpus convention](#seed-corpus-convention)
- [Running a target](#running-a-target)
- [Where the corpus lives](#where-the-corpus-lives)
- [What the targets do not cover](#what-the-targets-do-not-cover)
- [Findings and observations](#findings-and-observations)

## Targets

Seven targets. The seed counts were read off a plain
`go test -run '^Fuzz' -v` run of the two packages, counting the `FuzzX/seed#…` subtests, and
cross-checked against the `testing seed corpus: N/N completed` line of a fuzzing run.

| Target | Package | Seeds | Covers |
| --- | --- | --- | --- |
| `FuzzFormatReaders` | `pkg/importer` | 49 | image format detection and the compression unwrapping of `format-readers.go`, including `image.Header.Match`/`Size` and the qcow2 size parse |
| `FuzzSafeJoinPaths` | `pkg/importer` | 35 | the zip-slip guard applied to the tar entry names of a container image layer |
| `FuzzEnvsToLabels` | `pkg/importer` | 31 | conversion of the image's environment variables into the labels reported through the termination message |
| `FuzzNbdcopyProgress` | `pkg/importer` | 35 | parsing of the progress output `nbdcopy` writes to the pipe the target importer reads |
| `FuzzQemuImgInfo` | `pkg/image` | 41 | `checkOutputQemuImgInfo` and `checkIfURLIsValid`: the JSON `qemu-img info` prints and the format/size validation applied to it |
| `FuzzQemuInfo` | `pkg/image` | 33 | `qemuOperations.Info`: the URL scheme guard, the command line `qemu-img` is invoked with, and the resource limits applied to it |
| `FuzzQemuProgress` | `pkg/image` | 34 | parsing of the progress lines `qemu-img convert -p` writes, in both the convert-phase and the full-range projection |

The two files are `pkg/importer/importer_fuzz_test.go` and `pkg/image/qemu_fuzz_test.go`. Both
sit in the same package as the unit tests they were derived from
(`pkg/importer/nbdcopy_test.go`, `pkg/image/qemu_progress_test.go`), so they reuse the
package-internal functions directly instead of exporting or duplicating anything.

## Traceability

The module's threat model assigns fuzzing of disk image format parsing to three components -
`dvcr-importer`, `dvcr-uploader` and `pvc-importer` - under the threat of a compromise through
an uploaded VM disk image. For `pvc-importer` it asks for robustness of the qcow2, vmdk, vdi,
vhdx, vpc and iso parsing against malformed and malicious data, for the wrapper that
determines the image information (`qemuOperations.Info`), and for the correctness of the
resource limits placed on that parsing. The seven targets above cover those three things; the
seed corpus covers all six formats.

The threat model is being revised and is known to contain inaccuracies, so nothing here is
argued from it: where the document and the code disagree, this file describes what the code
does.

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
  wherever the package is tested. Nothing tests this module in CI today: the pipeline runs
  unit tests for `images/virtualization-artifact` and the hooks only, and no fuzz image is
  published either, so the seeds are checked only when someone runs them locally.

Every input is capped at 64 KiB with `t.Skip`. That is a technical limit against pointless
memory and time, never a filter on malformed data - malformed data is what these paths exist
to reject.

## Running a target

The whole module is Linux-only: `pkg/util` uses `syscall.Fallocate` and
`unix.FALLOC_FL_PUNCH_HOLE` / `FALLOC_FL_KEEP_SIZE`, and `GetAvailableSpace` relies on the
Linux field types of `syscall.Statfs_t`. It does not build on macOS, so neither
`go test` nor `golangci-lint` works there without `GOOS=linux`.

**No external binary is required.** `qemu-img`, `nbdkit`, `nbdcopy` and `file` are runtime
dependencies of the importer, not of these targets: `FuzzQemuInfo` replaces the exec seam
(`qemuExecFunction`) with a stub, and every other target works on bytes and strings only. A
run in a bare `debian:bookworm-slim` exercises the same code as a run in the importer image -
nothing is silently short-circuited by a missing tool.

### Through the Taskfile (the platform ABI)

`Taskfile.yaml` exposes `fuzz:list`, `fuzz:run`, `fuzz:coverage` and `fuzz:replay`, driven by
`FUZZ_PKG`, `FUZZ_TARGET`, `FUZZ_WORKERS` and `FUZZ_COVERAGE_FILE`. There is deliberately no
duration variable: the stopping policy belongs to the platform. From the repository root the
tasks are reachable as `task pvc-artifact:fuzz:list` and so on.

These are the tasks the external fuzzing platform calls, but it reaches them only through a
published fuzz image, and this repository does not build one — a test-only image must not be
able to break the module build. So today the tasks are for local use, and `fuzz:local:*` below
is what runs them on the architecture the component ships as.

```bash
task pvc-artifact:fuzz:list

FUZZ_PKG=./pkg/image FUZZ_TARGET=FuzzQemuImgInfo FUZZ_WORKERS=4 \
  task pvc-artifact:fuzz:run

FUZZ_PKG=./pkg/importer FUZZ_TARGET=FuzzFormatReaders \
  FUZZ_COVERAGE_FILE=/tmp/fuzz.cover task pvc-artifact:fuzz:coverage
```

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
task fuzz:local:list                                 # the seven targets
task fuzz:local:seeds                                # replay every seed corpus

FUZZ_PKG=./pkg/image FUZZ_TARGET=FuzzQemuImgInfo FUZZ_TIME=60s FUZZ_WORKERS=4 \
  task fuzz:local:run
```

Inside the container the four ABI tasks run unchanged - `fuzz:local:*` only provides the
container and the environment. The one addition is `FUZZ_TIME`, which `fuzz:local:run` hands to
`go test` through `GOFLAGS=-fuzztime=…`, because `fuzz:run` itself takes no duration by
contract.

**The image is linux/amd64, and on an arm64 host it is emulated.** The build takes minutes, and
a fuzzing run stalls at `0/sec` for seconds at a time while still making progress. Measured on
an arm64 macOS host, coverage guidance active in both: `FuzzQemuImgInfo` with 4 workers, 299868
execs and 29 new interesting inputs in 41 s; `FuzzFormatReaders`, 538379 execs and 3 in 26 s.
Read those numbers as a local sanity check, never as a throughput figure for a native amd64
machine.

**Dependencies are mounted, not baked in.** Contrary to what this document said before, the
module does *not* resolve on its own: `go.work` replaces `github.com/docker/docker` with the
stub in `staging/`, so `go.sum` has no hashes for the upstream module and `GOWORK=off` fails
with `missing go.sum entry for module providing package
github.com/docker/docker/api/types/versions`. With the workspace active the build list is the
union of every module in `go.work`, which reaches `kubevirt.io/api` on an internal GitLab host.
The tasks therefore mount the host module cache read-only and set `GOPROXY=off`; the
alternatives - copying a 9 GB cache into the image, or vendoring the workspace - cost far more
and buy nothing. The precondition is that the host has built this repository once. A cold cache
fails immediately and legibly (`module lookup disabled by GOPROXY=off`) instead of hanging on an
unreachable host.

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

Nothing crashed. Every target survives its seed corpus and a fuzzing run; the longest run so
far is 24.3M executions of `FuzzFormatReaders`, without coverage guidance.

The items below were found while choosing the targets and confirmed with a throwaway probe
that was removed afterwards. None of them is fixed here, and no production code was changed by
this work.

| Observation | Where | Why it is not asserted |
| --- | --- | --- |
| A malformed `IMPORTER_IMAGE_SIZE` panics the importer. The value is read from the environment and passed straight to `NewDataProcessor`, which calls `resource.MustParse` while computing the target size. Probed: `not-a-quantity` panics with `cannot parse 'not-a-quantity'`. The result is a crash-looping pod instead of a reported error, and `NewDataProcessor` returns no error to report it with. | `pkg/importer/data-processor.go:385`, called from `cmd/pvc-importer/importer.go:87` and `:186` | Fixing it changes the signature of `NewDataProcessor`, which is out of scope for this work. A target would fail on its own first seed, so the panic is recorded here instead. |
| The nbdcopy progress parser trusts the magnitude of the value: `1e300/100` sets the exported progress counter to `1e+300` and `+Inf/100` sets it to `+Inf`. Only the shape and the sign are checked. | `pkg/importer/nbdcopy.go:102` | The counter is fed by a same-pod child process, so this is robustness rather than a boundary crossing. `FuzzNbdcopyProgress` asserts the shape, the monotonicity and the absence of a negative delta, which is what a counter must not violate. The qemu-img parser is not affected: its regexp caps the value at 99.99. |
| Image environment variables become label keys that Kubernetes would reject. Probed: `KUBEVIRT_IO_ OS =x` becomes `kubevirt.io/ os `, `KUBEVIRT_IO_OS/NAME=x` becomes `kubevirt.io/os/name`, and a Cyrillic name becomes a Cyrillic key. The names come from the image config of the imported registry image. | `pkg/importer/util.go:89` and `:102` | Where these labels are applied is decided outside this module, so the consequence cannot be pinned from here. `FuzzEnvsToLabels` asserts the properties the conversion owns: the namespace, the lower casing and the verbatim values. |
| One oversized environment value makes the importer unable to report anything at all: an 8 KiB value produces a termination message of 8224 bytes, and `TerminationMessage.String` refuses to serialize more than the kubelet's 4096. Probed. | `pkg/common/common.go` (`String`), fed from `pkg/importer/util.go:89` | The truncation policy belongs to the caller that writes the message, not to the parser under test. |
| A payload that matches several magic numbers at once is resolved in map iteration order, one magic per round of `constructReaders`, because `matchHeader` re-offers the 512 byte header to the next round through a multi-reader. The final `ImageFormat` is stable - probed 200 times on a payload carrying both the qcow2 and the tar magic, always `qcow2` - but the reader stack grows by one multi-reader per round. | `pkg/importer/format-readers.go:99` and `:257` | Stable in outcome, so there is nothing to assert beyond the format consistency `FuzzFormatReaders` already checks. |
| `Info` accepts a URL with no scheme and passes it to `qemu-img` as the image argument, so a reference beginning with a dash would be read by the tool as an option rather than a file name. Not reachable today: the registry path builds the reference by joining the scratch directory with the file name found in the layer, which always yields an absolute path, and the streaming path builds an `nbd+unix` URL. | `pkg/image/qemu.go:226` | Asserting "the argument never starts with a dash" would fail on fuzzed input while nothing in production can produce it. `FuzzQemuInfo` asserts the scheme guard, the fixed argument list and the resource limits instead. |
| The format vocabularies differ between the two halves of the import: the header detection reports `vhd`, while the `qemu-img` allow-list spells the same format `vpc`. Harmless today - the detection's name is only compared against `raw`/`qcow2` for the direct transfer - but the two lists have to be kept in step by hand. | `pkg/image/filefmt.go:86` and `pkg/image/qemu.go:241` | A naming mismatch, not a property of a single function. |
