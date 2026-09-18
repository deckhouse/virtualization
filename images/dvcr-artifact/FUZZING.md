# Uploader Package Fuzzing Tests

Fuzzing tests for the uploader package's HTTP parsing and validation functions using Go's native fuzzing framework.

## Quick Reference

| Command                                            | Description                                |
| -------------------------------------------------- | ------------------------------------------ |
| `./docker-fuzz.sh`                                 | Run all fuzz tests in Docker (recommended) |
| `./docker-fuzz.sh -t 5m`                           | Run all tests for 5 minutes                |
| `cd pkg/uploader && go test -fuzz=. -fuzztime=30s` | Direct local testing                       |
| `./fuzz.sh`                                        | Direct local testing                       |

**🐳 Docker Required**: All testing should be done in Docker containers for isolation and reproducibility.

## Quick Start

### Docker-based Fuzzing (Recommended)

```bash
# Build and run all fuzzing tests
./docker-fuzz.sh

# Run with custom duration
./docker-fuzz.sh -t 5m

# Direct build and run (single command)
docker run --rm --platform linux/amd64 $(docker build --platform linux/amd64 -q -f fuzz.Dockerfile .)
```

## Direct Docker Commands

For immediate testing without scripts, you can build and run in a single command:

```bash
# Build and run immediately (recommended for quick testing)
docker run --rm --platform linux/amd64 $(docker build --platform linux/amd64 -q -f fuzz.Dockerfile .)

# With custom duration
docker run --rm --platform linux/amd64 -e $(docker build --platform linux/amd64 -q -f fuzz.Dockerfile .)
```

## Local Commands

For running locally, you can run the following commands:

```bash
./fuzz.sh
```

# Tests

### FuzzUploader

This test is used to fuzz the uploader package's HTTP parsing and validation functions using Go's native fuzzing framework.
Test start uploader server and DVCR mock server before running the test. The test will send a request to the uploader server with the fuzzed data.
After the request is sent, the test will check the response status code and the response body to ensure the request was successful. Then data will be sent to the DVCR mock server.

#### Example

```bash
$ ./fuzz.sh FuzzUploader
=== RUN   TestFuzzUploader
--- PASS: TestFuzzUploader (0.00s)
    fuzz_test.go:102: Fuzzing ProcessRequests in /virtualization/images/dvcr-artifact/pkg/uploader/uploader_fuzz_test.go
    fuzz_test.go:102: Fuzzing ProcessRequest in /virtualization/images/dvcr-artifact/pkg/uploader/uploader_fuzz_test.go
    fuzz_test.go:102: Fuzzing ProcessRequests in /virtualization/images/dvcr-artifact/pkg/uploader/uploader_fuzz_test.go
    fuzz_test.go:102: Fuzzing ProcessRequest in /virtualization/images/dvcr-artifact/pkg/uploader/uploader_fuzz_test.go
```

# The fuzz image for external analysis

Besides the local Docker path above, this module is built into a werf image handed to an
external laboratory. The image is the deliverable, not the campaign: it has to be
self-contained, carrying the source code under analysis — this module, the sources vendored in
the repository, and the sources of every dependency including the forks — alongside the fuzz
targets, so the laboratory can analyse and fuzz it without reaching back into our repositories.

## What the image carries

| What | Where in the image | How it gets there |
| --- | --- | --- |
| this module's sources | `/src` | `dvcr-artifact-src-artifact` adds `images/dvcr-artifact` to `/src`, the builder inherits it |
| the vendored Docker sources | `/src/staging/src/github.com/docker/docker` | part of the same tree; `go.mod` points the `github.com/docker/docker` replace at it |
| every dependency's sources, forks included | `/fuzz/gomod` | `go mod download` in the install stage, with `GOMODCACHE=/fuzz/gomod` so the modules land in the image layer instead of the mounted `/go/pkg` |
| the fuzz targets | next to the code they exercise | ordinary `_test.go` files of the module |
| the task runner contract | `/src/Taskfile.yml` | `Taskfile.fuzz.yml` mounted by the template |

The fork of CDI is in there by the same route as any other dependency: `go.mod` replaces
`kubevirt.io/containerized-data-importer` with
`github.com/deckhouse/3p-containerized-data-importer`, so `go mod download` writes the fork's
sources into `/fuzz/gomod`.

Note that the module root is `/src` itself on this branch, not `/src/images/dvcr-artifact` as on
`main`: the src-artifact here adds `images/dvcr-artifact` rather than the whole repository, so
there is no repository-shaped tree above the module. `FuzzWorkDir` in `werf.inc.yaml` follows
that.

## How the laboratory gets it

`build_fuzz_dev` keeps `WERF_REPO` pointed at `${MODULES_MODULE_SOURCE}/${MODULES_MODULE_NAME}`,
which on the DEV registry is `${DEV_MODULE_SOURCE}/virtualization` (see
`.gitlab/ci/variables.yml`), and the image is named `dvcr-artifact-fuzz` — `ModuleNamePrefix` is
empty outside embedded-module mode. werf tags it by content, so the exact tag comes from the
build report of the job that produced it.

## The -fuzz image

`werf.inc.yaml` ends with `{{- include "fuzz image" . }}`, which
[`.werf/defines/fuzz.tmpl`](../../.werf/defines/fuzz.tmpl) turns into
`{ModuleNamePrefix}dvcr-artifact-fuzz`. The platform finds components by scanning werf build
reports for images whose name contains `-fuzz`.

Two things keep a test-only image out of the way of the module build: it is `final: false`, so
it never reaches the module bundle, and the whole template is behind `WERF_BUILD_FUZZ_IMAGES=true`,
which only `build_fuzz_dev` and `build_fuzz_release` set. An ordinary `werf build` produces no
fuzz image.

The image is built from `dvcr-artifact-builder` with `CGO_ENABLED=1` — the targets reach CDI's
`pkg/importer`, which on `GOARCH=amd64` pulls in the cgo-only `libguestfs.org/libnbd`, so a
`CGO_ENABLED=0` build fails with `build constraints exclude all Go files`. The builder already
carries `libnbd`, `libxml2` and the toolchain, so the fuzz image adds only what the corpus sync
and the task runner need. The install stage adds whatever is missing of `git`, `curl`, `python3`,
`ca-certificates`, `jq`, `unzip` and `file` through the package manager the base ships (`pm`,
`apk` or `apt-get`, probed in that order), then the AWS CLI from amazonaws and go-task via
`go install`. MinIO Client is not used. `git` and `curl` already ship in `builder/golang-alt-1.25`.

Its install stage then downloads the modules into the image layer (`GOMODCACHE=/fuzz/gomod`, not
the mounted `/go/pkg`, which is outside the image) and mirrors the corpus overlay from
`s3://anomaloys-materials/<repository>/<branch slug>/` into each target's
`testdata/fuzz/<FuzzFunc>/`.

## The build job

`build_fuzz_dev` (`.gitlab/ci/jobs/build-fuzz.yml`) is manual on MR pipelines: the image sits on
top of the builder/qemu chain and the install stage replays the whole corpus, which costs 15-45
minutes — too much for every pipeline when nothing fuzz-related changed.

`build_fuzz_release` is the automatic job: the counterpart of `build_fuzz_main` on `main`, which
extends `.main` and would never fire here. It extends `.release` instead and runs on every push to
`release-1.10`, with the upstream `.build_fuzz_main` body: `FUZZ_S3_BRANCH_SLUG` comes from
`CI_COMMIT_REF_SLUG`, so the corpus is read from `anomaloys-materials/virtualization/release-1-10/`
and the build report goes to `build-reports/virtualization/release-1-10/`. The pipeline never writes
the corpus — the external fuzzing platform does, once it has picked the images up from the report.
Until then the replay checks only the seeds committed in the repository. `build_fuzz_dev` on an MR
reads the corpus of the MR target branch (its slug is derived the way GitLab builds
`CI_COMMIT_REF_SLUG`, so an MR into `release-1.10` reads `release-1-10`) and publishes nothing. The
upstream body would read `main`, whose entries for `pvc-artifact` never match: the module is named
differently on this branch.

## Targets in the image

| Target | Package | Surface |
| --- | --- | --- |
| `FuzzUploader` | `pkg/uploader` | the uploader's HTTP request parsing and validation |
| `FuzzImageInfo` | `pkg/registry` | qcow2/vmdk/vdi/vhdx/vpc/iso/raw parsing through `qemu-img` and `file` |
| `FuzzImageInfoSnappy` | `pkg/registry` | the same parsing behind the snappy framing of a blockdevice clone |
| `FuzzImporterHTTPSource` | `pkg/importer` | a hostile HTTP image source: status, headers, redirects, lengths, encodings |

The one target `main` additionally carries, `FuzzChecksums`, is not here: the multi-algorithm
checksum path it tests (`pkg/registry/checksum.go`) does not exist on this branch, whose import
handler accepts a sha256 and an md5 sum only. `FuzzImageInfo` and `FuzzImageInfoSnappy` are the
files of `main` with one line adapted to this branch's `NewDataProcessor` signature.

## Running a target by hand

Inside the image `Taskfile.fuzz.yml` is mounted as `Taskfile.yml` and implements the contract
the platform calls, which is also the shortest way to run a target manually:

| Task | What it does | Variables |
| --- | --- | --- |
| `fuzz:list` | prints the target names of this image | — |
| `fuzz:run` | fuzzes one target, with mutation, until it is stopped | `FUZZ_PKG`, `FUZZ_TARGET`, `FUZZ_WORKERS` |
| `fuzz:coverage` | writes the coverage profile of a target's corpus | `FUZZ_PKG`, `FUZZ_TARGET`, `FUZZ_COVERAGE_FILE` |
| `fuzz:replay` | runs the seed corpus and saved crashers as ordinary tests | `FUZZ_PKG`, `FUZZ_TARGET` |

```bash
FUZZ_PKG=./pkg/importer FUZZ_TARGET=FuzzImporterHTTPSource task fuzz:replay
FUZZ_PKG=./pkg/importer FUZZ_TARGET=FuzzImporterHTTPSource FUZZ_WORKERS=8 task fuzz:run
```

`fuzz:run` never ends on its own: no `-fuzztime` is passed, by contract the platform decides when
a campaign stops and sends `SIGTERM`. Bound it yourself when running it locally.
