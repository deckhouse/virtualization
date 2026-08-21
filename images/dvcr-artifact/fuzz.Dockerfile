# A linux/amd64 environment for running this module's fuzz targets the way they ship:
# GOARCH=amd64 with cgo enabled, so CDI's vddk-datasource_amd64.go and its libnbd
# binding are actually compiled. See FUZZING.md for why arm64-with-cgo-off is not
# the same thing.
#
# The image carries the toolchain and the native dependencies only; the module
# sources and the Go module cache are mounted at run time. Nothing here is copied
# from the repository, so this file has no build context to speak of and cannot
# go stale against go.mod. See FUZZING.md, "Why the module cache is mounted".
#
# Build and run through `task docker:fuzz:*` rather than by hand.

# Must satisfy the `go` directive of the repository-root go.work (1.25.12) on its
# own: GOTOOLCHAIN=local is set below because GOPROXY is off at run time and a
# toolchain download would be the first thing to fail.
FROM golang:1.26-bookworm

# qemu-utils and file are not optional. pkg/registry/imageinfo.go shells out to
# qemu-img and file(1); without them requireImageTools fails every target in
# pkg/registry outright.
# libnbd-dev + pkg-config are what the cgo build of libguestfs.org/libnbd needs.
RUN apt-get update \
    && apt-get install -y --no-install-recommends \
        file \
        libnbd-dev \
        pkg-config \
        qemu-utils \
    && rm -rf /var/lib/apt/lists/*

ARG TASK_VERSION=v3.45.4
RUN curl -fsSL "https://github.com/go-task/task/releases/download/${TASK_VERSION}/task_linux_amd64.tar.gz" \
    | tar -xz -C /usr/local/bin task

ENV CGO_ENABLED=1 \
    GOTOOLCHAIN=local \
    GOPROXY=off \
    GOFLAGS=-mod=readonly
# The libnbd Go bindings declare their callbacks K&R style in wrappers.h, which a
# gcc defaulting to C23 rejects. bookworm's gcc-12 still defaults to gnu17, so
# this is belt and braces against a newer base image.
ENV CGO_CFLAGS=-std=gnu17

# Fail the build, not the first fuzz run, if any of the four is missing.
RUN qemu-img --version | head -1 \
    && file --version | head -1 \
    && pkg-config --modversion libnbd \
    && task --version \
    && go version

# The repository is mounted at /src: the tests read testdata/ relative to the
# working directory, and go.work sits at the repository root.
WORKDIR /src/images/dvcr-artifact
