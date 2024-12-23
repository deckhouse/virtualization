https://gist.github.com/eagleusb/5cfa996e154c31154131424bb2821f21

https://github.com/qemu/qemu/blob/master/tests/docker/dockerfiles/centos9.docker


# ------

FROM alt:p11

# FROM alt:p11@sha256:39f03d3bca1a92dc36835c28c2ba2f22ec15257e950b3930e0a3f034466e8dfb

# ENV LIBVIRT_VERSION=10.2.0-alt1
ENV LIBVIRT_VERSION=10.10.0-alt1

RUN 
rpm-build-python3
meson >= 1.1.0
glibc-devel-static zlib-devel-static glib2-devel-static libpcre2-devel-static libattr-devel-static w-devel-static libatomic-devel-static
glib2-devel >= 2.66 libgio-devel
libdw-devel
makeinfo perl-devel python3-module-sphinx python3-module-sphinx_rtd_theme
libcap-ng-devel
libxfs-devel
zlib-devel libcurl-devel >= 7.29.0 libpci-devel glibc-kernheaders
ipxe-roms-qemu >= 1:20161208-alt1.git26050fd seavgabios seabios >= 1.7.4-alt2 libfdt-devel >= 1.5.1 qboot
libpixman-devel >= 0.21.8
libkeyutils-devel
libxdp-devel >= 1.4.0}
python3-devel >= 3.8
flex
libSDL2-devel libSDL2_image-devel}
libncursesw-devel}
libalsa-devel}
libpulseaudio-devel}
pkgconfig(libpipewire-0.3) >= 0.3.60}
libjack-devel jack-audio-connection-kit}
libsndio-devel}
pkgconfig(capstone) >= 3.0.5}
libsasl2-devel}
libjpeg-devel}
libpng-devel >= 1.6.34}
common: libxkbcommon-devel xkeyboard-config-devel}
libvde-devel}
libaio-devel}
liburing-devel >= 0.3}
libbpf-devel >= 1.1.0}
libspice-server-devel >= 0.14.0 spice-protocol >= 0.14.0}
libuuid-devel
libcacard-devel >= 2.5.1}
libusbredir-devel >= 0.5}
libepoxy-devel libgbm-devel}
glib2-devel >= 2.38}
ceph-devel >= 1.12.0}
libvitastor-devel}
libiscsi-devel >= 1.9.0}
libnfs-devel >= 1.9.3}
libzstd-devel >= 1.4.0}
libseccomp-devel >= 2.3.0}
pkgconfig(glusterfs-api)}
libgtk+3-devel >= 3.22.0 pkgconfig(vte-2.91)}
libgnutls-devel >= 3.5.18}
libnettle-devel >= 3.4}
libgcrypt-devel >= 1.8.0}
libselinux-devel}
libqpl-devel >= 1.5.0}
libpam-devel
libtasn1-devel
libslirp-devel >= 4.1.0
pkgconfig(virglrenderer)}
libssh-devel >= 0.8.7}
libusb-devel >= 1.0.13}
rdma-core-devel}
libnuma-devel}
liblzo2-devel}
libsnappy-devel}
bzlib-devel}
liblzfse-devel}
libxen-devel}
libudev-devel libmultipath-devel}
libblkio-devel}
libpmem-devel}
libudev-devel}
libdaxctl-devel}
libfuse3-devel}
# used by some linux user impls
 libdrm-devel

%global requires_all_modules \
Requires: %name-block-curl \
Requires: %name-block-dmg  \
sterfs:Requires: %name-block-gluster} \
iscsi:Requires: %name-block-iscsi} \
nfs:Requires: %name-block-nfs}     \
:Requires: %name-block-rbd}        \
asor:Requires: %name-block-vitasor} \
ssh:Requires: %name-block-ssh}     \
a:Requires: %name-audio-alsa}      \
:Requires: %name-audio-oss}        \
ewire:Requires: %name-audio-pipewire}  \
seaudio:Requires: %name-audio-pa}  \
k:Requires: %name-audio-jack}      \
io:Requires: %name-audio-sndio}    \
:Requires: %name-audio-sdl}        \
ce:Requires: %name-audio-spice}    \
ses:Requires: %name-ui-curses}     \
ce:Requires: %name-ui-spice-app}   \
ce:Requires: %name-ui-spice-core}  \
ce:Requires: %name-device-display-qxl} \
Requires: %name-device-display-virtio-gpu-pci    \
Requires: %name-device-display-virtio-vga        \
glrenderer:Requires: %name-device-display-virtio-gpu} \
glrenderer:Requires: %name-device-display-virtio-gpu-gl}  \
glrenderer:Requires: %name-device-display-virtio-gpu-pci-gl} \
glrenderer:Requires: %name-device-display-virtio-vga-gl}  \
glrenderer:Requires: %name-device-display-vhost-user-gpu} \
api:Requires: %name-char-baum} \
ce:Requires: %name-char-spice} \
Requires: %name-device-usb-host \
Requires: %name-device-usb-redirect \
rtcard:Requires: %name-device-usb-smartcard}

##%ngl:Requires: %%name-ui-opengl} \
##%ngl:Requires: %%name-ui-egl-headless} \
##%:Requires: %%name-ui-gtk}       \
##%:Requires: %%name-ui-sdl}


BuildRequires(pre): rpm-build-python3
BuildRequires: meson >= 1.1.0
BuildRequires: glibc-devel-static zlib-devel-static glib2-devel-static libpcre2-devel-static libattr-devel-static w-devel-static libatomic-devel-static
BuildRequires: glib2-devel >= 2.66 libgio-devel
BuildRequires: libdw-devel
BuildRequires: makeinfo perl-devel python3-module-sphinx python3-module-sphinx_rtd_theme
BuildRequires: libcap-ng-devel
BuildRequires: libxfs-devel
BuildRequires: zlib-devel libcurl-devel >= 7.29.0 libpci-devel glibc-kernheaders
BuildRequires: ipxe-roms-qemu >= 1:20161208-alt1.git26050fd seavgabios seabios >= 1.7.4-alt2 libfdt-devel >= 1.5.1 qboot
BuildRequires: libpixman-devel >= 0.21.8
BuildRequires: libkeyutils-devel
%{?_enable_af_xdp:BuildRequires: libxdp-devel >= 1.4.0}
BuildRequires: python3-devel >= 3.8
BuildRequires: flex
%{?_enable_sdl:BuildRequires: libSDL2-devel libSDL2_image-devel}
%{?_enable_curses:BuildRequires: libncursesw-devel}
%{?_enable_alsa:BuildRequires: libalsa-devel}
%{?_enable_pulseaudio:BuildRequires: libpulseaudio-devel}
%{?_enable_pipewire:BuildRequires: pkgconfig(libpipewire-0.3) >= 0.3.60}
%{?_enable_jack:BuildRequires: libjack-devel jack-audio-connection-kit}
%{?_enable_sndio:BuildRequires: libsndio-devel}
%{?_enable_capstone:BuildRequires: pkgconfig(capstone) >= 3.0.5}
%{?_enable_vnc_sasl:BuildRequires: libsasl2-devel}
%{?_enable_vnc_jpeg:BuildRequires: libjpeg-devel}
%{?_enable_png:BuildRequires: libpng-devel >= 1.6.34}
%{?_enable_xkbcommon:BuildRequires: libxkbcommon-devel xkeyboard-config-devel}
%{?_enable_vde:BuildRequires: libvde-devel}
%{?_enable_aio:BuildRequires: libaio-devel}
%{?_enable_io_uring:BuildRequires: liburing-devel >= 0.3}
%{?_enable_bpf:BuildRequires: libbpf-devel >= 1.1.0}
%{?_enable_spice:BuildRequires: libspice-server-devel >= 0.14.0 spice-protocol >= 0.14.0}
BuildRequires: libuuid-devel
%{?_enable_smartcard:BuildRequires: libcacard-devel >= 2.5.1}
%{?_enable_usb_redir:BuildRequires: libusbredir-devel >= 0.5}
%{?_enable_opengl:BuildRequires: libepoxy-devel libgbm-devel}
%{?_enable_guest_agent:BuildRequires: glib2-devel >= 2.38}
%{?_enable_rbd:BuildRequires: ceph-devel >= 1.12.0}
%{?_enable_vitastor:BuildRequires: libvitastor-devel}
%{?_enable_libiscsi:BuildRequires: libiscsi-devel >= 1.9.0}
%{?_enable_libnfs:BuildRequires: libnfs-devel >= 1.9.3}
%{?_enable_zstd:BuildRequires: libzstd-devel >= 1.4.0}
%{?_enable_seccomp:BuildRequires: libseccomp-devel >= 2.3.0}
%{?_enable_glusterfs:BuildRequires: pkgconfig(glusterfs-api)}
%{?_enable_gtk:BuildRequires: libgtk+3-devel >= 3.22.0 pkgconfig(vte-2.91)}
%{?_enable_gnutls:BuildRequires: libgnutls-devel >= 3.5.18}
%{?_enable_nettle:BuildRequires: libnettle-devel >= 3.4}
%{?_enable_gcrypt:BuildRequires: libgcrypt-devel >= 1.8.0}
%{?_enable_selinux:BuildRequires: libselinux-devel}
%{?_enable_qpl:BuildRequires: libqpl-devel >= 1.5.0}
BuildRequires: libpam-devel
BuildRequires: libtasn1-devel
BuildRequires: libslirp-devel >= 4.1.0
%{?_enable_virglrenderer:BuildRequires: pkgconfig(virglrenderer)}
%{?_enable_libssh:BuildRequires: libssh-devel >= 0.8.7}
%{?_enable_libusb:BuildRequires: libusb-devel >= 1.0.13}
%{?_enable_rdma:BuildRequires: rdma-core-devel}
%{?_enable_numa:BuildRequires: libnuma-devel}
%{?_enable_lzo:BuildRequires: liblzo2-devel}
%{?_enable_snappy:BuildRequires: libsnappy-devel}
%{?_enable_bzip2:BuildRequires: bzlib-devel}
%{?_enable_lzfse:BuildRequires: liblzfse-devel}
%{?_enable_xen:BuildRequires: libxen-devel}
%{?_enable_mpath:BuildRequires: libudev-devel libmultipath-devel}
%{?_enable_blkio:BuildRequires: libblkio-devel}
%{?_enable_libpmem:BuildRequires: libpmem-devel}
%{?_enable_libudev:BuildRequires: libudev-devel}
%{?_enable_libdaxctl:BuildRequires: libdaxctl-devel}
%{?_enable_fuse:BuildRequires: libfuse3-devel}
# used by some linux user impls
BuildRequires: libdrm-devel

%global requires_all_modules \
Requires: %name-block-curl \
Requires: %name-block-dmg  \
%{?_enable_glusterfs:Requires: %name-block-gluster} \
%{?_enable_libiscsi:Requires: %name-block-iscsi} \
%{?_enable_libnfs:Requires: %name-block-nfs}     \
%{?_enable_rbd:Requires: %name-block-rbd}        \
%{?_enable_vitasor:Requires: %name-block-vitasor} \
%{?_enable_libssh:Requires: %name-block-ssh}     \
%{?_enable_alsa:Requires: %name-audio-alsa}      \
%{?_enable_oss:Requires: %name-audio-oss}        \
%{?_enable_pipewire:Requires: %name-audio-pipewire}  \
%{?_enable_pulseaudio:Requires: %name-audio-pa}  \
%{?_enable_jack:Requires: %name-audio-jack}      \
%{?_enable_sndio:Requires: %name-audio-sndio}    \
%{?_enable_sdl:Requires: %name-audio-sdl}        \
%{?_enable_spice:Requires: %name-audio-spice}    \
%{?_enable_curses:Requires: %name-ui-curses}     \
%{?_enable_spice:Requires: %name-ui-spice-app}   \
%{?_enable_spice:Requires: %name-ui-spice-core}  \
%{?_enable_spice:Requires: %name-device-display-qxl} \
Requires: %name-device-display-virtio-gpu-pci    \
Requires: %name-device-display-virtio-vga        \
%{?_enable_virglrenderer:Requires: %name-device-display-virtio-gpu} \
%{?_enable_virglrenderer:Requires: %name-device-display-virtio-gpu-gl}  \
%{?_enable_virglrenderer:Requires: %name-device-display-virtio-gpu-pci-gl} \
%{?_enable_virglrenderer:Requires: %name-device-display-virtio-vga-gl}  \
%{?_enable_virglrenderer:Requires: %name-device-display-vhost-user-gpu} \
%{?_enable_brlapi:Requires: %name-char-baum} \
%{?_enable_spice:Requires: %name-char-spice} \
Requires: %name-device-usb-host \
Requires: %name-device-usb-redirect \
%{?_enable_smartcard:Requires: %name-device-usb-smartcard}

##%%{?_enable_opengl:Requires: %%name-ui-opengl} \
##%%{?_enable_opengl:Requires: %%name-ui-egl-headless} \
##%%{?_enable_gtk:Requires: %%name-ui-gtk}       \
##%%{?_enable_sdl:Requires: %%name-ui-sdl}