FROM golang:1.23-bookworm

RUN apt update -y && apt install -y \
  build-essential \
  libnbd-dev \
  qemu-utils

WORKDIR /app

COPY go.mod go.sum ./

RUN go mod download && go mod verify

COPY . .

COPY fuzz.sh /app/fuzz.sh
RUN chmod +x /app/fuzz.sh

ENV GOOS=linux
ENV GOARCH=amd64

CMD ["/app/fuzz.sh"]
