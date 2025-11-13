FROM golang:1.24.7-bookworm@sha256:2c5f7a0c252a17cf6aa30ddee15caa0f485ee29410a6ea64cddb62eea2b07bdf AS builder
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

ENV GO111MODULE=on
ENV GOOS=${TARGETOS:-linux}
ENV GOARCH=${TARGETARCH:-amd64}
ENV CGO_ENABLED=0

RUN go build -tags EE -gcflags "all=-N -l" -a -o virtualization-dra-plugin ./cmd/virtualization-dra-plugin

FROM busybox:1.36.1-glibc

WORKDIR /app
COPY --from=builder /go/bin/dlv /app/dlv
COPY --from=builder /app/images/virtualization-dra/virtualization-dra-plugin /app/virtualization-dra-plugin
USER 65532:65532

ENTRYPOINT ["./dlv", "--listen=:2345", "--headless=true", "--continue", "--log=true", "--log-output=debugger,debuglineerr,gdbwire,lldbout,rpc", "--accept-multiclient", "--api-version=2", "exec", "./virtualization-dra-plugin", "--"]
