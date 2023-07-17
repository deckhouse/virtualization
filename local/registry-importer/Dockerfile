FROM dev-registry.deckhouse.io/virt/virtualization-importer:latest

RUN apt update && apt install -y ca-certificates && rm -rf /var/lib/apt/lists/*

ADD entrypoint.sh /
