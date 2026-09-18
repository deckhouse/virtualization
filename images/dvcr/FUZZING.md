# Fuzzing the DVCR registry

DVCR is the container registry of the module. Its binary is built from the
[distribution fork](https://fox.flant.com/deckhouse/virtualization/fork/distribution), pinned in
`build/components/versions.yml` and cloned by `dvcr-src-artifact`, so the code under analysis
lives in that repository rather than in this one.

The `dvcr-fuzz` image exists to hand that code to an external laboratory: it is the deliverable,
not the campaign. It has to be self-contained — the fork's sources, the sources of every
dependency and the fuzz targets, all in one image — so the laboratory can analyse and fuzz it
without reaching back into our repositories.

`dvcr-artifact` has its own image built the same way; see
[its FUZZING.md](../dvcr-artifact/FUZZING.md) for the parts shared by both.

## What the image carries

| What | Where in the image | How it gets there |
| --- | --- | --- |
| the fork's sources | `/distribution` | `dvcr-src-artifact` clones the tag into `/src/distribution`, `dvcr-builder` imports it to `/distribution` |
| every dependency's sources | `/distribution/vendor` | vendored in the fork itself |
| the fuzz targets | next to the code they exercise | the fork's own `_test.go` files, plus `images/dvcr/fuzz/policy_fuzz_test.go` that werf places into `registry/auth/dvcrk8s/` (`FuzzTargetFiles`) |
| the task runner contract | `/distribution/Taskfile.yml` | `Taskfile.fuzz.yml` mounted by the template |

Unlike `dvcr-artifact`, this module is vendored, so the install stage skips `go mod download`:
`go test` defaults to `-mod=vendor` here and the module cache would be a second, unread copy of
the trees already in `vendor/`. The dependency sources the laboratory needs are in `vendor/`.

The clone drops `.git`, so the image carries a working tree rather than a history; the tag it
came from is the one in `build/components/versions.yml` at the commit that built the image.

## Targets in the image

Six targets: four come from upstream distribution, two are written in this repository. A Go fuzz
target has to live inside the package it tests, so `images/dvcr/fuzz/policy_fuzz_test.go` is kept
here and copied into the fork's `registry/auth/dvcrk8s/` when the image is built. Renaming the code
under test in the fork breaks this image's build — louder than a target that quietly stops covering
anything.

| Target | Package | Surface |
| --- | --- | --- |
| `FuzzConfigurationParse` | `configuration` | parsing the registry's own YAML configuration |
| `FuzzParseForwardedHeader` | `registry/api/v2` | the `Forwarded` HTTP header of a proxied request |
| `FuzzToken1`, `FuzzToken2` | `registry/auth/token` | bearer tokens presented to the registry |
| `FuzzAuthorize`, `FuzzClassify` | `registry/auth/dvcrk8s` | the access decision of the `dvcr-k8s` authorization plugin: Basic credentials, the `access` claim of a signed token, the requested repository name |

## The -fuzz image

`werf.inc.yaml` ends with `{{- include "fuzz image" . }}`, which
[`.werf/defines/fuzz.tmpl`](../../.werf/defines/fuzz.tmpl) turns into
`{ModuleNamePrefix}dvcr-fuzz`. The platform finds components by scanning werf build reports for
images whose name contains `-fuzz`.

Two things keep a test-only image out of the way of the module build: it is `final: false`, so it
never reaches the module bundle, and the whole template is behind `WERF_BUILD_FUZZ_IMAGES=true`,
which only `build_fuzz_dev` and `build_fuzz_release` set. An ordinary `werf build` produces no
fuzz image.

The image is built from `dvcr-builder` with `CGO_ENABLED=0`, matching the registry binary, and
imports no qemu tools — only `dvcr-artifact-fuzz` needs those. `dvcr-builder` stands on
`builder/golang-debian-1.25` (svace builds swap it for `builder/golang-alt-1.25`), which ships
`git` but not `curl`. The install stage adds whatever is missing of `git`, `curl`, `python3`,
`ca-certificates`, `jq`, `unzip` and `file` through the package manager the base ships (`pm`,
`apk` or `apt-get`, probed in that order), then the AWS CLI from amazonaws and go-task via
`go install`. MinIO Client is not used.

## How the laboratory gets it

`build_fuzz_dev` (`.gitlab/ci/jobs/build-fuzz.yml`) is manual on MR pipelines. It keeps
`WERF_REPO` pointed at `${MODULES_MODULE_SOURCE}/${MODULES_MODULE_NAME}`, which on the DEV
registry is `${DEV_MODULE_SOURCE}/virtualization` (see `.gitlab/ci/variables.yml`), and the image
is named `dvcr-fuzz` — `ModuleNamePrefix` is empty outside embedded-module mode. werf tags it by
content, so the exact tag comes from the build report of the job that produced it.

## Running a target by hand

Inside the image `Taskfile.fuzz.yml` is mounted as `Taskfile.yml` and implements the contract the
platform calls, which is also the shortest way to run a target manually:

```bash
task fuzz:list
FUZZ_PKG=./configuration FUZZ_TARGET=FuzzConfigurationParse task fuzz:replay
FUZZ_PKG=./configuration FUZZ_TARGET=FuzzConfigurationParse FUZZ_WORKERS=8 task fuzz:run
FUZZ_PKG=./registry/auth/dvcrk8s FUZZ_TARGET=FuzzAuthorize task fuzz:replay
```

`fuzz:run` keeps mutating until it is stopped; `fuzz:replay` runs the seed corpus and any saved
crashers once, as ordinary tests.
