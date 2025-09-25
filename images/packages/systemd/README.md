# systemd
```
├── [drwxr-xr-x    14]  etc
│   ├── [drwxr-xr-x     3]  X11
│   │   └── [drwxr-xr-x     3]  xinit.d
│   │       └── [-rwxr-xr-x   538]  50-systemd-user.sh
│   ├── [drwxr-xr-x     2]  binfmt.d
│   ├── [drwx------     2]  credstore
│   ├── [drwx------     2]  credstore.encrypted
│   ├── [drwxr-xr-x     3]  kernel
│   │   └── [drwxr-xr-x     2]  install.d
│   ├── [drwxr-xr-x     3]  pam.d
│   │   └── [-rw-r--r--   562]  systemd-user
│   ├── [drwxr-xr-x     3]  rc.d
│   │   └── [drwxr-xr-x     3]  init.d
│   │       └── [-rw-r--r--  1.1K]  README
│   ├── [drwxr-xr-x     2]  sysctl.d
│   ├── [drwxr-xr-x    19]  systemd
│   │   ├── [-rw-r--r--  1018]  coredump.conf
│   │   ├── [-rw-r--r--   890]  homed.conf
│   │   ├── [-rw-r--r--  1.1K]  journal-remote.conf
│   │   ├── [-rw-r--r--  1.0K]  journal-upload.conf
│   │   ├── [-rw-r--r--  1.4K]  journald.conf
│   │   ├── [-rw-r--r--  1.6K]  logind.conf
│   │   ├── [drwxr-xr-x     2]  network
│   │   ├── [-rw-r--r--  1.1K]  networkd.conf
│   │   ├── [-rw-r--r--   928]  oomd.conf
│   │   ├── [-rw-r--r--   879]  pstore.conf
│   │   ├── [-rw-r--r--  1.5K]  resolved.conf
│   │   ├── [-rw-r--r--  1.0K]  sleep.conf
│   │   ├── [drwxr-xr-x     2]  system
│   │   ├── [-rw-r--r--  2.3K]  system.conf
│   │   ├── [-rw-r--r--   989]  timesyncd.conf
│   │   ├── [drwxr-xr-x     2]  user
│   │   └── [-rw-r--r--  1.7K]  user.conf
│   ├── [drwxr-xr-x     2]  tmpfiles.d
│   ├── [drwxr-xr-x     6]  udev
│   │   ├── [drwxr-xr-x     2]  hwdb.d
│   │   ├── [-rw-r--r--   865]  iocost.conf
│   │   ├── [drwxr-xr-x     2]  rules.d
│   │   └── [-rw-r--r--   305]  udev.conf
│   └── [drwxr-xr-x     3]  xdg
│       └── [drwxr-xr-x     3]  systemd
│           └── [lrwxrwxrwx    18]  user ->  ../../systemd/user
├── [drwxr-xr-x     7]  usr
│   ├── [drwxr-xr-x    58]  bin
│   │   ├── [-rwxr-xr-x  668K]  bootctl
│   │   ├── [-rwxr-xr-x   91K]  busctl
│   │   ├── [-rwxr-xr-x   79K]  coredumpctl
│   │   ├── [-rwxr-xr-x  131K]  homectl
│   │   ├── [-rwxr-xr-x   31K]  hostnamectl
│   │   ├── [-rwxr-xr-x 1001K]  journalctl
│   │   ├── [-rwxr-xr-x   55K]  kernel-install
│   │   ├── [-rwxr-xr-x   27K]  localectl
│   │   ├── [-rwxr-xr-x   63K]  loginctl
│   │   ├── [-rwxr-xr-x   99K]  machinectl
│   │   ├── [-rwxr-xr-x  814K]  networkctl
│   │   ├── [-rwxr-xr-x   19K]  oomctl
│   │   ├── [-rwxr-xr-x  466K]  portablectl
│   │   ├── [-rwxr-xr-x  155K]  resolvectl
│   │   ├── [-rwxr-xr-x  1.4M]  systemctl
│   │   ├── [-rwxr-xr-x   15K]  systemd-ac-power
│   │   ├── [-rwxr-xr-x  203K]  systemd-analyze
│   │   ├── [-rwxr-xr-x   19K]  systemd-ask-password
│   │   ├── [-rwxr-xr-x   19K]  systemd-cat
│   │   ├── [-rwxr-xr-x   23K]  systemd-cgls
│   │   ├── [-rwxr-xr-x   39K]  systemd-cgtop
│   │   ├── [lrwxrwxrwx    14]  systemd-confext ->  systemd-sysext
│   │   ├── [-rwxr-xr-x   43K]  systemd-creds
│   │   ├── [-rwxr-xr-x   67K]  systemd-cryptenroll
│   │   ├── [-rwxr-xr-x   83K]  systemd-cryptsetup
│   │   ├── [-rwxr-xr-x   27K]  systemd-delta
│   │   ├── [-rwxr-xr-x   19K]  systemd-detect-virt
│   │   ├── [-rwxr-xr-x   71K]  systemd-dissect
│   │   ├── [-rwxr-xr-x   19K]  systemd-escape
│   │   ├── [-rwxr-xr-x  155K]  systemd-hwdb
│   │   ├── [-rwxr-xr-x   31K]  systemd-id128
│   │   ├── [-rwxr-xr-x   23K]  systemd-inhibit
│   │   ├── [-rwxr-xr-x   19K]  systemd-machine-id-setup
│   │   ├── [-rwxr-xr-x   51K]  systemd-mount
│   │   ├── [-rwxr-xr-x   27K]  systemd-notify
│   │   ├── [-rwxr-xr-x  352K]  systemd-nspawn
│   │   ├── [-rwxr-xr-x   19K]  systemd-path
│   │   ├── [-rwxr-xr-x  195K]  systemd-repart
│   │   ├── [-rwxr-xr-x  868K]  systemd-repart.standalone
│   │   ├── [lrwxrwxrwx    10]  systemd-resolve ->  resolvectl
│   │   ├── [-rwxr-xr-x   67K]  systemd-run
│   │   ├── [-rwxr-xr-x   27K]  systemd-socket-activate
│   │   ├── [-rwxr-xr-x   19K]  systemd-stdio-bridge
│   │   ├── [-rwxr-xr-x   51K]  systemd-sysext
│   │   ├── [-rwxr-xr-x   67K]  systemd-sysusers
│   │   ├── [-rwxr-xr-x  243K]  systemd-sysusers.standalone
│   │   ├── [-rwxr-xr-x  107K]  systemd-tmpfiles
│   │   ├── [-rwxr-xr-x  327K]  systemd-tmpfiles.standalone
│   │   ├── [-rwxr-xr-x   35K]  systemd-tty-ask-password-agent
│   │   ├── [lrwxrwxrwx    13]  systemd-umount ->  systemd-mount
│   │   ├── [-rwxr-xr-x   43K]  systemd-vmspawn
│   │   ├── [-rwxr-xr-x   47K]  timedatectl
│   │   ├── [-rwxr-xr-x  1.4M]  udevadm
│   │   ├── [-rwxr-xr-x   55K]  ukify
│   │   ├── [-rwxr-xr-x   47K]  userdbctl
│   │   └── [-rwxr-xr-x   31K]  varlinkctl
│   ├── [drwxr-xr-x     4]  include
│   │   ├── [-rw-r--r--  9.6K]  libudev.h
│   │   └── [drwxr-xr-x    16]  systemd
│   │       ├── [-rw-r--r--  3.7K]  _sd-common.h
│   │       ├── [-rw-r--r--  5.8K]  sd-bus-protocol.h
│   │       ├── [-rw-r--r--   23K]  sd-bus-vtable.h
│   │       ├── [-rw-r--r--   30K]  sd-bus.h
│   │       ├── [-rw-r--r--   14K]  sd-daemon.h
│   │       ├── [-rw-r--r--  8.3K]  sd-device.h
│   │       ├── [-rw-r--r--  8.9K]  sd-event.h
│   │       ├── [-rw-r--r--   25K]  sd-gpt.h
│   │       ├── [-rw-r--r--  1.6K]  sd-hwdb.h
│   │       ├── [-rw-r--r--  8.4K]  sd-id128.h
│   │       ├── [-rw-r--r--  8.5K]  sd-journal.h
│   │       ├── [-rw-r--r--   10K]  sd-login.h
│   │       ├── [-rw-r--r--   24K]  sd-messages.h
│   │       └── [-rw-r--r--  4.0K]  sd-path.h
│   ├── [drwxr-xr-x    14]  lib
│   │   ├── [drwxr-xr-x     2]  binfmt.d
│   │   ├── [drwxr-xr-x     2]  credstore
│   │   ├── [drwxr-xr-x     3]  environment.d
│   │   │   └── [lrwxrwxrwx    24]  99-environment.conf ->  ../../../etc/environment
│   │   ├── [drwxr-xr-x     4]  kernel
│   │   │   ├── [-rw-r--r--   407]  install.conf
│   │   │   └── [drwxr-xr-x     6]  install.d
│   │   │       ├── [-rwxr-xr-x  2.0K]  50-depmod.install
│   │   │       ├── [-rwxr-xr-x  8.5K]  60-ukify.install
│   │   │       ├── [-rwxr-xr-x  7.0K]  90-loaderentry.install
│   │   │       └── [-rwxr-xr-x  3.1K]  90-uki-copy.install
│   │   ├── [drwxr-xr-x     4]  modprobe.d
│   │   │   ├── [-rw-r--r--   306]  README
│   │   │   └── [-rw-r--r--   773]  systemd.conf
│   │   ├── [drwxr-xr-x    12]  pcrlock.d
│   │   │   ├── [-rw-r--r--   494]  350-action-efi-application.pcrlock
│   │   │   ├── [drwxr-xr-x     4]  400-secureboot-separator.pcrlock.d
│   │   │   │   ├── [-rw-r--r--   494]  300-0x00000000.pcrlock
│   │   │   │   └── [-rw-r--r--   494]  600-0xffffffff.pcrlock
│   │   │   ├── [drwxr-xr-x     4]  500-separator.pcrlock.d
│   │   │   │   ├── [-rw-r--r--  3.3K]  300-0x00000000.pcrlock
│   │   │   │   └── [-rw-r--r--  3.3K]  600-0xffffffff.pcrlock
│   │   │   ├── [drwxr-xr-x     4]  700-action-efi-exit-boot-services.pcrlock.d
│   │   │   │   ├── [-rw-r--r--   974]  300-present.pcrlock
│   │   │   │   └── [-rw-r--r--    15]  600-absent.pcrlock
│   │   │   ├── [-rw-r--r--   495]  750-enter-initrd.pcrlock
│   │   │   ├── [-rw-r--r--   495]  800-leave-initrd.pcrlock
│   │   │   ├── [-rw-r--r--   495]  850-sysinit.pcrlock
│   │   │   ├── [-rw-r--r--   495]  900-ready.pcrlock
│   │   │   ├── [-rw-r--r--   495]  950-shutdown.pcrlock
│   │   │   └── [-rw-r--r--   495]  990-final.pcrlock
│   │   ├── [drwxr-xr-x     3]  rpm
│   │   │   └── [drwxr-xr-x     3]  macros.d
│   │   │       └── [-rw-r--r--  6.9K]  macros.systemd
│   │   ├── [drwxr-xr-x     6]  sysctl.d
│   │   │   ├── [-rw-r--r--  1.8K]  50-coredump.conf
│   │   │   ├── [-rw-r--r--  1.9K]  50-default.conf
│   │   │   ├── [-rw-r--r--   649]  50-pid-max.conf
│   │   │   └── [-rw-r--r--   387]  README
│   │   ├── [drwxr-xr-x    89]  systemd
│   │   │   ├── [drwxr-xr-x     3]  boot
│   │   │   │   └── [drwxr-xr-x     5]  efi
│   │   │   │       ├── [-rw-r--r--  2.0K]  addonx64.efi.stub
│   │   │   │       ├── [-rw-r--r--   69K]  linuxx64.efi.stub
│   │   │   │       └── [-rw-r--r--   97K]  systemd-bootx64.efi
│   │   │   ├── [drwxr-xr-x    19]  catalog
│   │   │   │   ├── [-rw-r--r--   13K]  systemd.be.catalog
│   │   │   │   ├── [-rw-r--r--   10K]  systemd.be@latin.catalog
│   │   │   │   ├── [-rw-r--r--   29K]  systemd.bg.catalog
│   │   │   │   ├── [-rw-r--r--   28K]  systemd.catalog
│   │   │   │   ├── [-rw-r--r--  8.3K]  systemd.da.catalog
│   │   │   │   ├── [-rw-r--r--   748]  systemd.de.catalog
│   │   │   │   ├── [-rw-r--r--   14K]  systemd.fr.catalog
│   │   │   │   ├── [-rw-r--r--   11K]  systemd.hr.catalog
│   │   │   │   ├── [-rw-r--r--  8.7K]  systemd.hu.catalog
│   │   │   │   ├── [-rw-r--r--   16K]  systemd.it.catalog
│   │   │   │   ├── [-rw-r--r--   12K]  systemd.ko.catalog
│   │   │   │   ├── [-rw-r--r--   25K]  systemd.pl.catalog
│   │   │   │   ├── [-rw-r--r--  8.7K]  systemd.pt_BR.catalog
│   │   │   │   ├── [-rw-r--r--   21K]  systemd.ru.catalog
│   │   │   │   ├── [-rw-r--r--   11K]  systemd.sr.catalog
│   │   │   │   ├── [-rw-r--r--  7.7K]  systemd.zh_CN.catalog
│   │   │   │   └── [-rw-r--r--  7.7K]  systemd.zh_TW.catalog
│   │   │   ├── [-rw-r--r--  9.3K]  import-pubring.gpg
│   │   │   ├── [drwxr-xr-x    14]  network
│   │   │   │   ├── [-rw-r--r--   819]  80-6rd-tunnel.network
│   │   │   │   ├── [-rw-r--r--   719]  80-auto-link-local.network.example
│   │   │   │   ├── [-rw-r--r--   947]  80-container-host0.network
│   │   │   │   ├── [-rw-r--r--   940]  80-container-vb.network
│   │   │   │   ├── [-rw-r--r--  1.0K]  80-container-ve.network
│   │   │   │   ├── [-rw-r--r--  1023]  80-container-vz.network
│   │   │   │   ├── [-rw-r--r--   984]  80-vm-vt.network
│   │   │   │   ├── [-rw-r--r--   730]  80-wifi-adhoc.network
│   │   │   │   ├── [-rw-r--r--   664]  80-wifi-ap.network.example
│   │   │   │   ├── [-rw-r--r--   595]  80-wifi-station.network.example
│   │   │   │   ├── [-rw-r--r--   636]  89-ethernet.network.example
│   │   │   │   └── [-rw-r--r--   769]  99-default.link
│   │   │   ├── [drwxr-xr-x     3]  ntp-units.d
│   │   │   │   └── [-rw-r--r--   116]  80-systemd-timesync.list
│   │   │   ├── [drwxr-xr-x     3]  portable
│   │   │   │   └── [drwxr-xr-x     6]  profile
│   │   │   │       ├── [drwxr-xr-x     3]  default
│   │   │   │       │   └── [-rw-r--r--  1.0K]  service.conf
│   │   │   │       ├── [drwxr-xr-x     3]  nonetwork
│   │   │   │       │   └── [-rw-r--r--   975]  service.conf
│   │   │   │       ├── [drwxr-xr-x     3]  strict
│   │   │   │       │   └── [-rw-r--r--   712]  service.conf
│   │   │   │       └── [drwxr-xr-x     3]  trusted
│   │   │   │           └── [-rw-r--r--   223]  service.conf
│   │   │   ├── [drwxr-xr-x     3]  repart
│   │   │   │   └── [drwxr-xr-x     5]  definitions
│   │   │   │       ├── [drwxr-xr-x     5]  confext.repart.d
│   │   │   │       │   ├── [-rw-r--r--   437]  10-root.conf
│   │   │   │       │   ├── [-rw-r--r--   415]  20-root-verity.conf
│   │   │   │       │   └── [-rw-r--r--   410]  30-root-verity-sig.conf
│   │   │   │       ├── [drwxr-xr-x     5]  portable.repart.d
│   │   │   │       │   ├── [-rw-r--r--   433]  10-root.conf
│   │   │   │       │   ├── [-rw-r--r--   415]  20-root-verity.conf
│   │   │   │       │   └── [-rw-r--r--   410]  30-root-verity-sig.conf
│   │   │   │       └── [drwxr-xr-x     5]  sysext.repart.d
│   │   │   │           ├── [-rw-r--r--   453]  10-root.conf
│   │   │   │           ├── [-rw-r--r--   415]  20-root-verity.conf
│   │   │   │           └── [-rw-r--r--   410]  30-root-verity-sig.conf
│   │   │   ├── [-rw-r--r--   710]  resolv.conf
│   │   │   ├── [drwxr-xr-x   253]  system
│   │   │   │   ├── [lrwxrwxrwx    14]  autovt@.service ->  getty@.service
│   │   │   │   ├── [-rw-r--r--   927]  basic.target
│   │   │   │   ├── [-rw-r--r--   519]  blockdev@.target
│   │   │   │   ├── [-rw-r--r--   435]  bluetooth.target
│   │   │   │   ├── [-rw-r--r--   463]  boot-complete.target
│   │   │   │   ├── [-rw-r--r--  1.1K]  console-getty.service
│   │   │   │   ├── [-rw-r--r--  1.3K]  container-getty@.service
│   │   │   │   ├── [-rw-r--r--   473]  cryptsetup-pre.target
│   │   │   │   ├── [-rw-r--r--   420]  cryptsetup.target
│   │   │   │   ├── [lrwxrwxrwx    13]  ctrl-alt-del.target ->  reboot.target
│   │   │   │   ├── [lrwxrwxrwx    25]  dbus-org.freedesktop.hostname1.service ->  systemd-hostnamed.service
│   │   │   │   ├── [lrwxrwxrwx    23]  dbus-org.freedesktop.import1.service ->  systemd-importd.service
│   │   │   │   ├── [lrwxrwxrwx    23]  dbus-org.freedesktop.locale1.service ->  systemd-localed.service
│   │   │   │   ├── [lrwxrwxrwx    22]  dbus-org.freedesktop.login1.service ->  systemd-logind.service
│   │   │   │   ├── [lrwxrwxrwx    24]  dbus-org.freedesktop.machine1.service ->  systemd-machined.service
│   │   │   │   ├── [lrwxrwxrwx    25]  dbus-org.freedesktop.portable1.service ->  systemd-portabled.service
│   │   │   │   ├── [lrwxrwxrwx    25]  dbus-org.freedesktop.timedate1.service ->  systemd-timedated.service
│   │   │   │   ├── [-rw-r--r--  1.1K]  debug-shell.service
│   │   │   │   ├── [lrwxrwxrwx    16]  default.target ->  graphical.target
│   │   │   │   ├── [-rw-r--r--   775]  dev-hugepages.mount
│   │   │   │   ├── [-rw-r--r--   701]  dev-mqueue.mount
│   │   │   │   ├── [-rw-r--r--   813]  emergency.service
│   │   │   │   ├── [-rw-r--r--   479]  emergency.target
│   │   │   │   ├── [-rw-r--r--   549]  exit.target
│   │   │   │   ├── [-rw-r--r--   410]  factory-reset.target
│   │   │   │   ├── [-rw-r--r--   500]  final.target
│   │   │   │   ├── [-rw-r--r--   461]  first-boot-complete.target
│   │   │   │   ├── [-rw-r--r--   518]  getty-pre.target
│   │   │   │   ├── [-rw-r--r--   509]  getty.target
│   │   │   │   ├── [-rw-r--r--  2.0K]  getty@.service
│   │   │   │   ├── [-rw-r--r--   606]  graphical.target
│   │   │   │   ├── [drwxr-xr-x     3]  graphical.target.wants
│   │   │   │   │   └── [lrwxrwxrwx    39]  systemd-update-utmp-runlevel.service ->  ../systemd-update-utmp-runlevel.service
│   │   │   │   ├── [-rw-r--r--   542]  halt.target
│   │   │   │   ├── [-rw-r--r--   526]  hibernate.target
│   │   │   │   ├── [-rw-r--r--   538]  hybrid-sleep.target
│   │   │   │   ├── [-rw-r--r--   670]  initrd-cleanup.service
│   │   │   │   ├── [-rw-r--r--   598]  initrd-fs.target
│   │   │   │   ├── [-rw-r--r--  1.3K]  initrd-parse-etc.service
│   │   │   │   ├── [-rw-r--r--   566]  initrd-root-device.target
│   │   │   │   ├── [drwxr-xr-x     4]  initrd-root-device.target.wants
│   │   │   │   │   ├── [lrwxrwxrwx    27]  remote-cryptsetup.target ->  ../remote-cryptsetup.target
│   │   │   │   │   └── [lrwxrwxrwx    28]  remote-veritysetup.target ->  ../remote-veritysetup.target
│   │   │   │   ├── [-rw-r--r--   571]  initrd-root-fs.target
│   │   │   │   ├── [drwxr-xr-x     3]  initrd-root-fs.target.wants
│   │   │   │   │   └── [lrwxrwxrwx    25]  systemd-repart.service ->  ../systemd-repart.service
│   │   │   │   ├── [-rw-r--r--   614]  initrd-switch-root.service
│   │   │   │   ├── [-rw-r--r--   779]  initrd-switch-root.target
│   │   │   │   ├── [-rw-r--r--   823]  initrd-udevadm-cleanup-db.service
│   │   │   │   ├── [-rw-r--r--   571]  initrd-usr-fs.target
│   │   │   │   ├── [-rw-r--r--   810]  initrd.target
│   │   │   │   ├── [drwxr-xr-x     4]  initrd.target.wants
│   │   │   │   │   ├── [lrwxrwxrwx    32]  systemd-battery-check.service ->  ../systemd-battery-check.service
│   │   │   │   │   └── [lrwxrwxrwx    34]  systemd-pcrphase-initrd.service ->  ../systemd-pcrphase-initrd.service
│   │   │   │   ├── [-rw-r--r--   487]  integritysetup-pre.target
│   │   │   │   ├── [-rw-r--r--   430]  integritysetup.target
│   │   │   │   ├── [-rw-r--r--   549]  kexec.target
│   │   │   │   ├── [-rw-r--r--   756]  ldconfig.service
│   │   │   │   ├── [-rw-r--r--   453]  local-fs-pre.target
│   │   │   │   ├── [-rw-r--r--   556]  local-fs.target
│   │   │   │   ├── [drwxr-xr-x     3]  local-fs.target.wants
│   │   │   │   │   └── [lrwxrwxrwx    12]  tmp.mount ->  ../tmp.mount
│   │   │   │   ├── [-rw-r--r--   453]  machine.slice
│   │   │   │   ├── [-rw-r--r--   470]  machines.target
│   │   │   │   ├── [drwxr-xr-x     3]  machines.target.wants
│   │   │   │   │   └── [lrwxrwxrwx    25]  var-lib-machines.mount ->  ../var-lib-machines.mount
│   │   │   │   ├── [-rw-r--r--   573]  modprobe@.service
│   │   │   │   ├── [-rw-r--r--   540]  multi-user.target
│   │   │   │   ├── [drwxr-xr-x     6]  multi-user.target.wants
│   │   │   │   │   ├── [lrwxrwxrwx    15]  getty.target ->  ../getty.target
│   │   │   │   │   ├── [lrwxrwxrwx    33]  systemd-ask-password-wall.path ->  ../systemd-ask-password-wall.path
│   │   │   │   │   ├── [lrwxrwxrwx    25]  systemd-logind.service ->  ../systemd-logind.service
│   │   │   │   │   └── [lrwxrwxrwx    39]  systemd-update-utmp-runlevel.service ->  ../systemd-update-utmp-runlevel.service
│   │   │   │   ├── [-rw-r--r--   483]  network-online.target
│   │   │   │   ├── [-rw-r--r--   490]  network-pre.target
│   │   │   │   ├── [-rw-r--r--   499]  network.target
│   │   │   │   ├── [-rw-r--r--   562]  nss-lookup.target
│   │   │   │   ├── [-rw-r--r--   521]  nss-user-lookup.target
│   │   │   │   ├── [-rw-r--r--   407]  paths.target
│   │   │   │   ├── [-rw-r--r--   607]  poweroff.target
│   │   │   │   ├── [-rw-r--r--   433]  printer.target
│   │   │   │   ├── [-rw-r--r--   789]  proc-sys-fs-binfmt_misc.automount
│   │   │   │   ├── [-rw-r--r--   711]  proc-sys-fs-binfmt_misc.mount
│   │   │   │   ├── [-rw-r--r--   626]  quotaon.service
│   │   │   │   ├── [-rw-r--r--   751]  rc-local.service
│   │   │   │   ├── [-rw-r--r--   598]  reboot.target
│   │   │   │   ├── [-rw-r--r--   557]  remote-cryptsetup.target
│   │   │   │   ├── [-rw-r--r--   454]  remote-fs-pre.target
│   │   │   │   ├── [-rw-r--r--   530]  remote-fs.target
│   │   │   │   ├── [drwxr-xr-x     3]  remote-fs.target.wants
│   │   │   │   │   └── [lrwxrwxrwx    25]  var-lib-machines.mount ->  ../var-lib-machines.mount
│   │   │   │   ├── [-rw-r--r--   565]  remote-veritysetup.target
│   │   │   │   ├── [-rw-r--r--   804]  rescue.service
│   │   │   │   ├── [-rw-r--r--   500]  rescue.target
│   │   │   │   ├── [drwxr-xr-x     3]  rescue.target.wants
│   │   │   │   │   └── [lrwxrwxrwx    39]  systemd-update-utmp-runlevel.service ->  ../systemd-update-utmp-runlevel.service
│   │   │   │   ├── [-rw-r--r--   548]  rpcbind.target
│   │   │   │   ├── [lrwxrwxrwx    15]  runlevel0.target ->  poweroff.target
│   │   │   │   ├── [lrwxrwxrwx    13]  runlevel1.target ->  rescue.target
│   │   │   │   ├── [drwxr-xr-x     2]  runlevel1.target.wants
│   │   │   │   ├── [lrwxrwxrwx    17]  runlevel2.target ->  multi-user.target
│   │   │   │   ├── [drwxr-xr-x     2]  runlevel2.target.wants
│   │   │   │   ├── [lrwxrwxrwx    17]  runlevel3.target ->  multi-user.target
│   │   │   │   ├── [drwxr-xr-x     2]  runlevel3.target.wants
│   │   │   │   ├── [lrwxrwxrwx    17]  runlevel4.target ->  multi-user.target
│   │   │   │   ├── [drwxr-xr-x     2]  runlevel4.target.wants
│   │   │   │   ├── [lrwxrwxrwx    16]  runlevel5.target ->  graphical.target
│   │   │   │   ├── [drwxr-xr-x     2]  runlevel5.target.wants
│   │   │   │   ├── [lrwxrwxrwx    13]  runlevel6.target ->  reboot.target
│   │   │   │   ├── [-rw-r--r--  1.5K]  serial-getty@.service
│   │   │   │   ├── [-rw-r--r--   457]  shutdown.target
│   │   │   │   ├── [-rw-r--r--   410]  sigpwr.target
│   │   │   │   ├── [-rw-r--r--   468]  sleep.target
│   │   │   │   ├── [-rw-r--r--   462]  slices.target
│   │   │   │   ├── [-rw-r--r--   428]  smartcard.target
│   │   │   │   ├── [-rw-r--r--   409]  sockets.target
│   │   │   │   ├── [drwxr-xr-x    10]  sockets.target.wants
│   │   │   │   │   ├── [lrwxrwxrwx    26]  systemd-coredump.socket ->  ../systemd-coredump.socket
│   │   │   │   │   ├── [lrwxrwxrwx    25]  systemd-initctl.socket ->  ../systemd-initctl.socket
│   │   │   │   │   ├── [lrwxrwxrwx    34]  systemd-journald-dev-log.socket ->  ../systemd-journald-dev-log.socket
│   │   │   │   │   ├── [lrwxrwxrwx    26]  systemd-journald.socket ->  ../systemd-journald.socket
│   │   │   │   │   ├── [lrwxrwxrwx    27]  systemd-pcrextend.socket ->  ../systemd-pcrextend.socket
│   │   │   │   │   ├── [lrwxrwxrwx    24]  systemd-sysext.socket ->  ../systemd-sysext.socket
│   │   │   │   │   ├── [lrwxrwxrwx    31]  systemd-udevd-control.socket ->  ../systemd-udevd-control.socket
│   │   │   │   │   └── [lrwxrwxrwx    30]  systemd-udevd-kernel.socket ->  ../systemd-udevd-kernel.socket
│   │   │   │   ├── [-rw-r--r--   586]  soft-reboot.target
│   │   │   │   ├── [-rw-r--r--   428]  sound.target
│   │   │   │   ├── [-rw-r--r--   943]  storage-target-mode.target
│   │   │   │   ├── [-rw-r--r--   585]  suspend-then-hibernate.target
│   │   │   │   ├── [-rw-r--r--   511]  suspend.target
│   │   │   │   ├── [-rw-r--r--   402]  swap.target
│   │   │   │   ├── [-rw-r--r--  1.1K]  sys-fs-fuse-connections.mount
│   │   │   │   ├── [-rw-r--r--  1.1K]  sys-kernel-config.mount
│   │   │   │   ├── [-rw-r--r--   730]  sys-kernel-debug.mount
│   │   │   │   ├── [-rw-r--r--   756]  sys-kernel-tracing.mount
│   │   │   │   ├── [-rw-r--r--   574]  sysinit.target
│   │   │   │   ├── [drwxr-xr-x    37]  sysinit.target.wants
│   │   │   │   │   ├── [lrwxrwxrwx    20]  cryptsetup.target -> ../cryptsetup.target
│   │   │   │   │   ├── [lrwxrwxrwx    22]  dev-hugepages.mount -> ../dev-hugepages.mount
│   │   │   │   │   ├── [lrwxrwxrwx    19]  dev-mqueue.mount -> ../dev-mqueue.mount
│   │   │   │   │   ├── [lrwxrwxrwx    24]  integritysetup.target -> ../integritysetup.target
│   │   │   │   │   ├── [lrwxrwxrwx    19]  ldconfig.service -> ../ldconfig.service
│   │   │   │   │   ├── [lrwxrwxrwx    36]  proc-sys-fs-binfmt_misc.automount -> ../proc-sys-fs-binfmt_misc.automount
│   │   │   │   │   ├── [lrwxrwxrwx    32]  sys-fs-fuse-connections.mount -> ../sys-fs-fuse-connections.mount
│   │   │   │   │   ├── [lrwxrwxrwx    26]  sys-kernel-config.mount -> ../sys-kernel-config.mount
│   │   │   │   │   ├── [lrwxrwxrwx    25]  sys-kernel-debug.mount -> ../sys-kernel-debug.mount
│   │   │   │   │   ├── [lrwxrwxrwx    27]  sys-kernel-tracing.mount -> ../sys-kernel-tracing.mount
│   │   │   │   │   ├── [lrwxrwxrwx    36]  systemd-ask-password-console.path -> ../systemd-ask-password-console.path
│   │   │   │   │   ├── [lrwxrwxrwx    25]  systemd-binfmt.service -> ../systemd-binfmt.service
│   │   │   │   │   ├── [lrwxrwxrwx    35]  systemd-boot-random-seed.service -> ../systemd-boot-random-seed.service
│   │   │   │   │   ├── [lrwxrwxrwx    30]  systemd-hwdb-update.service -> ../systemd-hwdb-update.service
│   │   │   │   │   ├── [lrwxrwxrwx    41]  systemd-journal-catalog-update.service -> ../systemd-journal-catalog-update.service
│   │   │   │   │   ├── [lrwxrwxrwx    32]  systemd-journal-flush.service -> ../systemd-journal-flush.service
│   │   │   │   │   ├── [lrwxrwxrwx    27]  systemd-journald.service -> ../systemd-journald.service
│   │   │   │   │   ├── [lrwxrwxrwx    36]  systemd-machine-id-commit.service -> ../systemd-machine-id-commit.service
│   │   │   │   │   ├── [lrwxrwxrwx    29]  systemd-pcrmachine.service -> ../systemd-pcrmachine.service
│   │   │   │   │   ├── [lrwxrwxrwx    35]  systemd-pcrphase-sysinit.service -> ../systemd-pcrphase-sysinit.service
│   │   │   │   │   ├── [lrwxrwxrwx    27]  systemd-pcrphase.service -> ../systemd-pcrphase.service
│   │   │   │   │   ├── [lrwxrwxrwx    30]  systemd-random-seed.service -> ../systemd-random-seed.service
│   │   │   │   │   ├── [lrwxrwxrwx    25]  systemd-repart.service -> ../systemd-repart.service
│   │   │   │   │   ├── [lrwxrwxrwx    25]  systemd-sysctl.service -> ../systemd-sysctl.service
│   │   │   │   │   ├── [lrwxrwxrwx    27]  systemd-sysusers.service -> ../systemd-sysusers.service
│   │   │   │   │   ├── [lrwxrwxrwx    43]  systemd-tmpfiles-setup-dev-early.service -> ../systemd-tmpfiles-setup-dev-early.service
│   │   │   │   │   ├── [lrwxrwxrwx    37]  systemd-tmpfiles-setup-dev.service -> ../systemd-tmpfiles-setup-dev.service
│   │   │   │   │   ├── [lrwxrwxrwx    33]  systemd-tmpfiles-setup.service -> ../systemd-tmpfiles-setup.service
│   │   │   │   │   ├── [lrwxrwxrwx    35]  systemd-tpm2-setup-early.service -> ../systemd-tpm2-setup-early.service
│   │   │   │   │   ├── [lrwxrwxrwx    29]  systemd-tpm2-setup.service -> ../systemd-tpm2-setup.service
│   │   │   │   │   ├── [lrwxrwxrwx    31]  systemd-udev-trigger.service -> ../systemd-udev-trigger.service
│   │   │   │   │   ├── [lrwxrwxrwx    24]  systemd-udevd.service -> ../systemd-udevd.service
│   │   │   │   │   ├── [lrwxrwxrwx    30]  systemd-update-done.service -> ../systemd-update-done.service
│   │   │   │   │   ├── [lrwxrwxrwx    30]  systemd-update-utmp.service -> ../systemd-update-utmp.service
│   │   │   │   │   └── [lrwxrwxrwx    21]  veritysetup.target -> ../veritysetup.target
│   │   │   │   ├── [-rw-r--r--  1.4K]  syslog.socket
│   │   │   │   ├── [-rw-r--r--   468]  system-systemd\x2dcryptsetup.slice
│   │   │   │   ├── [-rw-r--r--   463]  system-systemd\x2dveritysetup.slice
│   │   │   │   ├── [-rw-r--r--  1.5K]  system-update-cleanup.service
│   │   │   │   ├── [-rw-r--r--   551]  system-update-pre.target
│   │   │   │   ├── [-rw-r--r--   625]  system-update.target
│   │   │   │   ├── [-rw-r--r--   771]  systemd-ask-password-console.path
│   │   │   │   ├── [-rw-r--r--   834]  systemd-ask-password-console.service
│   │   │   │   ├── [-rw-r--r--   695]  systemd-ask-password-wall.path
│   │   │   │   ├── [-rw-r--r--   747]  systemd-ask-password-wall.service
│   │   │   │   ├── [-rw-r--r--   777]  systemd-backlight@.service
│   │   │   │   ├── [-rw-r--r--   856]  systemd-battery-check.service
│   │   │   │   ├── [-rw-r--r--  1.2K]  systemd-binfmt.service
│   │   │   │   ├── [-rw-r--r--   690]  systemd-bless-boot.service
│   │   │   │   ├── [-rw-r--r--   730]  systemd-boot-check-no-failures.service
│   │   │   │   ├── [-rw-r--r--  1.0K]  systemd-boot-random-seed.service
│   │   │   │   ├── [-rw-r--r--   733]  systemd-boot-update.service
│   │   │   │   ├── [-rw-r--r--  1.0K]  systemd-confext.service
│   │   │   │   ├── [-rw-r--r--   617]  systemd-coredump.socket
│   │   │   │   ├── [-rw-r--r--  1.1K]  systemd-coredump@.service
│   │   │   │   ├── [-rw-r--r--   564]  systemd-exit.service
│   │   │   │   ├── [-rw-r--r--   724]  systemd-fsck-root.service
│   │   │   │   ├── [-rw-r--r--   712]  systemd-fsck@.service
│   │   │   │   ├── [-rw-r--r--   667]  systemd-growfs-root.service
│   │   │   │   ├── [-rw-r--r--   664]  systemd-growfs@.service
│   │   │   │   ├── [-rw-r--r--   562]  systemd-halt.service
│   │   │   │   ├── [-rw-r--r--   666]  systemd-hibernate-resume.service
│   │   │   │   ├── [-rw-r--r--   555]  systemd-hibernate.service
│   │   │   │   ├── [-rw-r--r--   645]  systemd-homed-activate.service
│   │   │   │   ├── [-rw-r--r--  1.4K]  systemd-homed.service
│   │   │   │   ├── [-rw-r--r--  1.2K]  systemd-hostnamed.service
│   │   │   │   ├── [-rw-r--r--   834]  systemd-hwdb-update.service
│   │   │   │   ├── [-rw-r--r--   576]  systemd-hybrid-sleep.service
│   │   │   │   ├── [-rw-r--r--  1.0K]  systemd-importd.service
│   │   │   │   ├── [-rw-r--r--   578]  systemd-initctl.service
│   │   │   │   ├── [-rw-r--r--   553]  systemd-initctl.socket
│   │   │   │   ├── [-rw-r--r--   750]  systemd-journal-catalog-update.service
│   │   │   │   ├── [-rw-r--r--   827]  systemd-journal-flush.service
│   │   │   │   ├── [-rw-r--r--  1.1K]  systemd-journal-gatewayd.service
│   │   │   │   ├── [-rw-r--r--   500]  systemd-journal-gatewayd.socket
│   │   │   │   ├── [-rw-r--r--  1.3K]  systemd-journal-remote.service
│   │   │   │   ├── [-rw-r--r--   450]  systemd-journal-remote.socket
│   │   │   │   ├── [-rw-r--r--  1.2K]  systemd-journal-upload.service
│   │   │   │   ├── [-rw-r--r--   724]  systemd-journald-audit.socket
│   │   │   │   ├── [-rw-r--r--  1.2K]  systemd-journald-dev-log.socket
│   │   │   │   ├── [-rw-r--r--   605]  systemd-journald-varlink@.socket
│   │   │   │   ├── [-rw-r--r--  2.4K]  systemd-journald.service
│   │   │   │   ├── [-rw-r--r--   934]  systemd-journald.socket
│   │   │   │   ├── [-rw-r--r--  1.6K]  systemd-journald@.service
│   │   │   │   ├── [-rw-r--r--   746]  systemd-journald@.socket
│   │   │   │   ├── [-rw-r--r--   569]  systemd-kexec.service
│   │   │   │   ├── [-rw-r--r--  1.2K]  systemd-localed.service
│   │   │   │   ├── [-rw-r--r--  2.0K]  systemd-logind.service
│   │   │   │   ├── [-rw-r--r--   748]  systemd-machine-id-commit.service
│   │   │   │   ├── [-rw-r--r--  1.3K]  systemd-machined.service
│   │   │   │   ├── [-rw-r--r--   792]  systemd-network-generator.service
│   │   │   │   ├── [-rw-r--r--   785]  systemd-networkd-wait-online.service
│   │   │   │   ├── [-rw-r--r--   804]  systemd-networkd-wait-online@.service
│   │   │   │   ├── [-rw-r--r--  2.4K]  systemd-networkd.service
│   │   │   │   ├── [-rw-r--r--   682]  systemd-networkd.socket
│   │   │   │   ├── [-rw-r--r--  1.6K]  systemd-nspawn@.service
│   │   │   │   ├── [-rw-r--r--  1.7K]  systemd-oomd.service
│   │   │   │   ├── [-rw-r--r--   838]  systemd-oomd.socket
│   │   │   │   ├── [-rw-r--r--   649]  systemd-pcrextend.socket
│   │   │   │   ├── [-rw-r--r--   650]  systemd-pcrextend@.service
│   │   │   │   ├── [-rw-r--r--   738]  systemd-pcrfs-root.service
│   │   │   │   ├── [-rw-r--r--   762]  systemd-pcrfs@.service
│   │   │   │   ├── [-rw-r--r--   767]  systemd-pcrlock-file-system.service
│   │   │   │   ├── [-rw-r--r--   803]  systemd-pcrlock-firmware-code.service
│   │   │   │   ├── [-rw-r--r--   814]  systemd-pcrlock-firmware-config.service
│   │   │   │   ├── [-rw-r--r--   764]  systemd-pcrlock-machine-id.service
│   │   │   │   ├── [-rw-r--r--   758]  systemd-pcrlock-make-policy.service
│   │   │   │   ├── [-rw-r--r--   822]  systemd-pcrlock-secureboot-authority.service
│   │   │   │   ├── [-rw-r--r--   816]  systemd-pcrlock-secureboot-policy.service
│   │   │   │   ├── [-rw-r--r--   711]  systemd-pcrmachine.service
│   │   │   │   ├── [-rw-r--r--   892]  systemd-pcrphase-initrd.service
│   │   │   │   ├── [-rw-r--r--   794]  systemd-pcrphase-sysinit.service
│   │   │   │   ├── [-rw-r--r--   756]  systemd-pcrphase.service
│   │   │   │   ├── [-rw-r--r--  1.0K]  systemd-portabled.service
│   │   │   │   ├── [-rw-r--r--   575]  systemd-poweroff.service
│   │   │   │   ├── [-rw-r--r--   815]  systemd-pstore.service
│   │   │   │   ├── [-rw-r--r--   683]  systemd-quotacheck.service
│   │   │   │   ├── [-rw-r--r--  1.2K]  systemd-random-seed.service
│   │   │   │   ├── [-rw-r--r--   568]  systemd-reboot.service
│   │   │   │   ├── [-rw-r--r--   787]  systemd-remount-fs.service
│   │   │   │   ├── [-rw-r--r--  1.3K]  systemd-repart.service
│   │   │   │   ├── [-rw-r--r--  1.8K]  systemd-resolved.service
│   │   │   │   ├── [-rw-r--r--   771]  systemd-rfkill.service
│   │   │   │   ├── [-rw-r--r--   776]  systemd-rfkill.socket
│   │   │   │   ├── [-rw-r--r--   588]  systemd-soft-reboot.service
│   │   │   │   ├── [-rw-r--r--   920]  systemd-storagetm.service
│   │   │   │   ├── [-rw-r--r--   623]  systemd-suspend-then-hibernate.service
│   │   │   │   ├── [-rw-r--r--   556]  systemd-suspend.service
│   │   │   │   ├── [-rw-r--r--   731]  systemd-sysctl.service
│   │   │   │   ├── [-rw-r--r--  1.0K]  systemd-sysext.service
│   │   │   │   ├── [-rw-r--r--   683]  systemd-sysext.socket
│   │   │   │   ├── [-rw-r--r--   657]  systemd-sysext@.service
│   │   │   │   ├── [-rw-r--r--  1.3K]  systemd-sysusers.service
│   │   │   │   ├── [-rw-r--r--  1.2K]  systemd-time-wait-sync.service
│   │   │   │   ├── [-rw-r--r--  1.1K]  systemd-timedated.service
│   │   │   │   ├── [-rw-r--r--  1.7K]  systemd-timesyncd.service
│   │   │   │   ├── [-rw-r--r--   747]  systemd-tmpfiles-clean.service
│   │   │   │   ├── [-rw-r--r--   539]  systemd-tmpfiles-clean.timer
│   │   │   │   ├── [-rw-r--r--   852]  systemd-tmpfiles-setup-dev-early.service
│   │   │   │   ├── [-rw-r--r--   877]  systemd-tmpfiles-setup-dev.service
│   │   │   │   ├── [-rw-r--r--  1005]  systemd-tmpfiles-setup.service
│   │   │   │   ├── [-rw-r--r--   708]  systemd-tpm2-setup-early.service
│   │   │   │   ├── [-rw-r--r--   796]  systemd-tpm2-setup.service
│   │   │   │   ├── [-rw-r--r--   863]  systemd-udev-settle.service
│   │   │   │   ├── [-rw-r--r--   758]  systemd-udev-trigger.service
│   │   │   │   ├── [-rw-r--r--   650]  systemd-udevd-control.socket
│   │   │   │   ├── [-rw-r--r--   624]  systemd-udevd-kernel.socket
│   │   │   │   ├── [-rw-r--r--  1.3K]  systemd-udevd.service
│   │   │   │   ├── [-rw-r--r--   682]  systemd-update-done.service
│   │   │   │   ├── [-rw-r--r--   849]  systemd-update-utmp-runlevel.service
│   │   │   │   ├── [-rw-r--r--   856]  systemd-update-utmp.service
│   │   │   │   ├── [-rw-r--r--  1.2K]  systemd-userdbd.service
│   │   │   │   ├── [-rw-r--r--   691]  systemd-userdbd.socket
│   │   │   │   ├── [-rw-r--r--  1.2K]  systemd-vconsole-setup.service
│   │   │   │   ├── [-rw-r--r--   743]  systemd-volatile-root.service
│   │   │   │   ├── [-rw-r--r--   434]  time-set.target
│   │   │   │   ├── [-rw-r--r--   487]  time-sync.target
│   │   │   │   ├── [-rw-r--r--   458]  timers.target
│   │   │   │   ├── [drwxr-xr-x     3]  timers.target.wants
│   │   │   │   │   └── [lrwxrwxrwx    31]  systemd-tmpfiles-clean.timer -> ../systemd-tmpfiles-clean.timer
│   │   │   │   ├── [-rw-r--r--   798]  tmp.mount
│   │   │   │   ├── [-rw-r--r--   465]  umount.target
│   │   │   │   ├── [-rw-r--r--   426]  usb-gadget.target
│   │   │   │   ├── [drwxr-xr-x     3]  user-.slice.d
│   │   │   │   │   └── [-rw-r--r--   458]  10-defaults.conf
│   │   │   │   ├── [-rw-r--r--   674]  user-runtime-dir@.service
│   │   │   │   ├── [-rw-r--r--   440]  user.slice
│   │   │   │   ├── [-rw-r--r--   833]  user@.service
│   │   │   │   ├── [drwxr-xr-x     3]  user@.service.d
│   │   │   │   │   └── [-rw-r--r--   605]  10-login-barrier.conf
│   │   │   │   ├── [drwxr-xr-x     3]  user@0.service.d
│   │   │   │   │   └── [-rw-r--r--   548]  10-login-barrier.conf
│   │   │   │   ├── [-rw-r--r--   807]  var-lib-machines.mount
│   │   │   │   ├── [-rw-r--r--   481]  veritysetup-pre.target
│   │   │   │   └── [-rw-r--r--   427]  veritysetup.target
│   │   │   ├── [drwxr-xr-x    15]  system-generators
│   │   │   │   ├── [-rwxr-xr-x   75K]  systemd-bless-boot-generator
│   │   │   │   ├── [-rwxr-xr-x   35K]  systemd-cryptsetup-generator
│   │   │   │   ├── [-rwxr-xr-x   15K]  systemd-debug-generator
│   │   │   │   ├── [-rwxr-xr-x   51K]  systemd-fstab-generator
│   │   │   │   ├── [-rwxr-xr-x   23K]  systemd-getty-generator
│   │   │   │   ├── [-rwxr-xr-x   35K]  systemd-gpt-auto-generator
│   │   │   │   ├── [-rwxr-xr-x   27K]  systemd-hibernate-resume-generator
│   │   │   │   ├── [-rwxr-xr-x   23K]  systemd-integritysetup-generator
│   │   │   │   ├── [-rwxr-xr-x   15K]  systemd-rc-local-generator
│   │   │   │   ├── [-rwxr-xr-x   15K]  systemd-run-generator
│   │   │   │   ├── [-rwxr-xr-x   15K]  systemd-system-update-generator
│   │   │   │   ├── [-rwxr-xr-x   31K]  systemd-sysv-generator
│   │   │   │   └── [-rwxr-xr-x   31K]  systemd-veritysetup-generator
│   │   │   ├── [drwxr-xr-x     3]  system-preset
│   │   │   │   └── [-rw-r--r--  1.4K]  90-systemd.preset
│   │   │   ├── [drwxr-xr-x     2]  system-shutdown
│   │   │   ├── [drwxr-xr-x     2]  system-sleep
│   │   │   ├── [-rwxr-xr-x   95K]  systemd
│   │   │   ├── [-rwxr-xr-x   35K]  systemd-backlight
│   │   │   ├── [-rwxr-xr-x   19K]  systemd-battery-check
│   │   │   ├── [-rwxr-xr-x   23K]  systemd-binfmt
│   │   │   ├── [-rwxr-xr-x  167K]  systemd-bless-boot
│   │   │   ├── [-rwxr-xr-x   15K]  systemd-boot-check-no-failures
│   │   │   ├── [-rwxr-xr-x   15K]  systemd-cgroups-agent
│   │   │   ├── [-rwxr-xr-x   83K]  systemd-coredump
│   │   │   ├── [lrwxrwxrwx    28]  systemd-cryptsetup ->  ../../bin/systemd-cryptsetup
│   │   │   ├── [-rwxr-xr-x  127K]  systemd-executor
│   │   │   ├── [-rwxr-xr-x   39K]  systemd-export
│   │   │   ├── [-rwxr-xr-x   27K]  systemd-fsck
│   │   │   ├── [-rwxr-xr-x   23K]  systemd-growfs
│   │   │   ├── [-rwxr-xr-x   27K]  systemd-hibernate-resume
│   │   │   ├── [-rwxr-xr-x  207K]  systemd-homed
│   │   │   ├── [-rwxr-xr-x  199K]  systemd-homework
│   │   │   ├── [-rwxr-xr-x   47K]  systemd-hostnamed
│   │   │   ├── [-rwxr-xr-x   51K]  systemd-import
│   │   │   ├── [-rwxr-xr-x   31K]  systemd-import-fs
│   │   │   ├── [-rwxr-xr-x   43K]  systemd-importd
│   │   │   ├── [-rwxr-xr-x   23K]  systemd-initctl
│   │   │   ├── [-rwxr-xr-x   23K]  systemd-integritysetup
│   │   │   ├── [-rwxr-xr-x   43K]  systemd-journal-gatewayd
│   │   │   ├── [-rwxr-xr-x   63K]  systemd-journal-remote
│   │   │   ├── [-rwxr-xr-x   43K]  systemd-journal-upload
│   │   │   ├── [-rwxr-xr-x  189K]  systemd-journald
│   │   │   ├── [-rwxr-xr-x   47K]  systemd-localed
│   │   │   ├── [-rwxr-xr-x  279K]  systemd-logind
│   │   │   ├── [-rwxr-xr-x  127K]  systemd-machined
│   │   │   ├── [-rwxr-xr-x   15K]  systemd-makefs
│   │   │   ├── [-rwxr-xr-x   47K]  systemd-measure
│   │   │   ├── [-rwxr-xr-x  135K]  systemd-network-generator
│   │   │   ├── [-rwxr-xr-x  2.5M]  systemd-networkd
│   │   │   ├── [-rwxr-xr-x  239K]  systemd-networkd-wait-online
│   │   │   ├── [-rwxr-xr-x   59K]  systemd-oomd
│   │   │   ├── [-rwxr-xr-x   27K]  systemd-pcrextend
│   │   │   ├── [-rwxr-xr-x  135K]  systemd-pcrlock
│   │   │   ├── [-rwxr-xr-x  762K]  systemd-portabled
│   │   │   ├── [-rwxr-xr-x   23K]  systemd-pstore
│   │   │   ├── [-rwxr-xr-x  103K]  systemd-pull
│   │   │   ├── [-rwxr-xr-x   15K]  systemd-quotacheck
│   │   │   ├── [-rwxr-xr-x   27K]  systemd-random-seed
│   │   │   ├── [-rwxr-xr-x   19K]  systemd-remount-fs
│   │   │   ├── [-rwxr-xr-x   15K]  systemd-reply-password
│   │   │   ├── [-rwxr-xr-x  519K]  systemd-resolved
│   │   │   ├── [-rwxr-xr-x   23K]  systemd-rfkill
│   │   │   ├── [-rwxr-xr-x   55K]  systemd-shutdown
│   │   │   ├── [-rwxr-xr-x  327K]  systemd-shutdown.standalone
│   │   │   ├── [-rwxr-xr-x   47K]  systemd-sleep
│   │   │   ├── [-rwxr-xr-x   31K]  systemd-socket-proxyd
│   │   │   ├── [-rwxr-xr-x   51K]  systemd-storagetm
│   │   │   ├── [-rwxr-xr-x   19K]  systemd-sulogin-shell
│   │   │   ├── [-rwxr-xr-x   23K]  systemd-sysctl
│   │   │   ├── [lrwxrwxrwx    41]  systemd-sysroot-fstab-check -> system-generators/systemd-fstab-generator
│   │   │   ├── [-rwxr-xr-x   91K]  systemd-time-wait-sync
│   │   │   ├── [-rwxr-xr-x   43K]  systemd-timedated
│   │   │   ├── [-rwxr-xr-x  506K]  systemd-timesyncd
│   │   │   ├── [-rwxr-xr-x   27K]  systemd-tpm2-setup
│   │   │   ├── [lrwxrwxrwx    17]  systemd-udevd -> ../../bin/udevadm
│   │   │   ├── [-rwxr-xr-x   15K]  systemd-update-done
│   │   │   ├── [-rwxr-xr-x  3.8K]  systemd-update-helper
│   │   │   ├── [-rwxr-xr-x   19K]  systemd-update-utmp
│   │   │   ├── [-rwxr-xr-x   23K]  systemd-user-runtime-dir
│   │   │   ├── [-rwxr-xr-x   27K]  systemd-userdbd
│   │   │   ├── [-rwxr-xr-x   31K]  systemd-userwork
│   │   │   ├── [-rwxr-xr-x   27K]  systemd-vconsole-setup
│   │   │   ├── [-rwxr-xr-x   27K]  systemd-veritysetup
│   │   │   ├── [-rwxr-xr-x   23K]  systemd-volatile-root
│   │   │   ├── [-rwxr-xr-x   15K]  systemd-xdg-autostart-condition
│   │   │   ├── [lrwxrwxrwx    15]  ukify ->  ../../bin/ukify
│   │   │   ├── [drwxr-xr-x    23]  user
│   │   │   │   ├── [-rw-r--r--   442]  app.slice
│   │   │   │   ├── [-rw-r--r--   446]  background.slice
│   │   │   │   ├── [-rw-r--r--   505]  basic.target
│   │   │   │   ├── [-rw-r--r--   427]  bluetooth.target
│   │   │   │   ├── [-rw-r--r--   471]  default.target
│   │   │   │   ├── [-rw-r--r--   510]  exit.target
│   │   │   │   ├── [-rw-r--r--   576]  graphical-session-pre.target
│   │   │   │   ├── [-rw-r--r--   492]  graphical-session.target
│   │   │   │   ├── [-rw-r--r--   402]  paths.target
│   │   │   │   ├── [-rw-r--r--   425]  printer.target
│   │   │   │   ├── [-rw-r--r--   443]  session.slice
│   │   │   │   ├── [-rw-r--r--   450]  shutdown.target
│   │   │   │   ├── [-rw-r--r--   428]  smartcard.target
│   │   │   │   ├── [-rw-r--r--   404]  sockets.target
│   │   │   │   ├── [-rw-r--r--   428]  sound.target
│   │   │   │   ├── [-rw-r--r--   598]  systemd-exit.service
│   │   │   │   ├── [-rw-r--r--   688]  systemd-tmpfiles-clean.service
│   │   │   │   ├── [-rw-r--r--   541]  systemd-tmpfiles-clean.timer
│   │   │   │   ├── [-rw-r--r--   728]  systemd-tmpfiles-setup.service
│   │   │   │   ├── [-rw-r--r--   453]  timers.target
│   │   │   │   └── [-rw-r--r--   477]  xdg-desktop-autostart.target
│   │   │   ├── [drwxr-xr-x     3]  user-environment-generators
│   │   │   │   └── [-rwxr-xr-x   15K]  30-systemd-environment-d-generator
│   │   │   ├── [drwxr-xr-x     3]  user-generators
│   │   │   │   └── [-rwxr-xr-x   31K]  systemd-xdg-autostart-generator
│   │   │   └── [drwxr-xr-x     3]  user-preset
│   │   │       └── [-rw-r--r--   580]  90-systemd.preset
│   │   ├── [drwxr-xr-x    11]  sysusers.d
│   │   │   ├── [-rw-r--r--   359]  README
│   │   │   ├── [-rw-r--r--  1.3K]  basic.conf
│   │   │   ├── [-rw-r--r--   335]  systemd-coredump.conf
│   │   │   ├── [-rw-r--r--   314]  systemd-journal.conf
│   │   │   ├── [-rw-r--r--   341]  systemd-network.conf
│   │   │   ├── [-rw-r--r--   339]  systemd-oom.conf
│   │   │   ├── [-rw-r--r--   398]  systemd-remote.conf
│   │   │   ├── [-rw-r--r--   331]  systemd-resolve.conf
│   │   │   └── [-rw-r--r--   344]  systemd-timesync.conf
│   │   ├── [drwxr-xr-x    20]  tmpfiles.d
│   │   │   ├── [-rw-r--r--   400]  README
│   │   │   ├── [-rw-r--r--   473]  credstore.conf
│   │   │   ├── [-rw-r--r--   524]  etc.conf
│   │   │   ├── [-rw-r--r--   362]  home.conf
│   │   │   ├── [-rw-r--r--  1.1K]  journal-nocow.conf
│   │   │   ├── [-rw-r--r--   841]  legacy.conf
│   │   │   ├── [-rw-r--r--   104]  portables.conf
│   │   │   ├── [-rw-r--r--   851]  provision.conf
│   │   │   ├── [-rw-r--r--   798]  static-nodes-permissions.conf
│   │   │   ├── [-rw-r--r--   583]  systemd-network.conf
│   │   │   ├── [-rw-r--r--   976]  systemd-nspawn.conf
│   │   │   ├── [-rw-r--r--  1.5K]  systemd-pstore.conf
│   │   │   ├── [-rw-r--r--   393]  systemd-resolve.conf
│   │   │   ├── [-rw-r--r--   823]  systemd-tmp.conf
│   │   │   ├── [-rw-r--r--  1.7K]  systemd.conf
│   │   │   ├── [-rw-r--r--   449]  tmp.conf
│   │   │   ├── [-rw-r--r--   568]  var.conf
│   │   │   └── [-rw-r--r--   617]  x11.conf
│   │   └── [drwxr-xr-x    12]  udev
│   │       ├── [-rwxr-xr-x   79K]  ata_id
│   │       ├── [-rwxr-xr-x   91K]  cdrom_id
│   │       ├── [-rwxr-xr-x   87K]  dmi_memory_id
│   │       ├── [-rwxr-xr-x  135K]  fido_id
│   │       ├── [drwxr-xr-x    33]  hwdb.d
│   │       │   ├── [-rw-r--r--  2.5M]  20-OUI.hwdb
│   │       │   ├── [-rw-r--r--  151K]  20-acpi-vendor.hwdb
│   │       │   ├── [-rw-r--r--  137K]  20-bluetooth-vendor-product.hwdb
│   │       │   ├── [-rw-r--r--   832]  20-dmi-id.hwdb
│   │       │   ├── [-rw-r--r--   111]  20-net-ifname.hwdb
│   │       │   ├── [-rw-r--r--   16K]  20-pci-classes.hwdb
│   │       │   ├── [-rw-r--r--  3.5M]  20-pci-vendor-model.hwdb
│   │       │   ├── [-rw-r--r--   783]  20-sdio-classes.hwdb
│   │       │   ├── [-rw-r--r--  4.1K]  20-sdio-vendor-model.hwdb
│   │       │   ├── [-rw-r--r--  8.8K]  20-usb-classes.hwdb
│   │       │   ├── [-rw-r--r--  1.4M]  20-usb-vendor-model.hwdb
│   │       │   ├── [-rw-r--r--  1.8K]  20-vmbus-class.hwdb
│   │       │   ├── [-rw-r--r--  2.7K]  60-autosuspend-chromiumos.hwdb
│   │       │   ├── [-rw-r--r--  6.7K]  60-autosuspend-fingerprint-reader.hwdb
│   │       │   ├── [-rw-r--r--  2.6K]  60-autosuspend.hwdb
│   │       │   ├── [-rw-r--r--   25K]  60-evdev.hwdb
│   │       │   ├── [-rw-r--r--  2.5K]  60-input-id.hwdb
│   │       │   ├── [-rw-r--r--   97K]  60-keyboard.hwdb
│   │       │   ├── [-rw-r--r--  1.1K]  60-seat.hwdb
│   │       │   ├── [-rw-r--r--   45K]  60-sensor.hwdb
│   │       │   ├── [-rw-r--r--  1.2K]  70-analyzers.hwdb
│   │       │   ├── [-rw-r--r--  2.9K]  70-av-production.hwdb
│   │       │   ├── [-rw-r--r--   679]  70-cameras.hwdb
│   │       │   ├── [-rw-r--r--  1.5K]  70-joystick.hwdb
│   │       │   ├── [-rw-r--r--   25K]  70-mouse.hwdb
│   │       │   ├── [-rw-r--r--   926]  70-pda.hwdb
│   │       │   ├── [-rw-r--r--  6.1K]  70-pointingstick.hwdb
│   │       │   ├── [-rw-r--r--   700]  70-sound-card.hwdb
│   │       │   ├── [-rw-r--r--  2.0K]  70-touchpad.hwdb
│   │       │   ├── [-rw-r--r--   49K]  80-ieee1394-unit-function.hwdb
│   │       │   └── [-rw-r--r--   518]  README
│   │       ├── [-rwxr-xr-x  151K]  iocost
│   │       ├── [-rwxr-xr-x   35K]  mtd_probe
│   │       ├── [drwxr-xr-x    38]  rules.d
│   │       │   ├── [-rw-r--r--  5.2K]  50-udev-default.rules
│   │       │   ├── [-rw-r--r--   704]  60-autosuspend.rules
│   │       │   ├── [-rw-r--r--   703]  60-block.rules
│   │       │   ├── [-rw-r--r--  1.0K]  60-cdrom_id.rules
│   │       │   ├── [-rw-r--r--   637]  60-dmi-id.rules
│   │       │   ├── [-rw-r--r--   834]  60-drm.rules
│   │       │   ├── [-rw-r--r--  1.1K]  60-evdev.rules
│   │       │   ├── [-rw-r--r--   491]  60-fido-id.rules
│   │       │   ├── [-rw-r--r--   379]  60-infiniband.rules
│   │       │   ├── [-rw-r--r--   282]  60-input-id.rules
│   │       │   ├── [-rw-r--r--   727]  60-persistent-alsa.rules
│   │       │   ├── [-rw-r--r--  3.2K]  60-persistent-input.rules
│   │       │   ├── [-rw-r--r--   411]  60-persistent-storage-mtd.rules
│   │       │   ├── [-rw-r--r--  2.5K]  60-persistent-storage-tape.rules
│   │       │   ├── [-rw-r--r--  9.2K]  60-persistent-storage.rules
│   │       │   ├── [-rw-r--r--  1.1K]  60-persistent-v4l.rules
│   │       │   ├── [-rw-r--r--  1.6K]  60-sensor.rules
│   │       │   ├── [-rw-r--r--  1.4K]  60-serial.rules
│   │       │   ├── [-rw-r--r--   616]  64-btrfs.rules
│   │       │   ├── [-rw-r--r--   280]  70-camera.rules
│   │       │   ├── [-rw-r--r--   432]  70-joystick.rules
│   │       │   ├── [-rw-r--r--   184]  70-memory.rules
│   │       │   ├── [-rw-r--r--   734]  70-mouse.rules
│   │       │   ├── [-rw-r--r--   576]  70-power-switch.rules
│   │       │   ├── [-rw-r--r--   473]  70-touchpad.rules
│   │       │   ├── [-rw-r--r--  3.7K]  71-seat.rules
│   │       │   ├── [-rw-r--r--   587]  73-seat-late.rules
│   │       │   ├── [-rw-r--r--   452]  75-net-description.rules
│   │       │   ├── [-rw-r--r--   174]  75-probe_mtd.rules
│   │       │   ├── [-rw-r--r--  4.7K]  78-sound-card.rules
│   │       │   ├── [-rw-r--r--   295]  80-net-setup-link.rules
│   │       │   ├── [-rw-r--r--   528]  81-net-dhcp.rules
│   │       │   ├── [-rw-r--r--   769]  90-iocost.rules
│   │       │   ├── [-rw-r--r--   518]  90-vconsole.rules
│   │       │   ├── [-rw-r--r--  5.0K]  99-systemd.rules
│   │       │   └── [-rw-r--r--   435]  README
│   │       ├── [-rwxr-xr-x   92K]  scsi_id
│   │       └── [-rwxr-xr-x   35K]  v4l_id
│   ├── [drwxr-xr-x    15]  lib64
│   │   ├── [drwxr-xr-x     5]  cryptsetup
│   │   │   ├── [-rwxr-xr-x   18K]  libcryptsetup-token-systemd-fido2.so
│   │   │   ├── [-rwxr-xr-x   18K]  libcryptsetup-token-systemd-pkcs11.so
│   │   │   └── [-rwxr-xr-x   22K]  libcryptsetup-token-systemd-tpm2.so
│   │   ├── [-rwxr-xr-x  159K]  libnss_myhostname.so.2
│   │   ├── [-rwxr-xr-x  341K]  libnss_mymachines.so.2
│   │   ├── [-rwxr-xr-x  167K]  libnss_resolve.so.2
│   │   ├── [-rwxr-xr-x  367K]  libnss_systemd.so.2
│   │   ├── [lrwxrwxrwx    15]  libsystemd.so ->  libsystemd.so.0
│   │   ├── [lrwxrwxrwx    20]  libsystemd.so.0 ->  libsystemd.so.0.38.0
│   │   ├── [-rwxr-xr-x  902K]  libsystemd.so.0.38.0
│   │   ├── [lrwxrwxrwx    12]  libudev.so ->  libudev.so.1
│   │   ├── [lrwxrwxrwx    16]  libudev.so.1 ->  libudev.so.1.7.8
│   │   ├── [-rwxr-xr-x  199K]  libudev.so.1.7.8
│   │   ├── [drwxr-xr-x     4]  pkgconfig
│   │   │   ├── [-rw-r--r--   545]  libsystemd.pc
│   │   │   └── [-rw-r--r--   571]  libudev.pc
│   │   └── [drwxr-xr-x     4]  systemd
│   │       ├── [-rwxr-xr-x  2.1M]  libsystemd-core-255.so
│   │       └── [-rwxr-xr-x  3.7M]  libsystemd-shared-255.so
│   └── [drwxr-xr-x    11]  sbin
│       ├── [lrwxrwxrwx    16]  halt ->  ../bin/systemctl
│       ├── [lrwxrwxrwx    22]  init ->  ../lib/systemd/systemd
│       ├── [lrwxrwxrwx    22]  mount.ddi ->  ../bin/systemd-dissect
│       ├── [lrwxrwxrwx    16]  poweroff ->  ../bin/systemctl
│       ├── [lrwxrwxrwx    16]  reboot ->  ../bin/systemctl
│       ├── [lrwxrwxrwx    17]  resolvconf ->  ../bin/resolvectl
│       ├── [lrwxrwxrwx    16]  runlevel ->  ../bin/systemctl
│       ├── [lrwxrwxrwx    16]  shutdown ->  ../bin/systemctl
│       └── [lrwxrwxrwx    16]  telinit ->  ../bin/systemctl
└── [drwxr-xr-x     3]  var
    └── [drwxr-xr-x     3]  lib
        └── [drwxr-xr-x     2]  systemd

101 directories, 692 files
```
