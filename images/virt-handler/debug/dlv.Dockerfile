FROM alt:p11@sha256:39f03d3bca1a92dc36835c28c2ba2f22ec15257e950b3930e0a3f034466e8dfb AS basealt
RUN groupadd --gid 1001 nonroot-user && useradd nonroot-user --uid 1001 --gid 1001 --shell /bin/bash --create-home

FROM basealt AS builder

RUN apt-get update

RUN apt-get install -y \
    git curl pkg-config \
    libvirt-libs libtool libvirt-devel libncurses-devel \
    libvirt-client libvirt-daemon libvirt \
    gcc gcc-c++ glibc-devel-static \
    glibc \
    golang && \
    apt-get clean && \
    rm --recursive --force /var/lib/apt/lists/ftp.altlinux.org* /var/cache/apt/*.bin

RUN go install github.com/go-delve/delve/cmd/dlv@latest

ARG BRANCH="1.3.1-virtualization"
ENV VERSION="1.3.1"
ENV GOVERSION="1.22.7"

RUN mkdir /kubevirt-config-files && echo "v$VERSION-dirty" > /kubevirt-config-files/.version

# Copy the git commits for rebuilding the image if the branch changes
ADD "https://api.github.com/repos/deckhouse/3p-kubevirt/commits/$BRANCH" /.git-commit-hash.tmp
RUN git clone --depth 1 --branch $BRANCH https://github.com/deckhouse/3p-kubevirt.git /kubevirt
WORKDIR /kubevirt

RUN go mod edit -go=$GOVERSION && \
    go mod download

RUN go mod vendor

ENV GO111MODULE=on
ENV GOOS=linux
ENV CGO_ENABLED=1
ENV GOARCH=amd64

RUN go build -o /kubevirt-binaries/virt-handler ./cmd/virt-handler/
RUN gcc -static cmd/container-disk-v2alpha/main.c -o /kubevirt-binaries/container-disk
RUN go build -o /kubevirt-binaries/virt-chroot ./cmd/virt-chroot/

FROM basealt

RUN apt-get update && apt-get install --yes \
        acl \
        procps \
        nftables \
        qemu-img==9.0.2-alt3 \
        xorriso==1.5.6-alt1 && \
    apt-get clean && \
    rm --recursive --force /var/lib/apt/lists/ftp.altlinux.org* /var/cache/apt/*.bin

RUN echo "qemu:x:107:107::/home/qemu:/bin/bash" >> /etc/passwd && \
    echo "qemu:x:107:" >> /etc/group                           && \
    mkdir -p /home/qemu                                        && \
    chown -R 107:107 /home/qemu

COPY --from=builder /kubevirt/cmd/virt-handler/virt_launcher.cil /virt_launcher.cil
COPY --from=builder /kubevirt-config-files/.version /.version
COPY --from=builder /kubevirt/cmd/virt-handler/nsswitch.conf /etc/nsswitch.conf

COPY --from=builder /kubevirt-binaries/virt-handler /usr/bin/virt-handler
COPY --from=builder /kubevirt-binaries/virt-chroot /usr/bin/virt-chroot
COPY --from=builder /kubevirt-binaries/container-disk /usr/bin/container-disk
COPY --from=builder /root/go/bin/dlv /usr/bin/dlv

ENTRYPOINT ["/usr/bin/dlv", "--listen=:2345", "--headless=true", "--continue", "--log=true", "--log-output=debugger,debuglineerr,gdbwire,lldbout,rpc", "--accept-multiclient", "--api-version=2", "exec", "/usr/bin/virt-handler", "--"]
