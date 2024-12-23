%ifarch %ix86 x86_64
%global kvm_package system-x86
%def_enable qemu_kvm
%endif

%global _group vmusers
%global rulenum 90
%global _libexecdir /usr/libexec
%global _localstatedir /var
%global firmwaredirs "%_datadir/qemu:%_datadir/seabios:%_datadir/seavgabios:%_datadir/ipxe:%_datadir/ipxe.efi"


Name: qemu
Version: 9.1.2
Release: alt1

Summary: QEMU CPU Emulator
License: BSD-2-Clause AND BSD-3-Clause AND GPL-2.0-only AND GPL-2.0-or-later AND LGPL-2.1-or-later AND MIT
Group: Emulators
Url: https://www.qemu.org
# git://git.qemu.org/qemu.git
Source0: %name-%version.tar
Source100: keycodemapdb.tar
Source101: berkeley-testfloat-3.tar
Source102: berkeley-softfloat-3.tar
# qemu-kvm back compat wrapper
Source5: qemu-kvm.sh
# guest agent service
Source8: qemu-guest-agent.rules
Source9: qemu-guest-agent.service
Source10: qemu-guest-agent.init
Source11: qemu-ga.sysconfig
# /etc/qemu/bridge.conf
Source12: bridge.conf

Patch: qemu-alt.patch

#%prep
#%autosetup -n %{name}-%{version}
%prep
%patch -p1

%build
mkdir -p _build
meson _build --prefix=%{_prefix} --libdir=%{_libdir} --sysconfdir=%{_sysconfdir} \
    --localstatedir=%{_localstatedir} -Ddefault-target-list="x86_64-softmmu"
ninja -C _build

%install
DESTDIR=%{buildroot} ninja -C _build install

%files
%doc README.md
%license LICENSE
%{_bindir}/qemu-system-x86_64
%{_libdir}/qemu/

%changelog
* Mon Oct 10 2023 Your Name <you@example.com> - <your_version>-1
- Initial build for KubeVirt