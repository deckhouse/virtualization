#FROM alpine:3.18
FROM ghcr.io/werf/cdi-registry-importer:latest

RUN apt update && apt install -y ca-certificates && rm -rf /var/lib/apt/lists/*

ADD entrypoint.sh /
