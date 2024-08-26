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


FROM alpine:3.20@sha256:0a4eaa0eecf5f8c050e5bba433f58c052be7587ee8af3e8b3910ef9ab5fbe9f5
# curl for testing progress metrics
RUN apk add --no-cache curl
COPY --from=builder /app/virtualization-controller /
