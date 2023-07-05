#FROM alpine:3.18
#FROM ghcr.io/werf/cdi-registry-importer:latest
#FROM ghcr.io/diafour/registry-importer:latest
FROM k3d-registry.virtualization-controller.test:5000/registry-importer-base:latest

RUN apt update && apt install -y ca-certificates && rm -rf /var/lib/apt/lists/*

ADD entrypoint.sh /
