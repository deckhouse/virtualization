FROM k3d-registry.virtualization-controller.test:5000/cdi-registry-importer:latest

RUN apt update && apt install -y ca-certificates && rm -rf /var/lib/apt/lists/*

ADD entrypoint.sh /
