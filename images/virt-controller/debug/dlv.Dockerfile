# syntax=docker/dockerfile:1
# Builder runs natively (no qemu emulation on arm64 hosts) and cross-compiles
# everything to linux/amd64; only the final busybox stage is platform-pinned.
FROM --platform=$BUILDPLATFORM golang:1.25 AS builder

RUN GOOS=linux GOARCH=amd64 go install github.com/go-delve/delve/cmd/dlv@latest &&     { cp "$(go env GOPATH)/bin/linux_amd64/dlv" /usr/local/bin/dlv-linux-amd64 2>/dev/null ||       cp "$(go env GOPATH)/bin/dlv" /usr/local/bin/dlv-linux-amd64 ; }

ARG BRANCH="v1.6.2-virtualization"
# Tip commit of BRANCH, resolved on the host by the dlv:virt-controller:build
# task; changing it invalidates the clone layer below.
ARG COMMIT=""
ENV VERSION="1.6.2"
ENV GOVERSION="1.24.0"

RUN mkdir -p -m 0700 ~/.ssh && ssh-keyscan fox.flant.com >> ~/.ssh/known_hosts

# The kubevirt fork lives on fox (private); the clone authenticates through
# the host ssh agent forwarded by `docker build --ssh default`.
RUN --mount=type=ssh echo "commit: $COMMIT" && \
    git clone --depth 1 --branch $BRANCH ssh://git@fox.flant.com/deckhouse/virtualization/fork/kubevirt.git /kubevirt
WORKDIR /kubevirt

RUN go mod edit -go=$GOVERSION && \
    go mod download

RUN go work vendor

RUN for p in $(test -d patches && ls -1 patches/*.patch 2>/dev/null) ; do \
        echo -n "Apply ${p} ... " ; \
        git apply --ignore-space-change --ignore-whitespace ${p} && echo OK || (echo FAIL ; exit 1) ; \
    done

ENV GO111MODULE=on
ENV GOOS=linux
ENV CGO_ENABLED=0
ENV GOARCH=amd64

RUN go build -gcflags="all=-N -l" -o /kubevirt-binaries/virt-controller ./cmd/virt-controller/

FROM busybox

WORKDIR /app
COPY --from=builder /kubevirt-binaries/virt-controller /app/virt-controller
COPY --from=builder /usr/local/bin/dlv-linux-amd64 /app/dlv
USER 65532:65532
ENTRYPOINT ["./dlv", "--listen=:2345", "--headless=true", "--continue", "--log=true", "--log-output=debugger,debuglineerr,gdbwire,lldbout,rpc", "--accept-multiclient", "--api-version=2", "exec", "/app/virt-controller", "--"]
