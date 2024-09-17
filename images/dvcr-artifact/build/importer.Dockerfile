ARG BUILDER_CACHE_IMAGE=golang:1.21-bookworm
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
    go build -o importer ./cmd/dvcr-importer

FROM debian:bookworm-slim
RUN apt-get -qq update && apt-get -qq install -y --no-install-recommends \
    ca-certificates \
    libnbd0 \
    qemu-utils \
    file && \
    rm -rf /var/lib/apt/lists/*
COPY --from=builder /app/importer /usr/local/bin/dvcr-importer

ADD build/importer_entrypoint.sh /

CMD ["/usr/local/bin/dvcr-importer"]
