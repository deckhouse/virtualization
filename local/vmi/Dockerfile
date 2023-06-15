ARG BUILDER_CACHE_IMAGE=golang:1.20-alpine3.16
FROM $BUILDER_CACHE_IMAGE as builder

# Cache-friendly download modules.
ADD go.mod go.sum /app/
WORKDIR /app
RUN go mod download

# Build operator
ADD . /app
RUN GOOS=linux \
    go build -o vmi-controller \
             ./cmd/vmi-controller


FROM alpine:3.18
COPY --from=builder /app/vmi-controller /
