ARG BUILDER_CACHE_IMAGE=golang:1.21-bookworm@sha256:c6a5b9308b3f3095e8fde83c8bf4d68bd101fce606c1a0a1394522542509dda9
FROM $BUILDER_CACHE_IMAGE AS builder

# Cache-friendly download modules.
ADD go.mod go.sum /app/
WORKDIR /app
RUN go mod download

# Build importer
RUN rm -rf /app
ADD . /app
RUN apt-get -qq update && apt-get -qq install -y --no-install-recommends libnbd-dev
RUN GOOS=linux \
    go build -o importer ./cmd/dvcr_importer

FROM debian:bookworm-slim@sha256:2ccc7e39b0a6f504d252f807da1fc4b5bcd838e83e4dec3e2f57b2a4a64e7214
RUN apt-get -qq update && apt-get -qq install -y --no-install-recommends \
    ca-certificates \
    libnbd0 \
    qemu-utils \
    file && \
    rm -rf /var/lib/apt/lists/*
COPY --from=builder /app/importer /usr/local/bin/dvcr_importer

ADD build/importer_entrypoint.sh /

CMD ["/usr/local/bin/dvcr_importer"]
