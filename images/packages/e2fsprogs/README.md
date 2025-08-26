# e2fsprogs
```
└── [drwxr-xr-x        4096]  usr
    ├── [drwxr-xr-x        4096]  bin
    │   ├── [-rwxr-xr-x       14952]  chattr
    │   ├── [-rwxr-xr-x        1342]  compile_et
    │   ├── [-rwxr-xr-x       14952]  lsattr
    │   └── [-rwxr-xr-x        1102]  mk_cmds
    ├── [drwxr-xr-x        4096]  etc
    │   ├── [-rw-r--r--         685]  e2scrub.conf
    │   └── [-rw-r--r--         813]  mke2fs.conf
    ├── [drwxr-xr-x        4096]  include
    │   ├── [-rw-r--r--        2118]  com_err.h
    │   ├── [drwxr-xr-x        4096]  e2p
    │   │   └── [-rw-r--r--        3338]  e2p.h
    │   ├── [drwxr-xr-x        4096]  et
    │   │   └── [-rw-r--r--        2118]  com_err.h
    │   ├── [drwxr-xr-x        4096]  ext2fs
    │   │   ├── [-rw-r--r--       19838]  bitops.h
    │   │   ├── [-rw-r--r--       12036]  ext2_err.h
    │   │   ├── [-rw-r--r--        3137]  ext2_ext_attr.h
    │   │   ├── [-rw-r--r--       44151]  ext2_fs.h
    │   │   ├── [-rw-r--r--        5748]  ext2_io.h
    │   │   ├── [-rw-r--r--        4212]  ext2_types.h
    │   │   ├── [-rw-r--r--       79995]  ext2fs.h
    │   │   ├── [-rw-r--r--        4558]  ext3_extents.h
    │   │   ├── [-rw-r--r--        1183]  hashmap.h
    │   │   ├── [-rw-r--r--        2620]  qcow2.h
    │   │   └── [-rw-r--r--        8871]  tdb.h
    │   └── [drwxr-xr-x        4096]  ss
    │       ├── [-rw-r--r--        3116]  ss.h
    │       └── [-rw-r--r--        1193]  ss_err.h
    ├── [drwxr-xr-x        4096]  lib64
    │   ├── [-rwxr-xr-x       50368]  e2initrd_helper
    │   ├── [-r--r--r--       50800]  libcom_err.a
    │   ├── [lrwxrwxrwx          15]  libcom_err.so -> libcom_err.so.2
    │   ├── [lrwxrwxrwx          17]  libcom_err.so.2 -> libcom_err.so.2.1
    │   ├── [-rwxr-xr-x       18984]  libcom_err.so.2.1
    │   ├── [-r--r--r--      274520]  libe2p.a
    │   ├── [lrwxrwxrwx          11]  libe2p.so -> libe2p.so.2
    │   ├── [lrwxrwxrwx          13]  libe2p.so.2 -> libe2p.so.2.3
    │   ├── [-rwxr-xr-x       45544]  libe2p.so.2.3
    │   ├── [-r--r--r--     2905846]  libext2fs.a
    │   ├── [lrwxrwxrwx          14]  libext2fs.so -> libext2fs.so.2
    │   ├── [lrwxrwxrwx          16]  libext2fs.so.2 -> libext2fs.so.2.4
    │   ├── [-rwxr-xr-x      447032]  libext2fs.so.2.4
    │   ├── [-r--r--r--      161008]  libss.a
    │   ├── [lrwxrwxrwx          10]  libss.so -> libss.so.2
    │   ├── [lrwxrwxrwx          12]  libss.so.2 -> libss.so.2.0
    │   ├── [-rwxr-xr-x       35576]  libss.so.2.0
    │   └── [drwxr-xr-x        4096]  pkgconfig
    │       ├── [-rw-r--r--         254]  com_err.pc
    │       ├── [-rw-r--r--         242]  e2p.pc
    │       ├── [-rw-r--r--         239]  ext2fs.pc
    │       └── [-rw-r--r--         265]  ss.pc
    ├── [drwxr-xr-x        4096]  sbin
    │   ├── [-rwxr-xr-x       35440]  badblocks
    │   ├── [-rwxr-xr-x      252032]  debugfs
    │   ├── [-rwxr-xr-x       31336]  dumpe2fs
    │   ├── [-rwxr-xr-x       14936]  e2freefrag
    │   ├── [-rwxr-xr-x      361184]  e2fsck
    │   ├── [-rwxr-xr-x       56064]  e2image
    │   ├── [-rwxr-xr-x      121632]  e2label
    │   ├── [-rwxr-xr-x       31336]  e2mmpstatus
    │   ├── [-rwxr-xr-x        7556]  e2scrub
    │   ├── [-rwxr-xr-x        5033]  e2scrub_all
    │   ├── [-rwxr-xr-x       23128]  e2undo
    │   ├── [-rwxr-xr-x       27224]  e4crypt
    │   ├── [-rwxr-xr-x       35416]  e4defrag
    │   ├── [-rwxr-xr-x       19064]  filefrag
    │   ├── [-rwxr-xr-x      361184]  fsck.ext2
    │   ├── [-rwxr-xr-x      361184]  fsck.ext3
    │   ├── [-rwxr-xr-x      361184]  fsck.ext4
    │   ├── [-rwxr-xr-x       14944]  logsave
    │   ├── [-rwxr-xr-x      146272]  mke2fs
    │   ├── [-rwxr-xr-x      146272]  mkfs.ext2
    │   ├── [-rwxr-xr-x      146272]  mkfs.ext3
    │   ├── [-rwxr-xr-x      146272]  mkfs.ext4
    │   ├── [-rwxr-xr-x       14936]  mklost+found
    │   ├── [-rwxr-xr-x       68184]  resize2fs
    │   └── [-rwxr-xr-x      121632]  tune2fs
    └── [drwxr-xr-x        4096]  share
        ├── [drwxr-xr-x        4096]  et
        │   ├── [-rw-r--r--        6485]  et_c.awk
        │   └── [-rw-r--r--        4539]  et_h.awk
        ├── [drwxr-xr-x        4096]  info
        └── [drwxr-xr-x        4096]  ss
            ├── [-rw-r--r--        1551]  ct_c.awk
            └── [-rw-r--r--        2290]  ct_c.sed

16 directories, 72 files
```