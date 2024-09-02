ARG BUILDER_CACHE_IMAGE=golang:1.20-alpine3.16@sha256:6469405d7297f82d56195c90a3270b0806ef4bd897aa0628477d9959ab97a577
FROM $BUILDER_CACHE_IMAGE AS builder

# Cache-friendly download modules.
ADD go.mod go.sum /app/
WORKDIR /app
RUN go mod download

# Build operator
RUN rm -rf /app
ADD . /app
RUN GOOS=linux \
    go build -o virtualization-controller ./cmd/virtualization-controller


FROM alpine:3.18@sha256:5292533eb4efd4b5cf35e93b5a2b7d0e07ea193224c49446c7802c19ee4f2da5
# curl for testing progress metrics
RUN apk add --no-cache curl
COPY --from=builder /app/virtualization-controller /
