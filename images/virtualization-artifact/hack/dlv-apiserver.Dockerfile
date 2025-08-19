FROM golang:1.24.5-bookworm@sha256:ef8c5c733079ac219c77edab604c425d748c740d8699530ea6aced9de79aea40 AS builder
ARG TARGETOS
ARG TARGETARCH

WORKDIR /app/virtualization-artifact
RUN go install github.com/go-delve/delve/cmd/dlv@latest

COPY ./images/virtualization-artifact/go.mod /app/virtualization-artifact/
COPY ./images/virtualization-artifact/go.sum /app/virtualization-artifact/
COPY ./api/ /app/api/

RUN go mod download

COPY ./images/virtualization-artifact/cmd /app/virtualization-artifact/cmd
COPY ./images/virtualization-artifact/pkg /app/virtualization-artifact/pkg

ENV GO111MODULE=on
ENV GOOS=${TARGETOS:-linux}
ENV GOARCH=${TARGETARCH:-amd64}
ENV CGO_ENABLED=0

RUN go build -gcflags "all=-N -l" -a -o virtualization-api ./cmd/virtualization-api

FROM busybox:1.36.1-glibc

WORKDIR /app
COPY --from=builder /go/bin/dlv /app/dlv
COPY --from=builder /app/virtualization-artifact/virtualization-api /app/virtualization-api
USER 65532:65532

ENTRYPOINT ["./dlv", "--listen=:2345", "--headless=true", "--continue", "--log=true", "--log-output=debugger,debuglineerr,gdbwire,lldbout,rpc", "--accept-multiclient", "--api-version=2", "exec", "./virtualization-api", "--"]
