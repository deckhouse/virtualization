FROM golang:1.25-bookworm@sha256:019c22232e57fda8ded2b10a8f201989e839f3d3f962d4931375069bbb927e03 AS builder
ARG TARGETOS
ARG TARGETARCH

WORKDIR /app/images/virtualization-dra
RUN go install github.com/go-delve/delve/cmd/dlv@latest

COPY ./images/virtualization-dra/go.mod /app/images/virtualization-dra/
COPY ./images/virtualization-dra/go.sum /app/images/virtualization-dra/

RUN go mod download

COPY ./images/virtualization-dra/cmd /app/images/virtualization-dra/cmd
COPY ./images/virtualization-dra/internal /app/images/virtualization-dra/internal
COPY ./images/virtualization-dra/pkg /app/images/virtualization-dra/pkg
COPY ./images/virtualization-dra/api /app/images/virtualization-dra/api

ENV GO111MODULE=on
ENV GOOS=${TARGETOS:-linux}
ENV GOARCH=${TARGETARCH:-amd64}
ENV CGO_ENABLED=0

RUN go build -gcflags "all=-N -l" -a -o virtualization-dra-plugin ./cmd/virtualization-dra-plugin/main.go

FROM busybox:1.36.1-glibc

WORKDIR /app
COPY --from=builder /go/bin/dlv /app/dlv
COPY --from=builder /app/images/virtualization-dra/virtualization-dra-plugin /app/virtualization-dra-plugin
USER 65532:65532

ENTRYPOINT ["./dlv", "--listen=:2345", "--headless=true", "--continue", "--log=true", "--log-output=debugger,debuglineerr,gdbwire,lldbout,rpc", "--accept-multiclient", "--api-version=2", "exec", "./virtualization-dra-plugin", "--"]
