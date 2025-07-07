# Build kube-api-rewriter for local development purposes.
# Note: it is not a part of the production build!

# Go builder.
FROM golang:1.22.7-alpine3.19 AS builder

RUN go install github.com/go-delve/delve/cmd/dlv@latest

# Cache-friendly download of go dependencies.
ADD go.mod go.sum /app/
WORKDIR /app
RUN go mod download

ADD . /app

RUN GOOS=linux \
    go build -o kube-api-rewriter ./cmd/kube-api-rewriter

# Go builder.
FROM golang:1.22.7-alpine3.19 AS builder-test-controller

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
    wget https://storage.googleapis.com/kubernetes-release/release/v1.30.0/bin/${kubectlArch}/kubectl -O /bin/kubectl && \
    chmod +x /bin/kubectl
COPY --from=builder /go/bin/dlv /
COPY --from=builder /app/kube-api-rewriter /
COPY --from=builder-test-controller /app/test-controller /
ADD local/kube-api-rewriter.kubeconfig /

# Use user nobody.
USER 65534:65534
WORKDIR /
