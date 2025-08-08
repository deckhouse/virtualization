FROM golang:1.24-bookworm@sha256:ef8c5c733079ac219c77edab604c425d748c740d8699530ea6aced9de79aea40 AS builder
WORKDIR /app/images/vm-route-forge
RUN go install github.com/go-delve/delve/cmd/dlv@latest

COPY ./images/vm-route-forge/go.mod /app/images/vm-route-forge
COPY ./images/vm-route-forge/go.sum /app/images/vm-route-forge
COPY ./api/ /app/api
RUN go mod download

COPY ./images/vm-route-forge/cmd /app/images/vm-route-forge/cmd
COPY ./images/vm-route-forge/internal /app/images/vm-route-forge/internal
COPY ./images/vm-route-forge/bpf /app/images/vm-route-forge/bpf

ENV GOOS=linux
ENV GOARCH=amd64
ENV CGO_ENABLED=0

RUN go build -gcflags "all=-N -l" -v -a -o vm-route-forge cmd/vm-route-forge/main.go

FROM busybox

COPY --from=builder /go/bin/dlv /app/dlv
COPY --from=builder /app/images/vm-route-forge/vm-route-forge /app/vm-route-forge
USER 65532:65532
WORKDIR /app

ENTRYPOINT ["./dlv", "--listen=:2345", "--headless=true", "--continue", "--log=true", "--log-output=debugger,debuglineerr,gdbwire,lldbout,rpc", "--accept-multiclient", "--api-version=2", "exec", "./vm-route-forge", "--"]
