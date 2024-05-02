# Go builder.
FROM golang:1.21-alpine3.19 AS builder

# Cache-friendly download of go dependencies.
ADD go.mod go.sum /app/
WORKDIR /app
RUN go mod download

ADD . /app

RUN GOOS=linux \
    go build -o proxy ./cmd/kube-api-proxy

# Go builder.
FROM golang:1.21-alpine3.19 AS builder-test-controller

# Cache-friendly download of go dependencies.
ADD local/test-controller/go.mod local/test-controller/go.sum /app/
WORKDIR /app
RUN go mod download

ADD local/test-controller/main.go /app/

RUN GOOS=linux \
    go build -o test-controller .

FROM alpine:3.19
RUN apk --no-cache add ca-certificates bash sed tini curl && \
    kubectlArch=linux/amd64 && \
    echo "Download kubectl for ${kubectlArch}" && \
    wget https://storage.googleapis.com/kubernetes-release/release/v1.27.5/bin/${kubectlArch}/kubectl -O /bin/kubectl && \
    chmod +x /bin/kubectl
COPY --from=builder /app/proxy /
COPY --from=builder-test-controller /app/test-controller /
ADD local/proxy.kubeconfig /

# Use user nobody.
USER 65534:65534
WORKDIR /
