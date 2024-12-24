FROM alt:p11

# FROM alt:p11@sha256:39f03d3bca1a92dc36835c28c2ba2f22ec15257e950b3930e0a3f034466e8dfb

# ENV LIBVIRT_VERSION=10.2.0-alt1
ENV LIBVIRT_VERSION=10.10.0-alt1

RUN echo "rpm-src [p11] http://ftp.altlinux.org/pub/distributions/ALTLinux p11/branch/x86_64 classic" > /etc/apt/sources.list.d/src.list && \
    apt-get update && apt-get install --yes su \
    gear rpm dmidecode \
    rpm-build \
    rpm-build-systemd \
    rpm-build-python3 \
    gcc-c++ \
    git \
    gettext \
    bash-completion \
    clang \
    ccache \
    make cmake \
    meson \
    ninja-build \
    libudev-devel \
    libpciaccess-devel \
    libyajl-devel \
    sanlock-devel \
    libpcap-devel \
    libnl-devel \
    libselinux-devel \
    iproute \
    iptables \
    iptables-nft \
    iptables-ipv6 \
    openvswitch \
    ebtables \
    libsasl2-devel \
    pkgconfig \
    libssh-devel \
    libssh2-devel \
    polkit kmod\
    qemu-img \
    lvm2 \
    libparted-devel \
    parted \
    libdevmapper-devel \
    ceph-devel \
    open-iscsi \
    libiscsi-devel \
    libglusterfs-devel \
    libnuma-devel \
    libcap-ng-devel \
    libcurl-devel \
    libaudit-devel \
    libfuse-devel \
    libnbd-devel \
    libblkid-devel \
    libgcrypt-devel \
    libgnutls-devel \
    libp11-kit-devel \
    libreadline-devel \
    libtasn1-devel \
    libattr-devel \
    libbsd-devel \
    libsystemd-devel \
    libuuid-devel \
    libjson-c-devel \
    systemtap-sdt-devel \
    systemd-container \
    attr \
    libacl-devel \
    glib2-devel \
    glibc-utils \
    libgio-devel \
    libxml2-devel \
    xml-utils \
    xsltproc \
    python3 python3-devel \
    python3-module-pytest \
    python3-module-docutils \
    python3-tools \
    python3-module-pip \
    polkit \
    libtirpc-devel \
    libsasl2-devel \
    wireshark-devel \
    tshark \
    zlib-devel \
    mdevctl \
    util-linux dmsetup pm-utils \
    libfuse3-devel libnuma libslirp-devel libtirpc-devel \
    libyajl-devel && \ 
    apt-get clean && \
    rm --recursive --force /var/lib/apt/lists/ftp.altlinux.org* /var/cache/apt/*.bin && \
        rm -f /usr/lib*/python3*/EXTERNALLY-MANAGED && \
        rpm -qa | sort > /packages.txt && \
        mkdir -p /usr/libexec/ccache-wrappers && \
        ln -s /usr/bin/ccache /usr/libexec/ccache-wrappers/cc && \
        ln -s /usr/bin/ccache /usr/libexec/ccache-wrappers/clang && \
        ln -s /usr/bin/ccache /usr/libexec/ccache-wrappers/gcc

RUN /usr/bin/pip3 install black && \
    grep -qw ^rpm /etc/group || groupadd rpm && \
    id -u builder &>/dev/null || useradd builder --shell /bin/bash --create-home --group rpm

USER builder
ENV CCACHE_WRAPPERSDIR="/usr/libexec/ccache-wrappers" \
    MAKE="/usr/bin/make" \
    NINJA="/usr/bin/ninja" \
    PYTHON="/usr/bin/python3"
    
WORKDIR /home/builder
RUN git clone http://git.altlinux.org/gears/l/libvirt.git
RUN cd libvirt && git checkout ${LIBVIRT_VERSION}

RUN cd libvirt && gear-rpm -bs --verbose
RUN apt-get source /home/builder/RPM/SRPMS/libvirt-${LIBVIRT_VERSION}.src.rpm

COPY ./libvirt.spec /home/builder/RPM/SPECS/libvirt.spec

RUN grep -n 'wireshark/plugins' /home/builder/RPM/SPECS/libvirt.spec
# RUN rpm -ba /home/builder/RPM/SPECS/libvirt.spec

RUN trap "echo Build log ; cat /home/builder/RPM/BUILD//home/builder/RPM/BUILD/libvirt-${LIBVIRT_VERSION}/x86_64-alt-linux/meson-logs/testlog.txt" EXIT ; rpm -ba /home/builder/RPM/SPECS/libvirt.spec

RUN ls -la /home/builder/RPM/RPMS/x86_64/

# apt-get clean && \
    # rm --recursive --force /var/lib/apt/lists/ftp.altlinux.org* /var/
# meson setup build \
#     -Dprefix=$HOME/libvirt_build/usr \
#     -Dlocalstatedir=$HOME/libvirt_build/var \
#     -Dsbindir=$HOME/libvirt_build/usr/sbin \
#     -Dsystem=enabled \
#     -Ddriver_qemu=enabled \
#     -Ddriver_libvirtd=enabled

# CFLAGS="-Wframe-larger-than=8192" meson setup build --prefix=$HOME/libvirt_build -Dsystem=true -Ddriver_qemu=enabled -Ddriver_openvz=disabled --wipe
# ninja -j6 -C build

# /tmp/.private/builder/libvirt-buildroot

# su - builder /bin/bash -c 'git clone http://git.altlinux.org/gears/l/libvirt.git'

# RUN su - builder /bin/bash -c 'cd libvirt ; gear-rpm -bs --verbose'
# RUN su - builder /bin/bash -c 'apt-get source /home/builder/RPM/SRPMS/libvirt-10.10.0-alt1.src.rpm'

# RUN su - builder -c 'trap "echo Build log tail: ; tail -n 1024 /tmp/build.log" EXIT ; rpm -ba /home/builder/RPM/SPECS/libvirt.spec 2>&1'

# apt-cache showpkg libssh2-devel
# apt-cache showpkg libssh-devel
