# Fuzzing

Native Go fuzzing (`testing.F`) over the untrusted-input paths of `dvcr-artifact`: image
parsing, checksum verification, upload, and the importer's handling of a hostile HTTP source.

This one Go module builds both `dvcr-importer` and `dvcr-uploader`, so it holds the targets of
two components. `task fuzz:*` runs them, and those are the tasks the external fuzzing platform
calls — though no fuzz image is published from this repository yet, so today they are run
locally through the container loop below.

Contents:

- [Targets and components](#targets-and-components)
- [Seed corpus convention](#seed-corpus-convention)
- [Running a target](#running-a-target)
- [The local loop: task docker:fuzz:*](#the-local-loop-task-dockerfuzz)
- [FuzzUploader and /dev/termination-log](#fuzzuploader-and-devtermination-log)
- [Where the corpus lives](#where-the-corpus-lives)
- [What fuzzing found](#what-fuzzing-found)
- [Known gaps](#known-gaps)

## Targets and components

Five targets. The seed counts below were read off the `FuzzX/seed#…` subtests of a plain
`go test -v` run, not counted from source lines.

| Target | Package | Seeds | Reached by | Covers |
| --- | --- | --- | --- | --- |
| `FuzzUploader` | `pkg/uploader` | 23 | dvcr-uploader | the `/upload` handler: request body and headers |
| `FuzzImageInfo` | `pkg/registry` | 55 | both | qcow2/vmdk/vdi/vhdx/vpc/iso/raw parsing through `qemu-img` and `file` |
| `FuzzImageInfoSnappy` | `pkg/registry` | 40 | dvcr-uploader | the same parsing behind the snappy framing of a blockdevice clone |
| `FuzzChecksums` | `pkg/registry` | 35 | both | checksum spec parsing and verification |
| `FuzzImporterHTTPSource` | `pkg/importer` | 47 | dvcr-importer | a hostile remote image source: status, headers, framing, body |

"Reached by" is the binary that actually links the code under test, read off the imports:
`pkg/uploader` goes into `dvcr-uploader` only, `pkg/importer` into `dvcr-importer` only, and
`pkg/registry` into both — `ParseChecksums` is called from `pkg/uploader/options.go` and from
`pkg/importer/importer.go`, and `getImageInfo` from the upload and the import path alike. The
snappy wrapping is `pkg/uploader/uploader.go` alone: `snappy` appears nowhere else in the
module, so `FuzzImageInfoSnappy` belongs to the uploader even though it lives in
`pkg/registry`.

### Where the requirement comes from

The project threat model assigns fuzzing of image formats to the image-upload path: qcow2,
vmdk, vdi, vhdx, vpc and iso parsing against malformed and malicious data, plus the upload
body of `dvcr-uploader`, where it names `FuzzUploader` as the target. That is what this module
fuzzes. QEMU and libvirt device emulation is not fuzzed here: both are consumed as prebuilt
container-factory packages (`pmPackages: [qemu]` in `images/qemu/werf.inc.yaml`), so there is
nothing in this repository to instrument.

Two notes about the document rather than about the code:

- Its components table names `dvcr-uploader` and `pvc-importer` in the fuzzing row but not
  `dvcr-importer`, while the same document describes `dvcr-importer` as the component that
  first receives and parses untrusted binary data. `FuzzImporterHTTPSource` stays either way:
  the input is untrusted, and the target already shares this module with the uploader.
- The same row also asks for the correctness of resource limiting. The limits it refers to
  (1 GiB of memory, 30 CPU seconds) are `maxMemory` and `maxCPUSecs` in
  [`images/pvc-artifact/pkg/image/qemu.go`](../pvc-artifact/pkg/image/qemu.go), not in this
  module — nothing here bounds `qemu-img`, it inherits the request context and dies with it.
  That half of the assignment is not satisfiable from this module.

## Seed corpus convention

**Every target carries at least 20 seeds, spelled out in the test file.** This is a
certification requirement, not a style preference. The lowest count today is 23.

- Seeds must be visible in `*_fuzz_test.go`. A table-driven corpus with a single `f.Add` in a
  `for` loop is fine — what matters is that the inputs are enumerated in the source, not
  generated. Both forms are in use in `pkg/registry/imageinfo_fuzz_test.go`: one `f.Add` per
  seed for the qcow2 and framing cases, and a loop over `imageFormatMagics` that gives every
  other disk format its bare magic, its padded header and a header whose fields all claim
  their maximum.
- Seeds should be *interesting*, not filler: format magic numbers, boundary values, truncated
  headers, overflowing lengths, path traversal, invalid UTF-8, and oversized inputs.
- Every target skips inputs above 1 MiB (`fuzzMaxInputSize`). A larger hostile image is not a
  more hostile one, and each iteration copies its payload through a temporary file and a child
  process. All seeds are well under the cap.
- **Non-ASCII and Cyrillic seeds are allowed.** `task validation:no-cyrillic` skips any file
  matching `_fuzz_test.go$` — the `skipFuzzTestRe` variable in
  [`tools/validation/no_cyrillic.go`](../../tools/validation/no_cyrillic.go) — and reports the
  skip reason as `fuzz seed corpus`. A seed corpus is input data, not prose. The exemption
  covers the test files only: this document, and any other `.md` outside `doc-ru-*`/`*.ru.md`,
  is still checked.
- Seed corpus entries run as ordinary subtests under `go test`, so a broken seed fails wherever
  the package is tested — but nothing tests this module in CI. CI runs unit tests for
  `images/virtualization-artifact` and the hooks only (`test:virtualization-controller` and
  `test:hooks` in `.gitlab/ci/jobs/test.yml`), and `lint:go` prunes `images/dvcr-artifact`
  outright (`.gitlab/ci/jobs/lint-validate.yml`). The seeds here are only checked when someone
  runs them locally.

## Running a target

All of `images/dvcr-artifact` is Linux-only: CDI's `pkg/util` uses `syscall.Fallocate` and
`unix.FALLOC_FL_PUNCH_HOLE` / `FALLOC_FL_KEEP_SIZE`, so the module does not build on macOS.

**Whether it needs cgo depends on `GOARCH`, and the shipped architecture needs it.** CDI's
`pkg/importer/vddk-datasource_amd64.go` is guarded by `//go:build amd64` and imports
`libguestfs.org/libnbd`, which is cgo-only. So on `GOARCH=amd64` — what this module actually
builds (`werf.inc.yaml`) — every fuzz package reaches it through
`pkg/registry` → `pkg/datasource` → CDI's `pkg/importer`, and `CGO_ENABLED=0` fails with
`build constraints exclude all Go files in .../libguestfs.org/libnbd`. On `GOARCH=arm64` that
one file is excluded, `libnbd` disappears from the dependency graph entirely, and
`CGO_ENABLED=0` succeeds.

The practical consequence: an unset `GOARCH` measures the host, so on an arm64 machine a
`CGO_ENABLED=0` build passes and looks like proof that cgo is unnecessary. It is not. Any image
that runs these targets needs `CGO_ENABLED=1` and `pkgconf`, `libnbd-devel` and `libxml2-devel`
present *in the image*, not merely in a build stage — the fuzzing platform overrides `GOCACHE`
per target, so `task fuzz:run` recompiles wherever it runs.

`fuzz.Dockerfile` and the `task docker:fuzz:*` tasks below run the targets in that
configuration on any host: `linux/amd64`, `CGO_ENABLED=1`, `libnbd` in the dependency graph of
`pkg/registry` (`go list -deps ./pkg/registry | grep libnbd`). Prefer them to anything
cross-compiled for `GOARCH=arm64`, which excludes the libnbd path altogether.

### task fuzz:* (the platform contract)

**No fuzz image is published from this repository today.** The external fuzzing platform finds
components by scanning werf build reports for images whose name contains `-fuzz`, so until such
an image exists the platform does not run these targets — the tasks below are the agreed
interface, and what actually runs them right now is the local container loop further down.
Adding the image was deliberately left out of the build: a test-only image must not be able to
break the module build, which is exactly what it did on its first CI run.


`Taskfile.dist.yaml` implements the four tasks the external fuzzing platform calls. They are
also the shortest way to run a target by hand. Everything is driven by environment variables,
because that is how the platform passes its parameters:

| Task | What it does | Variables |
| --- | --- | --- |
| `fuzz:list` | prints the target names of this image | `FUZZ_PACKAGES` (default `./...`) |
| `fuzz:run` | fuzzes one target, with mutation, until it is stopped | `FUZZ_PKG`, `FUZZ_TARGET`, `FUZZ_WORKERS` (default `1`) |
| `fuzz:coverage` | writes the coverage profile of a target's corpus | `FUZZ_PKG`, `FUZZ_TARGET`, `FUZZ_COVERAGE_FILE` |
| `fuzz:replay` | runs the seed corpus and saved crashers as ordinary tests | `FUZZ_PKG`, `FUZZ_TARGET` |

```bash
cd images/dvcr-artifact
task fuzz:list
FUZZ_PKG=./pkg/registry FUZZ_TARGET=FuzzChecksums task fuzz:replay
FUZZ_PKG=./pkg/registry FUZZ_TARGET=FuzzChecksums FUZZ_WORKERS=8 task fuzz:run
```

Notes that matter when reading or changing these tasks:

- **`fuzz:run` never ends on its own.** No `-fuzztime` is passed, by contract: the platform
  decides when a campaign stops (2 h without a new coverage path) and sends `SIGTERM`. Bound it
  yourself when running locally.
- **`FUZZ_WORKERS` becomes `-parallel`**, not a `GOFLAGS` injection.
- **`fuzz:list` is scoped by `FUZZ_PACKAGES`,** so a per-component fuzz image would list only
  its own targets: `./pkg/uploader/... ./pkg/registry/...` for `dvcr-uploader` and
  `./pkg/importer/... ./pkg/registry/...` for `dvcr-importer`. The scoping is by package,
  which is as fine-grained as `go test -list` gets, so `FuzzImageInfoSnappy` appears in both
  lists even though only the uploader reaches the snappy path. It costs one extra target run
  per campaign and exercises shared parsing code, so it is left alone.
- **Run the tasks from `images/dvcr-artifact`.** `Taskfile.dist.yaml` is one of the names
  go-task discovers on its own, so no `-t` flag is needed — but the root `Taskfile.yaml` does
  not include this module, so there is no `task dvcr:fuzz:list` from the repository root.

### Native (Linux host)

```bash
cd images/dvcr-artifact
go test -run '^$' -fuzz='^FuzzChecksums$' -fuzztime=60s ./pkg/registry/
```

### Container requirements

`pkg/registry/imageinfo.go` shells out to two binaries, and both must be present or the run is
worthless rather than merely noisy:

| Binary | Package | What breaks without it |
| --- | --- | --- |
| `qemu-img` | `qemu-utils` | `getImageInfo` fails on *every* input (`error running qemu-img info: exec: "qemu-img": executable file not found`), and the image parsing is never reached. |
| `file` | `file` | The ISO branch of `getImageInfoStandard` is unreachable — `qemu-img` reports an ISO as `raw`, and the ISO decision comes from `file -b`. `TestGetImageInfoRawVirtualSize` fails too. |

A missing binary used to be invisible: `FuzzImageInfo` treats a failed `getImageInfo` as a
rejected image, so every input returned early and the run reported `PASS` at thousands of
execs/s over nothing. `requireImageTools` now looks both binaries up once per target and fails
it outright, so a misconfigured image cannot report a green run:

```text
--- FAIL: FuzzImageInfo (0.00s)
    imageinfo_fuzz_test.go:84: qemu-img is required to reach the image parsing path: exec: "qemu-img": executable file not found in $PATH
```

Verified by running the same binary in `debian:bookworm-slim` with neither tool, with
`qemu-utils` only, and with both.

### Flag pitfalls

| Symptom | Cause | Fix |
| --- | --- | --- |
| `will not fuzz, -fuzz matches more than one fuzz test: [FuzzImageInfo FuzzImageInfoSnappy]` | `-test.fuzz` is an unanchored regexp | anchor it: `-test.fuzz='^FuzzImageInfo$'` |
| the package's unit tests and every other target's seed corpus run before fuzzing starts | `-test.run` defaults to "everything" | pass `-test.run '^$'` |
| `warning: the test binary was not built with coverage instrumentation, so fuzzing will run without coverage guidance and may be inefficient` | a binary prebuilt with `go test -c` carries no libFuzzer instrumentation | let `go test -fuzz` build it, as `task fuzz:run` does |

The last row is about prebuilt binaries only. `go test -fuzz` instruments the target itself, so
`task fuzz:run` always has coverage guidance and prints `gathering baseline coverage: N/N`;
measured through `task docker:fuzz:run`, `FuzzChecksums` reported `new interesting: 16` in 45 s.
`FuzzImageInfo` is the exception, and not because of instrumentation: it spawns `qemu-img` per
exec and stalls at a few hundred execs (779 in 45 s on a native arm64 run, 59 under emulation),
with `new interesting` never moving off zero.

## The local loop: task docker:fuzz:*

`fuzz.Dockerfile` builds a `linux/amd64` image holding the toolchain, `libnbd-dev` +
`pkg-config` for the cgo build, and the two binaries the targets shell out to. Three tasks
drive it; they wrap the platform's `fuzz:*` tasks and never replace them.

```bash
cd images/dvcr-artifact
task docker:fuzz:build                                  # ~35 s
task docker:fuzz:list                                   # all five targets
task docker:fuzz:seeds                                  # every seed corpus
FUZZ_PKG=./pkg/registry FUZZ_TARGET=FuzzChecksums task docker:fuzz:seeds
FUZZ_PKG=./pkg/registry FUZZ_TARGET=FuzzChecksums FUZZ_DOCKER_TIME=5m task docker:fuzz:run
```

`FUZZ_PKG`, `FUZZ_TARGET`, `FUZZ_WORKERS`, `FUZZ_PACKAGES` and `FUZZ_COVERAGE_FILE` pass
straight through to the container. `FUZZ_DOCKER_TIME` (default `60s`) exists only in the
wrappers: `fuzz:run` deliberately has no `-fuzztime`, so `docker:fuzz:run` supplies one through
`GOFLAGS`. Do not bound the run by killing the container instead — go-task discards the output
of a command it had to terminate, and the whole progress log goes with it.

**It runs under emulation on an arm64 host, and that is the dominant cost.** Measured against
the same code in a native `linux/arm64` container: the `pkg/registry` seed replay takes 4.5 s
against 0.38 s (~12x), and `FuzzImageInfo` manages 59 execs in 45 s against 779 (~13x) — both
bound by spawning `qemu-img`. Pure-Go paths pay much less: `FuzzChecksums` did 32 536 execs in
20 s against 50 903 native (~1.6x). Use the emulated image to confirm behaviour in the shipped
configuration, not for long soaks.

### Why the module cache is mounted

`images/dvcr-artifact/go.mod` has no `replace` directives of its own: every replacement,
including `kubevirt.io/api` pointing at a fork on an internal GitLab host, lives in the
repository-root `go.work`. A container with an empty module cache therefore cannot resolve the
dependency graph at all.

```text
# GOTOOLCHAIN=local, base image older than the go.work directive
go: /src/go.work requires go >= 1.25.12 (running go 1.25.11; GOTOOLCHAIN=local)

# GOTOOLCHAIN=auto, no credentials for the fork host
kubevirt.io/api@v1.3.1: no secure protocol found for repository
```

So the image contains no sources and no cache, and `_docker:fuzz:exec` mounts three things:
the repository at `/src` (the tests read `testdata/` relative to the working directory, and
`go.work` must be visible above the module), the host's `GOMODCACHE` at `/go/pkg/mod`, and a
named volume for `GOCACHE` so the next run does not recompile the Kubernetes stack — which
under emulation is the difference between nine minutes and twenty-five seconds. `GOPROXY=off`
in the image turns a cache miss into an immediate error instead of a doomed fetch.

The alternative, baking a cache into the image at build time, was rejected: it needs the host
cache inside the build context, and it dates the moment `go.mod` changes. Mounting keeps the
image independent of the module graph at the cost of not being self-contained — it cannot run
on a machine that has never built this module. That is the right trade for a developer loop;
the platform's own fuzz image is where a self-contained build belongs.

Two consequences of the repository mount. The container runs as root, so anything it writes
into the tree is root-owned — check `git status` after a run, and delete stray
`testdata/fuzz/<FuzzFunc>/` reproducers unless they are real findings. And `go clean
-fuzzcache` inside the container clears the named volume, not the host cache.

## FuzzUploader and /dev/termination-log

`pkg/uploader/uploader.go` reports the result of every upload through the pod termination
message: `WriteImportCompleteMessage` on success and `WriteImportFailureMessage` on failure.
Both end up in `pkg/monitoring/termination_message.go` → CDI's `util.WriteTerminationMessage`,
which writes `common.PodTerminationMessageFile` — a hard-coded `/dev/termination-log`.

- **In Docker as root it just works.** `/dev` is a writable `tmpfs` owned by root, so the file
  is created. `task docker:fuzz:*` runs as root for exactly this reason. A 23-seed run of
  `FuzzUploader` writes the message 46 times — 6 through the success path
  (`Image is saved in DVCR`) and 40 through the failure path
  (`Failed to save image to DVCR:`), which `WriteImportFailureMessage` emits only after the
  write itself returns without error.
- **As an unprivileged user it does not.** `/dev` is mode 0755 root-owned, so every write
  fails with `open /dev/termination-log: permission denied`. In a 23-seed run as `--user
  1000:1000` this fired 46 times.
- **The test still passes,** which is the trap. On the success path the error propagates to
  `processUpload`, which answers `500`, and `pkg/fuzz/http.go` only reports
  `resp.StatusCode > 500`. On the failure path the error is merely logged. So an unprivileged
  run silently degrades to exercising nothing but error handling.

Run `FuzzUploader` as root in a container, or in a pod where the kubelet has provisioned
`/dev/termination-log`. If you must run it unprivileged on a host, expect the success path to
be dead.

Two logging traps around this code, neither of them a write failure:
`WriteImportFailureMessage` returns the *original* import error even when the write succeeded,
so `uploader.go:334` prints `Failed to write the termination message: <the upload error>` on
every failed upload. And `error running qemu-img info: : signal: killed` with empty `qemu-img`
output is the pipeline's own `context.WithCancel` (`pkg/registry/registry.go:138`) tearing the
child down after some *other* step failed — not a timeout and not a missing binary.

## Where the corpus lives

Two different directories, and only one of them is under Go's control in the way people
assume.

| Location | Contents | Reaches git? |
| --- | --- | --- |
| `$(go env GOCACHE)/fuzz/<package import path>/<FuzzFunc>/` | every coverage-expanding input the fuzzer finds | no |
| `<package>/testdata/fuzz/<FuzzFunc>/` | crash reproducers only, written when a target fails | yes, it is not gitignored |

Consequences:

- **The interesting corpus is not versioned.** Go writes discovered inputs only to the build
  cache; `go clean -fuzzcache` erases them, and nothing carries them into `testdata/fuzz`.
  If a corpus is worth keeping, copy it out deliberately.
- **`-test.fuzzcachedir` decides where that cache goes** for a prebuilt binary. Point it at a
  mounted path if you want the corpus to survive the container.
- **A stale cache makes seed counts unreadable.** With coverage guidance the run prints
  `gathering baseline coverage: N/N` over seeds *plus* cached inputs, so `N` drifts above the
  seed count. To count seeds, run the target as a plain test (`go test -run '^FuzzX$' -v`,
  which counts `FuzzX/seed#…` subtests) or `go clean -fuzzcache` first.
- **A container run with the repo mounted can write into the repo.** If a target crashes, the
  reproducer lands in the mounted `testdata/fuzz/<FuzzFunc>/` as root. Delete it unless it is
  a real finding worth committing as a regression case, and check `git status` after any
  container run.
- `.gitignore` covers `images/dvcr-artifact/fuzzartifact/` only.

## What fuzzing found

The defects are real regardless of scope, so they are kept on record. Three of the four were
found by targets that have since been removed as out of scope and are no longer reproducible
from this repository.

| Defect | Location |
| --- | --- |
| Unbounded memory in `generateValidGrid`: `sizingPolicies[].memory` has no numeric bounds in the CRD (only a quantity `pattern`), while `cores` has `minimum: 1`, `maximum: 1024` and CEL rules for `max > min` / `max > step`. A wide `min`/`max` with a tiny `step` allocates one `resource.Quantity` (56 bytes) per step, for a single admission call. The fuzz run that found it reported ~3.4 GB; an isolated reproduction with `min=0`, `max=32Mi`, `step=1` built 33.5M elements and 5 GB of heap. | `images/virtualization-artifact/pkg/controller/service/size_policy_service.go:335` |
| Non-terminating loop in the same function: the quantity `pattern` accepts a leading `-`, and the loop condition is `val.Cmp(max) == CmpLesser` with `val.Add(step)`. A negative `step` walks away from `max` forever, appending as it goes. | `images/virtualization-artifact/pkg/controller/service/size_policy_service.go:335` |
| Unbounded HTTP redirects in the CDI fork's http-datasource: `client.CheckRedirect` is set to a function that always returns `nil`, which replaces `net/http`'s default 10-hop cap with no cap at all. An attacker-controlled `http.url` can chain redirects indefinitely. | `github.com/deckhouse/3p-containerized-data-importer` → `pkg/importer/http-datasource.go:326` |
| Panic on a short device name: `device.DeviceName[4:]` slices unconditionally, so any `DeviceName` shorter than 4 bytes panics in the DRA driver. | `images/virtualization-dra/internal/cdi/cdi.go:115` |

## Known gaps

Open items in the targets themselves, kept here so nobody has to rediscover them.

- **`FuzzUploader`'s DVCR mock used to make the success path unreachable. Fixed — keep it that
  way.** The mock is easy to break silently, because a broken one still reports `PASS`: the
  target degrades to exercising error handling only, and nothing says so. Two defects had to go:
  every pattern in `startDVCRMockServer` except the `POST` ended in a slash after its wildcard
  (`PATCH /v2/uploader/blobs/uploads/{id}/`) while the requests go to
  `/v2/uploader/blobs/uploads/test_data` without one, and Go's `ServeMux` reads a trailing slash
  as a subtree prefix, so nothing matched; and the `PUT` that closes a blob upload answered
  `200 OK` where a registry client requires `201 Created`. Together they failed all 46 uploads
  of a 23-seed run, first with `stream was already consumed` — `go-containerregistry` retrying
  a stream it cannot rewind — then with `unexpected status code 200 OK`.
  A 23-seed run must log `Image is saved in DVCR` **6 times**. That number is the check: if it
  drops to zero, the mock stopped routing, not the uploader stopped working. The remaining
  failures in such a run are the seeds that are meant to fail, mostly
  `could not process image header`.
- **`FuzzUploader` asserts very little.** Its only invariant comes from `pkg/fuzz/http.go`,
  which fails the iteration when the status code is above `500`. A `500` is therefore an
  accepted answer to any input, and the upload is not checked for what it wrote to DVCR.
- **`FuzzImporterHTTPSource` reports the redirect defect with `t.Logf`, not `t.Fatalf`.** The
  unbounded redirect chain below is a known defect of the CDI fork, so failing on it would
  leave the target permanently red. It stays a log line until the fork is fixed.
- **The shared uploader server is not per-iteration state.** `FuzzUploader` starts one
  uploader and one DVCR mock for the whole target: cheap, but the two are the only state that
  survives between iterations. `keepAlive` keeps the server up through a failed upload
  (`processUpload` in `pkg/uploader/uploader.go`); without it, the first non-permanent upload
  error shut the listener down and every later iteration silently talked to a closed port.
- **Nothing runs these targets in CI.** See the seed corpus convention above: the module is
  outside both the test and the lint jobs, and no fuzz image is published either. The seeds are
  checked only by whoever runs them locally, through the container loop above.
