├── [drwxr-xr-x     5]  etc
│   ├── [drwxr-xr-x    13]  libvirt
│   │   ├── [-rw-r--r--   547]  libvirt.conf
│   │   ├── [-rw-r--r--   17K]  libvirtd.conf
│   │   ├── [-rw-r--r--  1.0K]  network.conf
│   │   ├── [drwxr-xr-x     4]  qemu
│   │   │   ├── [drwxr-xr-x     2]  autostart
│   │   │   └── [drwxr-xr-x     4]  networks
│   │   │       ├── [drwxr-xr-x     3]  autostart
│   │   │       │   └── [lrwxrwxrwx    14]  default.xml -> ../default.xml
│   │   │       └── [-rw-r--r--   228]  default.xml
│   │   ├── [drwxr-xr-x     2]  secrets
│   │   ├── [drwxr-xr-x     3]  storage
│   │   │   └── [drwxr-xr-x     2]  autostart
│   │   ├── [-rw-r--r--   12K]  virtinterfaced.conf
│   │   ├── [-rw-r--r--  4.0K]  virtlogd.conf
│   │   ├── [-rw-r--r--   12K]  virtnodedevd.conf
│   │   ├── [-rw-r--r--   12K]  virtnwfilterd.conf
│   │   └── [-rw-r--r--   17K]  virtproxyd.conf
│   ├── [drwxr-xr-x     3]  sasl2
│   │   └── [-rw-r--r--  1.7K]  libvirt.conf
└── [drwxr-xr-x    10]  usr
    ├── [drwxr-xr-x     9]  bin
    │   ├── [-rwxr-xr-x   26K]  virt-host-validate
    │   ├── [-rwxr-xr-x   14K]  virt-pki-query-dn
    │   ├── [-rwxr-xr-x   42K]  virt-pki-validate
    │   ├── [-rwxr-xr-x   22K]  virt-qemu-run
    │   ├── [-rwxr-xr-x   45K]  virt-qemu-sev-validate
    │   ├── [-rwxr-xr-x   30K]  virt-ssh-helper
    │   └── [-rwxr-xr-x  3.1K]  virt-xml-validate
    ├── [drwxr-xr-x     3]  include
    │   └── [drwxr-xr-x    20]  libvirt
    │       ├── [-rw-r--r--   14K]  libvirt-admin.h
    │       ├── [-rw-r--r--   11K]  libvirt-common.h
    │       ├── [-rw-r--r--  6.9K]  libvirt-domain-checkpoint.h
    │       ├── [-rw-r--r--   12K]  libvirt-domain-snapshot.h
    │       ├── [-rw-r--r--  223K]  libvirt-domain.h
    │       ├── [-rw-r--r--  6.8K]  libvirt-event.h
    │       ├── [-rw-r--r--   28K]  libvirt-host.h
    │       ├── [-rw-r--r--  4.8K]  libvirt-interface.h
    │       ├── [-rw-r--r--  1.9K]  libvirt-lxc.h
    │       ├── [-rw-r--r--   19K]  libvirt-network.h
    │       ├── [-rw-r--r--   13K]  libvirt-nodedev.h
    │       ├── [-rw-r--r--  5.6K]  libvirt-nwfilter.h
    │       ├── [-rw-r--r--  5.0K]  libvirt-qemu.h
    │       ├── [-rw-r--r--  8.9K]  libvirt-secret.h
    │       ├── [-rw-r--r--   25K]  libvirt-storage.h
    │       ├── [-rw-r--r--  9.3K]  libvirt-stream.h
    │       ├── [-rw-r--r--  1.6K]  libvirt.h
    │       └── [-rw-r--r--   22K]  virterror.h
    ├── [drwxr-xr-x     4]  lib
    │   ├── [drwxr-xr-x     4]  sysctl.d
    │   │   ├── [-rw-r--r--   499]  60-libvirtd.conf
    │   │   └── [-rw-r--r--   312]  60-qemu-postcopy-migration.conf
    │   └── [drwxr-xr-x     3]  sysusers.d
    │       └── [-rw-r--r--    63]  libvirt-qemu.conf
    ├── [drwxr-xr-x     6]  lib64
    │   ├── [-rwxr-xr-x   18K]  libnss_libvirt.so.2
    │   ├── [-rwxr-xr-x   18K]  libnss_libvirt_guest.so.2
    │   ├── [drwxr-xr-x     6]  libvirt
    │   │   ├── [drwxr-xr-x     9]  connection-driver
    │   │   │   ├── [-rwxr-xr-x   33K]  libvirt_driver_interface.so
    │   │   │   ├── [-rwxr-xr-x  141K]  libvirt_driver_network.so
    │   │   │   ├── [-rwxr-xr-x   93K]  libvirt_driver_nodedev.so
    │   │   │   ├── [-rwxr-xr-x  121K]  libvirt_driver_nwfilter.so
    │   │   │   ├── [-rwxr-xr-x  2.0M]  libvirt_driver_qemu.so
    │   │   │   ├── [-rwxr-xr-x   24K]  libvirt_driver_secret.so
    │   │   │   └── [-rwxr-xr-x  141K]  libvirt_driver_storage.so
    │   │   ├── [drwxr-xr-x     3]  lock-driver
    │   │   │   └── [-rwxr-xr-x   26K]  lockd.so
    │   │   ├── [drwxr-xr-x     3]  storage-backend
    │   │   │   └── [-rwxr-xr-x   14K]  libvirt_storage_backend_fs.so
    │   │   └── [drwxr-xr-x     3]  storage-file
    │   │       └── [-rwxr-xr-x   14K]  libvirt_storage_file_fs.so
    │   └── [drwxr-xr-x     6]  pkgconfig
    │       ├── [-rw-r--r--   285]  libvirt-admin.pc
    │       ├── [-rw-r--r--   293]  libvirt-lxc.pc
    │       ├── [-rw-r--r--   298]  libvirt-qemu.pc
    │       └── [-rw-r--r--   472]  libvirt.pc
    ├── [drwxr-xr-x     6]  libexec
    │   ├── [-rwxr-xr-x   17K]  libvirt-guests.sh
    │   ├── [-rwxr-xr-x   18K]  libvirt-ssh-proxy
    │   ├── [-rwxr-xr-x   82K]  libvirt_iohelper
    │   └── [-rwxr-xr-x   19K]  libvirt_leaseshelper
    ├── [drwxr-xr-x     4]  local
    │   ├── [drwxr-xr-x    14]  lib64
    │   │   ├── [lrwxrwxrwx    18]  libvirt-admin.so -> libvirt-admin.so.0
    │   │   ├── [lrwxrwxrwx    26]  libvirt-admin.so.0 -> libvirt-admin.so.0.10009.0
    │   │   ├── [-rwxr-xr-x   67K]  libvirt-admin.so.0.10009.0
    │   │   ├── [lrwxrwxrwx    16]  libvirt-lxc.so -> libvirt-lxc.so.0
    │   │   ├── [lrwxrwxrwx    24]  libvirt-lxc.so.0 -> libvirt-lxc.so.0.10009.0
    │   │   ├── [-rwxr-xr-x   14K]  libvirt-lxc.so.0.10009.0
    │   │   ├── [lrwxrwxrwx    17]  libvirt-qemu.so -> libvirt-qemu.so.0
    │   │   ├── [lrwxrwxrwx    25]  libvirt-qemu.so.0 -> libvirt-qemu.so.0.10009.0
    │   │   ├── [-rwxr-xr-x   18K]  libvirt-qemu.so.0.10009.0
    │   │   ├── [lrwxrwxrwx    12]  libvirt.so -> libvirt.so.0
    │   │   ├── [lrwxrwxrwx    20]  libvirt.so.0 -> libvirt.so.0.10009.0
    │   │   └── [-rwxr-xr-x  3.9M]  libvirt.so.0.10009.0
    │   └── [drwxr-xr-x     3]  share
    │       └── [drwxr-xr-x    49]  locale
    │           ├── [drwxr-xr-x     3]  as
    │           │   └── [drwxr-xr-x     3]  LC_MESSAGES
    │           │       └── [-rw-r--r--  762K]  libvirt.mo
    │           ├── [drwxr-xr-x     3]  bg
    │           │   └── [drwxr-xr-x     3]  LC_MESSAGES
    │           │       └── [-rw-r--r--   32K]  libvirt.mo
    │           ├── [drwxr-xr-x     3]  bn_IN
    │           │   └── [drwxr-xr-x     3]  LC_MESSAGES
    │           │       └── [-rw-r--r--  288K]  libvirt.mo
    │           ├── [drwxr-xr-x     3]  bs
    │           │   └── [drwxr-xr-x     3]  LC_MESSAGES
    │           │       └── [-rw-r--r--   14K]  libvirt.mo
    │           ├── [drwxr-xr-x     3]  ca
    │           │   └── [drwxr-xr-x     3]  LC_MESSAGES
    │           │       └── [-rw-r--r--   34K]  libvirt.mo
    │           ├── [drwxr-xr-x     3]  cs
    │           │   └── [drwxr-xr-x     3]  LC_MESSAGES
    │           │       └── [-rw-r--r--  1.2M]  libvirt.mo
    │           ├── [drwxr-xr-x     3]  da
    │           │   └── [drwxr-xr-x     3]  LC_MESSAGES
    │           │       └── [-rw-r--r--   19K]  libvirt.mo
    │           ├── [drwxr-xr-x     3]  de
    │           │   └── [drwxr-xr-x     3]  LC_MESSAGES
    │           │       └── [-rw-r--r--  511K]  libvirt.mo
    │           ├── [drwxr-xr-x     3]  el
    │           │   └── [drwxr-xr-x     3]  LC_MESSAGES
    │           │       └── [-rw-r--r--   20K]  libvirt.mo
    │           ├── [drwxr-xr-x     3]  en_GB
    │           │   └── [drwxr-xr-x     3]  LC_MESSAGES
    │           │       └── [-rw-r--r--  519K]  libvirt.mo
    │           ├── [drwxr-xr-x     3]  es
    │           │   └── [drwxr-xr-x     3]  LC_MESSAGES
    │           │       └── [-rw-r--r--  516K]  libvirt.mo
    │           ├── [drwxr-xr-x     3]  fi
    │           │   └── [drwxr-xr-x     3]  LC_MESSAGES
    │           │       └── [-rw-r--r--  225K]  libvirt.mo
    │           ├── [drwxr-xr-x     3]  fr
    │           │   └── [drwxr-xr-x     3]  LC_MESSAGES
    │           │       └── [-rw-r--r--  1.2M]  libvirt.mo
    │           ├── [drwxr-xr-x     3]  gu
    │           │   └── [drwxr-xr-x     3]  LC_MESSAGES
    │           │       └── [-rw-r--r--  759K]  libvirt.mo
    │           ├── [drwxr-xr-x     3]  hi
    │           │   └── [drwxr-xr-x     3]  LC_MESSAGES
    │           │       └── [-rw-r--r--  427K]  libvirt.mo
    │           ├── [drwxr-xr-x     3]  hr
    │           │   └── [drwxr-xr-x     3]  LC_MESSAGES
    │           │       └── [-rw-r--r--   348]  libvirt.mo
    │           ├── [drwxr-xr-x     3]  hu
    │           │   └── [drwxr-xr-x     3]  LC_MESSAGES
    │           │       └── [-rw-r--r--   23K]  libvirt.mo
    │           ├── [drwxr-xr-x     3]  id
    │           │   └── [drwxr-xr-x     3]  LC_MESSAGES
    │           │       └── [-rw-r--r--   32K]  libvirt.mo
    │           ├── [drwxr-xr-x     3]  it
    │           │   └── [drwxr-xr-x     3]  LC_MESSAGES
    │           │       └── [-rw-r--r--  222K]  libvirt.mo
    │           ├── [drwxr-xr-x     3]  ja
    │           │   └── [drwxr-xr-x     3]  LC_MESSAGES
    │           │       └── [-rw-r--r--  1.3M]  libvirt.mo
    │           ├── [drwxr-xr-x     3]  ka
    │           │   └── [drwxr-xr-x     3]  LC_MESSAGES
    │           │       └── [-rw-r--r--   43K]  libvirt.mo
    │           ├── [drwxr-xr-x     3]  kn
    │           │   └── [drwxr-xr-x     3]  LC_MESSAGES
    │           │       └── [-rw-r--r--  836K]  libvirt.mo
    │           ├── [drwxr-xr-x     3]  ko
    │           │   └── [drwxr-xr-x     3]  LC_MESSAGES
    │           │       └── [-rw-r--r--  1.2M]  libvirt.mo
    │           ├── [drwxr-xr-x     3]  mk
    │           │   └── [drwxr-xr-x     3]  LC_MESSAGES
    │           │       └── [-rw-r--r--   29K]  libvirt.mo
    │           ├── [drwxr-xr-x     3]  ml
    │           │   └── [drwxr-xr-x     3]  LC_MESSAGES
    │           │       └── [-rw-r--r--  891K]  libvirt.mo
    │           ├── [drwxr-xr-x     3]  mr
    │           │   └── [drwxr-xr-x     3]  LC_MESSAGES
    │           │       └── [-rw-r--r--  850K]  libvirt.mo
    │           ├── [drwxr-xr-x     3]  ms
    │           │   └── [drwxr-xr-x     3]  LC_MESSAGES
    │           │       └── [-rw-r--r--  2.7K]  libvirt.mo
    │           ├── [drwxr-xr-x     3]  nb
    │           │   └── [drwxr-xr-x     3]  LC_MESSAGES
    │           │       └── [-rw-r--r--  8.7K]  libvirt.mo
    │           ├── [drwxr-xr-x     3]  nl
    │           │   └── [drwxr-xr-x     3]  LC_MESSAGES
    │           │       └── [-rw-r--r--  200K]  libvirt.mo
    │           ├── [drwxr-xr-x     3]  or
    │           │   └── [drwxr-xr-x     3]  LC_MESSAGES
    │           │       └── [-rw-r--r--  707K]  libvirt.mo
    │           ├── [drwxr-xr-x     3]  pa
    │           │   └── [drwxr-xr-x     3]  LC_MESSAGES
    │           │       └── [-rw-r--r--  682K]  libvirt.mo
    │           ├── [drwxr-xr-x     3]  pl
    │           │   └── [drwxr-xr-x     3]  LC_MESSAGES
    │           │       └── [-rw-r--r--  228K]  libvirt.mo
    │           ├── [drwxr-xr-x     3]  pt
    │           │   └── [drwxr-xr-x     3]  LC_MESSAGES
    │           │       └── [-rw-r--r--   26K]  libvirt.mo
    │           ├── [drwxr-xr-x     3]  pt_BR
    │           │   └── [drwxr-xr-x     3]  LC_MESSAGES
    │           │       └── [-rw-r--r--  513K]  libvirt.mo
    │           ├── [drwxr-xr-x     3]  ro
    │           │   └── [drwxr-xr-x     3]  LC_MESSAGES
    │           │       └── [-rw-r--r--  4.5K]  libvirt.mo
    │           ├── [drwxr-xr-x     3]  ru
    │           │   └── [drwxr-xr-x     3]  LC_MESSAGES
    │           │       └── [-rw-r--r--  1.3M]  libvirt.mo
    │           ├── [drwxr-xr-x     3]  si
    │           │   └── [drwxr-xr-x     3]  LC_MESSAGES
    │           │       └── [-rw-r--r--   575]  libvirt.mo
    │           ├── [drwxr-xr-x     3]  sr
    │           │   └── [drwxr-xr-x     3]  LC_MESSAGES
    │           │       └── [-rw-r--r--   56K]  libvirt.mo
    │           ├── [drwxr-xr-x     3]  sr@latin
    │           │   └── [drwxr-xr-x     3]  LC_MESSAGES
    │           │       └── [-rw-r--r--   44K]  libvirt.mo
    │           ├── [drwxr-xr-x     3]  sv
    │           │   └── [drwxr-xr-x     3]  LC_MESSAGES
    │           │       └── [-rw-r--r--  1.1M]  libvirt.mo
    │           ├── [drwxr-xr-x     3]  ta
    │           │   └── [drwxr-xr-x     3]  LC_MESSAGES
    │           │       └── [-rw-r--r--  904K]  libvirt.mo
    │           ├── [drwxr-xr-x     3]  te
    │           │   └── [drwxr-xr-x     3]  LC_MESSAGES
    │           │       └── [-rw-r--r--  762K]  libvirt.mo
    │           ├── [drwxr-xr-x     3]  tr
    │           │   └── [drwxr-xr-x     3]  LC_MESSAGES
    │           │       └── [-rw-r--r--  2.2K]  libvirt.mo
    │           ├── [drwxr-xr-x     3]  uk
    │           │   └── [drwxr-xr-x     3]  LC_MESSAGES
    │           │       └── [-rw-r--r--  1.6M]  libvirt.mo
    │           ├── [drwxr-xr-x     3]  vi
    │           │   └── [drwxr-xr-x     3]  LC_MESSAGES
    │           │       └── [-rw-r--r--  194K]  libvirt.mo
    │           ├── [drwxr-xr-x     3]  zh_CN
    │           │   └── [drwxr-xr-x     3]  LC_MESSAGES
    │           │       └── [-rw-r--r-- 1012K]  libvirt.mo
    │           └── [drwxr-xr-x     3]  zh_TW
    │               └── [drwxr-xr-x     3]  LC_MESSAGES
    │                   └── [-rw-r--r--   20K]  libvirt.mo
    ├── [drwxr-xr-x    13]  sbin
    │   ├── [-rwxr-xr-x  714K]  libvirtd
    │   ├── [-rwxr-xr-x  706K]  virtinterfaced
    │   ├── [-rwxr-xr-x   88K]  virtlockd
    │   ├── [-rwxr-xr-x   96K]  virtlogd
    │   ├── [-rwxr-xr-x  706K]  virtnetworkd
    │   ├── [-rwxr-xr-x  706K]  virtnodedevd
    │   ├── [-rwxr-xr-x  706K]  virtnwfilterd
    │   ├── [-rwxr-xr-x  718K]  virtproxyd
    │   ├── [-rwxr-xr-x  706K]  virtqemud
    │   ├── [-rwxr-xr-x  706K]  virtsecretd
    │   └── [-rwxr-xr-x  706K]  virtstoraged
    └── [drwxr-xr-x     5]  share
        ├── [drwxr-xr-x     3]  libvirt
        │   └── [drwxr-xr-x    67]  cpu_map
        │       ├── [-rw-r--r--  4.4K]  index.xml
        │       ├── [-rw-r--r--   190]  x86_486.xml
        │       ├── [-rw-r--r--  5.5K]  x86_Broadwell-IBRS.xml
        │       ├── [-rw-r--r--  5.5K]  x86_Broadwell-noTSX-IBRS.xml
        │       ├── [-rw-r--r--  5.4K]  x86_Broadwell-noTSX.xml
        │       ├── [-rw-r--r--  5.5K]  x86_Broadwell.xml
        │       ├── [-rw-r--r--  5.9K]  x86_Cascadelake-Server-noTSX.xml
        │       ├── [-rw-r--r--  5.9K]  x86_Cascadelake-Server.xml
        │       ├── [-rw-r--r--  2.2K]  x86_Conroe.xml
        │       ├── [-rw-r--r--  6.2K]  x86_Cooperlake.xml
        │       ├── [-rw-r--r--  1.9K]  x86_Dhyana.xml
        │       ├── [-rw-r--r--  3.3K]  x86_EPYC-Genoa.xml
        │       ├── [-rw-r--r--  2.0K]  x86_EPYC-IBPB.xml
        │       ├── [-rw-r--r--  2.5K]  x86_EPYC-Milan.xml
        │       ├── [-rw-r--r--  2.3K]  x86_EPYC-Rome.xml
        │       ├── [-rw-r--r--  2.0K]  x86_EPYC.xml
        │       ├── [-rw-r--r--  6.4K]  x86_GraniteRapids.xml
        │       ├── [-rw-r--r--  5.3K]  x86_Haswell-IBRS.xml
        │       ├── [-rw-r--r--  5.3K]  x86_Haswell-noTSX-IBRS.xml
        │       ├── [-rw-r--r--  5.2K]  x86_Haswell-noTSX.xml
        │       ├── [-rw-r--r--  5.3K]  x86_Haswell.xml
        │       ├── [-rw-r--r--  2.3K]  x86_Icelake-Client-noTSX.xml
        │       ├── [-rw-r--r--  2.3K]  x86_Icelake-Client.xml
        │       ├── [-rw-r--r--  6.3K]  x86_Icelake-Server-noTSX.xml
        │       ├── [-rw-r--r--  6.1K]  x86_Icelake-Server.xml
        │       ├── [-rw-r--r--  4.7K]  x86_IvyBridge-IBRS.xml
        │       ├── [-rw-r--r--  4.7K]  x86_IvyBridge.xml
        │       ├── [-rw-r--r--  4.2K]  x86_Nehalem-IBRS.xml
        │       ├── [-rw-r--r--  4.1K]  x86_Nehalem.xml
        │       ├── [-rw-r--r--   861]  x86_Opteron_G1.xml
        │       ├── [-rw-r--r--   973]  x86_Opteron_G2.xml
        │       ├── [-rw-r--r--  1.1K]  x86_Opteron_G3.xml
        │       ├── [-rw-r--r--  1.4K]  x86_Opteron_G4.xml
        │       ├── [-rw-r--r--  1.5K]  x86_Opteron_G5.xml
        │       ├── [-rw-r--r--  2.4K]  x86_Penryn.xml
        │       ├── [-rw-r--r--  4.4K]  x86_SandyBridge-IBRS.xml
        │       ├── [-rw-r--r--  4.3K]  x86_SandyBridge.xml
        │       ├── [-rw-r--r--  7.0K]  x86_SapphireRapids.xml
        │       ├── [-rw-r--r--  5.7K]  x86_SierraForest.xml
        │       ├── [-rw-r--r--  5.7K]  x86_Skylake-Client-IBRS.xml
        │       ├── [-rw-r--r--  5.7K]  x86_Skylake-Client-noTSX-IBRS.xml
        │       ├── [-rw-r--r--  5.7K]  x86_Skylake-Client.xml
        │       ├── [-rw-r--r--  5.8K]  x86_Skylake-Server-IBRS.xml
        │       ├── [-rw-r--r--  5.8K]  x86_Skylake-Server-noTSX-IBRS.xml
        │       ├── [-rw-r--r--  5.7K]  x86_Skylake-Server.xml
        │       ├── [-rw-r--r--  5.7K]  x86_Snowridge.xml
        │       ├── [-rw-r--r--  4.1K]  x86_Westmere-IBRS.xml
        │       ├── [-rw-r--r--  4.2K]  x86_Westmere.xml
        │       ├── [-rw-r--r--   754]  x86_athlon.xml
        │       ├── [-rw-r--r--  2.1K]  x86_core2duo.xml
        │       ├── [-rw-r--r--  1.6K]  x86_coreduo.xml
        │       ├── [-rw-r--r--   784]  x86_cpu64-rhel5.xml
        │       ├── [-rw-r--r--   841]  x86_cpu64-rhel6.xml
        │       ├── [-rw-r--r--   37K]  x86_features.xml
        │       ├── [-rw-r--r--  1.5K]  x86_kvm32.xml
        │       ├── [-rw-r--r--  1.8K]  x86_kvm64.xml
        │       ├── [-rw-r--r--   803]  x86_n270.xml
        │       ├── [-rw-r--r--   349]  x86_pentium.xml
        │       ├── [-rw-r--r--   589]  x86_pentium2.xml
        │       ├── [-rw-r--r--   615]  x86_pentium3.xml
        │       ├── [-rw-r--r--   564]  x86_pentiumpro.xml
        │       ├── [-rw-r--r--   977]  x86_phenom.xml
        │       ├── [-rw-r--r--   586]  x86_qemu32.xml
        │       ├── [-rw-r--r--  1.0K]  x86_qemu64.xml
        │       └── [-rw-r--r--   154]  x86_vendors.xml
        ├── [drwxr-xr-x     4]  polkit-1
        │   ├── [drwxr-xr-x     4]  actions
        │   │   ├── [-rw-r--r--   34K]  org.libvirt.api.policy
        │   │   └── [-rw-r--r--  2.0K]  org.libvirt.unix.policy
        │   └── [drwxr-xr-x     3]  rules.d
        │       └── [-rw-r--r--   281]  50-libvirt.rules
        └── [drwxr-xr-x     3]  systemtap
            └── [drwxr-xr-x     3]  tapset
                └── [-rw-r--r--   41K]  libvirt_functions.stp

133 directories, 198 files