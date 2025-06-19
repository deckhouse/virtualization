FROM golang:1.22.7 AS builder

RUN go install github.com/go-delve/delve/cmd/dlv@latest

ARG BRANCH="1.3.1-virtualization"
ENV VERSION="1.3.1"
ENV GOVERSION="1.22.7"

# Copy the git commits for rebuilding the image if the branch changes
ADD "https://api.github.com/repos/deckhouse/3p-kubevirt/commits/$BRANCH" /.git-commit-hash.tmp
RUN git clone --depth 1 --branch $BRANCH https://github.com/deckhouse/3p-kubevirt.git /kubevirt
WORKDIR /kubevirt

RUN go mod edit -go=$GOVERSION && \
    go mod download

RUN go mod vendor

ENV GO111MODULE=on
ENV GOOS=linux
ENV CGO_ENABLED=0
ENV GOARCH=amd64

RUN go build -o /kubevirt-binaries/virt-controller ./cmd/virt-controller/

FROM busybox

WORKDIR /app
COPY --from=builder /kubevirt-binaries/virt-controller /app/virt-controller
COPY --from=builder /go/bin/dlv /app/dlv
USER 65532:65532
ENTRYPOINT ["./dlv", "--listen=:2345", "--headless=true", "--continue", "--log=true", "--log-output=debugger,debuglineerr,gdbwire,lldbout,rpc", "--accept-multiclient", "--api-version=2", "exec", "/app/virt-controller", "--"]
