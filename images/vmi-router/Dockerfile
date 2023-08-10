ARG BASE_UBUNTU=docker.io/ubuntu:jammy-20230624@sha256:0bced47fffa3361afa981854fcabcd4577cd43cebbb808cea2b1f33a3dd7f508
ARG BASE_GOLANG_19_BULLSEYE=docker.io/golang:1.19.12-bullseye@sha256:a0381d20066951ba8a2e6a190dbf960d62ca367dc0306d865ac65e8ccb6f62dd
# Build the manager binary
FROM $BASE_GOLANG_19_BULLSEYE as builder

WORKDIR /workspace
# Copy the Go Modules manifests
COPY go.mod go.mod
COPY go.sum go.sum
# cache deps before building and copying source so that we don't need to re-download as much
# and so that source changes don't invalidate our downloaded layer
RUN go mod download

# Copy the go source
COPY . .

# Build
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -a -o vmi-router main.go

# Use distroless as minimal base image to package the manager binary
# Refer to https://github.com/GoogleContainerTools/distroless for more details
FROM $BASE_UBUNTU
WORKDIR /
COPY --from=builder /workspace/vmi-router .
USER 65532:65532

ENTRYPOINT ["/vmi-router"]
