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
# Contents: the Go toolchain, plus go-task. Nothing else. The targets need no external
# binary: `FuzzQemuInfo` replaces the exec seam (`qemuExecFunction`) with a stub,
# `FuzzPrometheusEndpoint` talks to a listener it opens itself, and the rest work on bytes
# and strings only, so qemu-img, nbdkit, nbdcopy and file(1) are deliberately absent.
# Installing them would only hide a target that started shelling out for real.
#
# DEPENDENCIES ARE MOUNTED, NOT BAKED IN. On this branch the module resolves on its own:
# there is no go.work, and the `github.com/docker/docker` replace pointing at the stub in
# staging/ lives in go.mod. The mount is still the cheap way to get the modules in: the
# host has them already (any successful `go build` in this repository fills the cache),
# nothing has to be fetched through the container, and GOPROXY=off turns a cold cache into
# an immediate, readable error instead of a download by root into the host cache.
FROM golang:1.25.12-bookworm

# go-task: the fuzz:* tasks are written for it, and the external fuzzing platform calls them.
ARG TASK_VERSION=v3.53.1
RUN go install github.com/go-task/task/v3/cmd/task@${TASK_VERSION} \
    && go clean -cache -modcache

# GOTOOLCHAIN=local is already the default here and is kept on purpose: go.mod asks for
# go 1.25.0, which this base satisfies, so a toolchain download is never needed and an
# accidental version bump should fail loudly rather than fetch a toolchain.
ENV GOTOOLCHAIN=local \
    GOPROXY=off \
    GOMODCACHE=/gomodcache \
    GOCACHE=/gocache

# GOCACHE also holds the fuzzing corpus ($GOCACHE/fuzz), which is why the tasks give it a
# named volume: without one, everything the fuzzer discovers dies with the container.
RUN mkdir -p /gocache /gomodcache

WORKDIR /src/images/pvc-artifact
