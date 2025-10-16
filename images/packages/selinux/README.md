# selinux
/selinux
```
└── [drwxr-xr-x     6]  usr
    ├── [drwxr-xr-x     3]  include
    │   └── [drwxr-xr-x     9]  selinux
    │       ├── [-rw-r--r--   16K]  avc.h
    │       ├── [-rw-r--r--  1.2K]  context.h
    │       ├── [-rw-r--r--  2.9K]  get_context_list.h
    │       ├── [-rw-r--r--   643]  get_default_type.h
    │       ├── [-rw-r--r--  6.3K]  label.h
    │       ├── [-rw-r--r--  7.3K]  restorecon.h
    │       └── [-rw-r--r--   29K]  selinux.h
    ├── [drwxr-xr-x     7]  lib64
    │   ├── [-rw-r--r--  444K]  libselinux.a
    │   ├── [lrwxrwxrwx    15]  libselinux.so -> libselinux.so.1
    │   ├── [-rwxr-xr-x  196K]  libselinux.so.1
    │   ├── [drwxr-xr-x     3]  pkgconfig
    │   │   └── [-rw-r--r--   276]  libselinux.pc
    │   └── [drwxr-xr-x     3]  python3
    │       └── [drwxr-xr-x     5]  site-packages
    │           ├── [lrwxrwxrwx    31]  _selinux.cpython-312.so ->                         ↵
selinux/_selinux.cpython-312.so
    │           ├── [drwxr-xr-x     5]  selinux
    │           │   ├── [-rw-r--r--   38K]  __init__.py
    │           │   ├── [-rwxr-xr-x  267K]  _selinux.cpython-312.so
    │           │   └── [-rwxr-xr-x  247K]  audit2why.cpython-312.so
    │           └── [drwxr-xr-x     9]  selinux-3.8.dist-info
    │               ├── [-rw-r--r--     4]  INSTALLER
    │               ├── [-rw-r--r--   201]  METADATA
    │               ├── [-rw-r--r--   743]  RECORD
    │               ├── [-rw-r--r--     0]  REQUESTED
    │               ├── [-rw-r--r--   104]  WHEEL
    │               ├── [-rw-r--r--    53]  direct_url.json
    │               └── [-rw-r--r--     8]  top_level.txt
    ├── [drwxr-xr-x    33]  sbin
    │   ├── [-rwxr-xr-x   15K]  avcstat
    │   ├── [-rwxr-xr-x   15K]  compute_av
    │   ├── [-rwxr-xr-x   15K]  compute_create
    │   ├── [-rwxr-xr-x   15K]  compute_member
    │   ├── [-rwxr-xr-x   15K]  compute_relabel
    │   ├── [-rwxr-xr-x   15K]  getconlist
    │   ├── [-rwxr-xr-x   15K]  getdefaultcon
    │   ├── [-rwxr-xr-x   15K]  getenforce
    │   ├── [-rwxr-xr-x   15K]  getfilecon
    │   ├── [-rwxr-xr-x   15K]  getpidcon
    │   ├── [-rwxr-xr-x   15K]  getpidprevcon
    │   ├── [-rwxr-xr-x   15K]  getpolicyload
    │   ├── [-rwxr-xr-x   15K]  getsebool
    │   ├── [-rwxr-xr-x   15K]  getseuser
    │   ├── [-rwxr-xr-x   15K]  matchpathcon
    │   ├── [-rwxr-xr-x   15K]  policyvers
    │   ├── [-rwxr-xr-x  115K]  sefcontext_compile
    │   ├── [-rwxr-xr-x   15K]  selabel_compare
    │   ├── [-rwxr-xr-x   15K]  selabel_digest
    │   ├── [-rwxr-xr-x   15K]  selabel_get_digests_all_partial_matches
    │   ├── [-rwxr-xr-x   15K]  selabel_lookup
    │   ├── [-rwxr-xr-x   15K]  selabel_lookup_best_match
    │   ├── [-rwxr-xr-x   15K]  selabel_partial_match
    │   ├── [-rwxr-xr-x   15K]  selinux_check_access
    │   ├── [-rwxr-xr-x   15K]  selinux_check_securetty_context
    │   ├── [-rwxr-xr-x   15K]  selinuxenabled
    │   ├── [-rwxr-xr-x   15K]  selinuxexeccon
    │   ├── [-rwxr-xr-x   15K]  setenforce
    │   ├── [-rwxr-xr-x   15K]  setfilecon
    │   ├── [-rwxr-xr-x   15K]  togglesebool
    │   └── [-rwxr-xr-x   15K]  validatetrans
    └── [drwxr-xr-x     2]  share

12 directories, 53 files
```
