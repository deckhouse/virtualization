ARG BASE_UBUNTU=registry.deckhouse.io/base_images/ubuntu:jammy-20221130@sha256:c14c3b1242536729ce5227ff833144977b4e378723858fb73a4cf40ea6daaf6a
ARG BASE_GOLANG_19_BULLSEYE=registry.deckhouse.io/base_images/golang:1.19.3-bullseye@sha256:3d68e9eabd09f01f5851297902f2756ee2456a2e28b212e553315a3ddfcffe4b
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
