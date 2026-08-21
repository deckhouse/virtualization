# Local fuzz-run environment for images/pvc-artifact.
#
# Why this file exists: the module is Linux-only (syscall.Fallocate, SYS_PRLIMIT64), so
# nothing here builds or runs on a macOS host, and the architecture that ships is amd64.
# This image is the committed replacement for hand-rolled cross-compile recipes.
#
# ARCHITECTURE: linux/amd64 only. The `fuzz:local:*` tasks in Taskfile.yaml pass
# `--platform=linux/amd64` to both build and run. On an arm64 host every process in here
# runs under QEMU emulation and is several times slower than native; the seed corpus still
# completes in seconds, but throughput figures from a fuzzing run are not comparable with
# a native amd64 machine.
#
# Contents: the Go toolchain the workspace requires, plus go-task. Nothing else.
# The seven fuzz targets need no external binary: `FuzzQemuInfo` replaces the exec seam
# (`qemuExecFunction`) with a stub and the other six work on bytes and strings only, so
# qemu-img, nbdkit, nbdcopy and file(1) are deliberately absent. Installing them would
# only hide a target that started shelling out for real.
#
# DEPENDENCIES ARE MOUNTED, NOT BAKED IN. This module does not resolve on its own:
# `github.com/docker/docker` is replaced by the repository's go.work with the local stub in
# images/pvc-artifact/staging, so go.sum has no hashes for the upstream module and
# `GOWORK=off go build ./...` fails with "missing go.sum entry for module providing package
# github.com/docker/docker/api/types/versions". With the workspace active the build list is
# the union of every module in go.work, which includes `kubevirt.io/api` pointing at a fork
# on an internal GitLab host ("no secure protocol found for repository" from a container
# with no credentials).
#
# Three ways out were considered:
#   * mount the host module cache at run time  <- chosen
#   * copy the host module cache into the image at build time
#   * vendor the whole workspace (`go work vendor`)
# The host cache is 9 GB, so baking it in produces an unusable image and requires the build
# context to be the cache directory; vendoring puts a huge tree in the repository and has to
# be regenerated on every dependency change. Mounting costs nothing and stays correct by
# construction. The price is a precondition: the host must have populated its module cache
# once (any successful `go build` in this repository does it). GOPROXY=off below turns a
# cold cache into an immediate, readable error instead of a hang on an unreachable host.
FROM golang:1.25.12-bookworm

# go-task: the fuzz:* tasks are written for it, and the external fuzzing platform calls them.
ARG TASK_VERSION=v3.53.1
RUN go install github.com/go-task/task/v3/cmd/task@${TASK_VERSION} \
    && go clean -cache -modcache

# GOTOOLCHAIN=local is already the default here and is kept on purpose: the base image tag
# matches the `go` directive of go.work exactly, so a toolchain download is never needed and
# an accidental version bump should fail loudly rather than fetch a toolchain.
ENV GOTOOLCHAIN=local \
    GOPROXY=off \
    GOMODCACHE=/gomodcache \
    GOCACHE=/gocache

# GOCACHE also holds the fuzzing corpus ($GOCACHE/fuzz), which is why the tasks give it a
# named volume: without one, everything the fuzzer discovers dies with the container.
RUN mkdir -p /gocache /gomodcache

WORKDIR /src/images/pvc-artifact
