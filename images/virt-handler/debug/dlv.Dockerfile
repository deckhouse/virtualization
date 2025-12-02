FROM alt:p11@sha256:39f03d3bca1a92dc36835c28c2ba2f22ec15257e950b3930e0a3f034466e8dfb AS basealt
RUN groupadd --gid 1001 nonroot-user && useradd nonroot-user --uid 1001 --gid 1001 --shell /bin/bash --create-home

FROM basealt AS builder

# TODO add pin repository url
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

ARG BRANCH="1.6.2-virtualization"
ENV VERSION="1.6.2"
ENV GOVERSION="1.23.0"

RUN mkdir /kubevirt-config-files && echo "v$VERSION-dirty" > /kubevirt-config-files/.version

# Copy the git commits for rebuilding the image if the branch changes
ADD "https://api.github.com/repos/deckhouse/3p-kubevirt/commits/$BRANCH" /.git-commit-hash.tmp
RUN git clone --depth 1 --branch $BRANCH https://github.com/deckhouse/3p-kubevirt.git /kubevirt
WORKDIR /kubevirt

RUN go mod edit -go=$GOVERSION && \
    go mod download

RUN go work vendor

RUN for p in $(test -d patches && ls -1 patches/*.patch 2>/dev/null) ; do \
        echo -n "Apply ${p} ... " ; \
        git apply --ignore-space-change --ignore-whitespace ${p} && echo OK || (echo FAIL ; exit 1) ; \
    done

ENV GO111MODULE=on
ENV GOOS=linux
ENV CGO_ENABLED=1
ENV GOARCH=amd64

RUN go build -gcflags="all=-N -l" -o /kubevirt-binaries/virt-handler ./cmd/virt-handler/
RUN gcc -static cmd/container-disk-v2alpha/main.c -o /kubevirt-binaries/container-disk
RUN go build -gcflags="all=-N -l" -o /kubevirt-binaries/virt-chroot ./cmd/virt-chroot/

FROM basealt

RUN apt-get update && apt-get install --yes \
        acl \
        procps \
        nftables && \
    apt-get clean && \
    rm --recursive --force /var/lib/apt/lists/ftp.altlinux.org* /var/cache/apt/*.bin

RUN echo "qemu:x:107:107::/home/qemu:/bin/bash" >> /etc/passwd && \
    echo "qemu:x:107:" >> /etc/group                           && \
    mkdir -p /home/qemu                                        && \
    chown -R 107:107 /home/qemu

COPY --from=builder /kubevirt-config-files/.version /.version
COPY --from=builder /kubevirt/cmd/virt-handler/nsswitch.conf /etc/nsswitch.conf

COPY --from=builder /kubevirt-binaries/virt-handler /usr/bin/virt-handler
COPY --from=builder /kubevirt-binaries/virt-chroot /usr/bin/virt-chroot
COPY --from=builder /kubevirt-binaries/container-disk /usr/bin/container-disk
COPY --from=builder /root/go/bin/dlv /usr/bin/dlv

ENTRYPOINT ["/usr/bin/dlv", "--listen=:2345", "--headless=true", "--continue", "--log=true", "--log-output=debugger,debuglineerr,gdbwire,lldbout,rpc", "--accept-multiclient", "--api-version=2", "exec", "/usr/bin/virt-handler", "--"]
