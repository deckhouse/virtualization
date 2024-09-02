FROM golang:1.21.8-bookworm@sha256:ac14cc827536ef1a124cd2f7a03178c3335c1db8ad3807e7fdd57f74096abfa0 AS builder
ARG TARGETOS
ARG TARGETARCH

WORKDIR /app/images/virtualization-artifact
RUN go install github.com/go-delve/delve/cmd/dlv@latest

COPY ./images/virtualization-artifact/go.mod /app/images/virtualization-artifact/
COPY ./images/virtualization-artifact/go.sum /app/images/virtualization-artifact/
COPY ./api/ /app/api/

RUN go mod download

COPY ./images/virtualization-artifact/cmd /app/images/virtualization-artifact/cmd
COPY ./images/virtualization-artifact/pkg /app/images/virtualization-artifact/pkg

ENV GO111MODULE=on
ENV GOOS=${TARGETOS:-linux}
ENV GOARCH=${TARGETARCH:-amd64}
ENV CGO_ENABLED=0

RUN go build -gcflags "all=-N -l" -a -o virtualization-controller ./cmd/virtualization-controller

FROM busybox:1.36.1-musl@sha256:d097373897633648934dce99c57a4ddfc9b31872c64c5bd35ea986fc47a53fcb

WORKDIR /app
COPY --from=builder /go/bin/dlv /app/dlv
COPY --from=builder /app/images/virtualization-artifact/virtualization-controller /app/virtualization-controller
USER 65532:65532

ENTRYPOINT ["./dlv", "--listen=:2345", "--headless=true", "--continue", "--log=true", "--log-output=debugger,debuglineerr,gdbwire,lldbout,rpc", "--accept-multiclient", "--api-version=2", "exec", "./virtualization-controller", "--"]
