ARG BUILDER_CACHE_IMAGE=golang:1.20-bookworm
FROM $BUILDER_CACHE_IMAGE as builder

# Cache-friendly download modules.
ADD go.mod go.sum /app/
WORKDIR /app
RUN go mod download

# Build uploader
RUN rm -rf /app
ADD . /app
RUN apt-get -qq update && apt-get -qq install -y --no-install-recommends libnbd-dev
RUN GOOS=linux \
    go build -o uploader ./cmd/dvcr_uploader

FROM debian:bookworm-slim
RUN apt-get -qq update && apt-get -qq install -y --no-install-recommends \
    ca-certificates \
    libnbd0 \
    qemu-utils \
    file && \
    rm -rf /var/lib/apt/lists/*
COPY --from=builder /app/uploader /usr/local/bin/dvcr_uploader

ADD build/uploader_entrypoint.sh /

CMD ["/usr/local/bin/dvcr_uploader"]
