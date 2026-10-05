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
| the task runner contract | `/src/Taskfile.yml` | `Taskfile.fuzz.yml` copied by `werf.inc.yaml` |

The fork of CDI is in there by the same route as any other dependency: `go.mod` replaces
`kubevirt.io/containerized-data-importer` with
`github.com/deckhouse/3p-containerized-data-importer`, so `go mod download` writes the fork's
sources into `/fuzz/gomod`.

Note that the module root is `/src` itself on this branch, not `/src/images/dvcr-artifact` as on
`main`: the src-artifact here adds `images/dvcr-artifact` rather than the whole repository, so
there is no repository-shaped tree above the module. The working directory passed to the
`fuzz image` template in `werf.inc.yaml` follows that.

## How the laboratory gets it

`build_fuzz` publishes to `WERF_REPO=${MODULES_MODULE_SOURCE}/${MODULES_MODULE_NAME}`, which on
the DEV registry is `${DEV_MODULE_SOURCE}/virtualization` (see `.gitlab/ci/variables.yml`), and
the image is named `dvcr-artifact-fuzz` — `ModuleNamePrefix` is empty outside embedded-module
mode. werf tags it by content, so the exact tag comes from the build report the job saves as an
artifact and, after a successful replay on `release-1.10`, publishes for the platform.

## The -fuzz image

`werf.inc.yaml` ends with `{{- include "fuzz image" (list . "/src") }}`, which
[`.werf/defines/fuzz-image.tmpl`](../../.werf/defines/fuzz-image.tmpl) turns into
`{ModuleNamePrefix}dvcr-artifact-fuzz`. The platform finds components by scanning werf build
reports for images whose name ends in `-fuzz`. The template only carries what every fuzz image
shares: the pinned `ci-images/fuzz-go` base from
[`ci_images.yml`](../../build/base-images/ci_images.yml), the working directory and the
`io.deckhouse.fuzz.engine` label. Source imports, git files, environment variables and install
commands stay in this module's `werf.inc.yaml`.

The image is `final: false`, so it never reaches the module bundle, and the ordinary build jobs
run werf with final images only, so they never build it. There is no `WERF_BUILD_FUZZ_IMAGES`
switch any more: the shared `build_fuzz` job builds every image whose name ends in `-fuzz`.

The image is built with `CGO_ENABLED=1` — the targets reach CDI's `pkg/importer`, which on
`GOARCH=amd64` pulls in the cgo-only `libguestfs.org/libnbd`, so a `CGO_ENABLED=0` build fails
with `build constraints exclude all Go files`. `fuzz-go` ships only the common fuzzing tools, so
the install stage fetches the native compiler, headers, `libnbd`, `libxml2` and `file` from
container-factory with `pm`, imports `/qemu-img` from the `qemu` image and downloads the Go
modules into the image layer (`GOMODCACHE=/fuzz/gomod`). Nothing about the corpus happens at
build time: restore and replay moved to the CI child pipeline.

## The build and replay jobs

[`build-fuzz.yml`](../../.gitlab/ci/jobs/build-fuzz.yml) uses the shared
`Build_Fuzz.gitlab-ci.yml` and `Replay_Fuzz.gitlab-ci.yml` templates from `modules-gitlab-ci`
`v15.0`:

1. `build_fuzz` builds the fuzz images, saves their build report and generates the replay
   child pipeline. It runs automatically in MR pipelines and on every push to `release-1.10`.
2. `replay_fuzz` starts that pipeline. Each replay job runs its built image, restores the S3
   corpus into the targets' `testdata/fuzz/<FuzzFunc>/` directories, discovers targets with
   `task fuzz:list` and checks the corpus as ordinary Go tests. A failing target or an empty
   target list fails the replay job. The trigger is manual and non-blocking in MRs; on a push
   to `release-1.10` it runs automatically and publishes the image build report for the
   platform after a successful replay.
3. The replay job saves `fuzz-replay/summary.txt`, the full `fuzz-replay/job.log` and failure
   artifacts. The console shows only a short summary or the tail of a failed run's log, so
   verbose `qemu-img` output cannot push the job past the GitLab log size limit.

The corpus is keyed by branch: both runs read `anomaloys-materials/virtualization/release-1-10/`,
never `main` — the shared template would default an MR to `main`, whose entries for
`pvc-artifact` never match because the module is named differently on this branch, so
`build-fuzz.yml` pins the slug. The pipeline never writes the corpus; the external fuzzing
platform does, once it has picked the images up from the published report. Until then the
replay checks only the seeds committed in the repository.

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
