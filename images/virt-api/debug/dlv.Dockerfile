# syntax=docker/dockerfile:1
# dlv builds natively (no emulation) and cross-compiles to linux/amd64.
FROM --platform=$BUILDPLATFORM golang:1.25 AS dlv-builder
RUN GOOS=linux GOARCH=amd64 go install github.com/go-delve/delve/cmd/dlv@latest && \
    { cp "$(go env GOPATH)/bin/linux_amd64/dlv" /dlv-linux-amd64 2>/dev/null || \
      cp "$(go env GOPATH)/bin/dlv" /dlv-linux-amd64 ; }

# Builder runs natively and cross-compiles to linux/amd64.
FROM --platform=$BUILDPLATFORM golang:1.25 AS builder

ARG BRANCH="1.6.2-virtualization"
ENV VERSION="1.6.2"
ENV GOVERSION="1.23.0"

RUN mkdir -p -m 0700 ~/.ssh && ssh-keyscan fox.flant.com >> ~/.ssh/known_hosts

# Tip commit of BRANCH is resolved on the host by the Taskfile (cache buster);
# the clone authenticates through the forwarded host ssh agent.
ARG COMMIT=""
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

RUN go build -o /kubevirt-binaries/virt-api ./cmd/virt-api/

FROM busybox

WORKDIR /app
COPY --from=builder /kubevirt-binaries/virt-api /app/virt-api
COPY --from=dlv-builder /dlv-linux-amd64 /app/dlv
USER 65532:65532
ENTRYPOINT ["./dlv", "--listen=:2345", "--headless=true", "--continue", "--log=true", "--log-output=debugger,debuglineerr,gdbwire,lldbout,rpc", "--accept-multiclient", "--api-version=2", "exec", "/app/virt-api", "--"]
