ARG BUILDER_CACHE_IMAGE=golang:1.20-alpine3.16
FROM $BUILDER_CACHE_IMAGE as builder

# Cache-friendly download modules.
ADD go.mod go.sum /app/
WORKDIR /app
RUN go mod download

# Build operator
RUN rm -rf /app
ADD . /app
RUN GOOS=linux \
    go build -o virtualization-controller ./cmd/virtualization-controller


FROM alpine:3.18
COPY --from=builder /app/virtualization-controller /
