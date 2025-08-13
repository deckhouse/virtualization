# e2fsprogs
```
├── sbin
│   ├── badblocks
│   ├── blkid
│   ├── debugfs
│   ├── dumpe2fs
│   ├── e2freefrag
│   ├── e2fsck
│   ├── e2image
│   ├── e2label
│   ├── e2mmpstatus
│   ├── e2scrub
│   ├── e2scrub_all
│   ├── e2undo
│   ├── e4crypt
│   ├── e4defrag
│   ├── filefrag
│   ├── findfs
│   ├── fsck
│   ├── fsck.ext2
│   ├── fsck.ext3
│   ├── fsck.ext4
│   ├── logsave
│   ├── mke2fs
│   ├── mkfs.ext2
│   ├── mkfs.ext3
│   ├── mkfs.ext4
│   ├── mklost+found
│   ├── resize2fs
│   └── tune2fs
└── usr
    ├── bin
    │   ├── chattr
    │   ├── compile_et
    │   ├── lsattr
    │   ├── mk_cmds
    │   └── uuidgen
    ├── etc
    │   ├── e2scrub.conf
    │   └── mke2fs.conf
    ├── include
    │   ├── blkid
    │   │   ├── blkid.h
    │   │   └── blkid_types.h
    │   ├── com_err.h
    │   ├── e2p
    │   │   └── e2p.h
    │   ├── et
    │   │   └── com_err.h
    │   ├── ext2fs
    │   │   ├── bitops.h
    │   │   ├── ext2_err.h
    │   │   ├── ext2_ext_attr.h
    │   │   ├── ext2_fs.h
    │   │   ├── ext2_io.h
    │   │   ├── ext2_types.h
    │   │   ├── ext2fs.h
    │   │   ├── ext3_extents.h
    │   │   ├── hashmap.h
    │   │   ├── qcow2.h
    │   │   └── tdb.h
    │   ├── ss
    │   │   ├── ss.h
    │   │   └── ss_err.h
    │   └── uuid
    │       └── uuid.h
    ├── lib64
    │   ├── e2initrd_helper
    │   ├── libblkid.a
    │   ├── libblkid.so -> libblkid.so.1
    │   ├── libblkid.so.1 -> libblkid.so.1.0
    │   ├── libblkid.so.1.0
    │   ├── libcom_err.a
    │   ├── libcom_err.so -> libcom_err.so.2
    │   ├── libcom_err.so.2 -> libcom_err.so.2.1
    │   ├── libcom_err.so.2.1
    │   ├── libe2p.a
    │   ├── libe2p.so -> libe2p.so.2
    │   ├── libe2p.so.2 -> libe2p.so.2.3
    │   ├── libe2p.so.2.3
    │   ├── libext2fs.a
    │   ├── libext2fs.so -> libext2fs.so.2
    │   ├── libext2fs.so.2 -> libext2fs.so.2.4
    │   ├── libext2fs.so.2.4
    │   ├── libss.a
    │   ├── libss.so -> libss.so.2
    │   ├── libss.so.2 -> libss.so.2.0
    │   ├── libss.so.2.0
    │   ├── libuuid.a
    │   ├── libuuid.so -> libuuid.so.1
    │   ├── libuuid.so.1 -> libuuid.so.1.2
    │   ├── libuuid.so.1.2
    │   └── pkgconfig
    │       ├── blkid.pc
    │       ├── com_err.pc
    │       ├── e2p.pc
    │       ├── ext2fs.pc
    │       ├── ss.pc
    │       └── uuid.pc
    └── share
        ├── et
        │   ├── et_c.awk
        │   └── et_h.awk
        ├── info
        ├── locale
        │   ├── ca
        │   │   └── LC_MESSAGES
        │   │       └── e2fsprogs.mo
        │   ├── cs
        │   │   └── LC_MESSAGES
        │   │       └── e2fsprogs.mo
        │   ├── da
        │   │   └── LC_MESSAGES
        │   │       └── e2fsprogs.mo
        │   ├── de
        │   │   └── LC_MESSAGES
        │   │       └── e2fsprogs.mo
        │   ├── eo
        │   │   └── LC_MESSAGES
        │   │       └── e2fsprogs.mo
        │   ├── es
        │   │   └── LC_MESSAGES
        │   │       └── e2fsprogs.mo
        │   ├── fi
        │   │   └── LC_MESSAGES
        │   │       └── e2fsprogs.mo
        │   ├── fr
        │   │   └── LC_MESSAGES
        │   │       └── e2fsprogs.mo
        │   ├── fur
        │   │   └── LC_MESSAGES
        │   │       └── e2fsprogs.mo
        │   ├── hu
        │   │   └── LC_MESSAGES
        │   │       └── e2fsprogs.mo
        │   ├── id
        │   │   └── LC_MESSAGES
        │   │       └── e2fsprogs.mo
        │   ├── it
        │   │   └── LC_MESSAGES
        │   │       └── e2fsprogs.mo
        │   ├── ms
        │   │   └── LC_MESSAGES
        │   │       └── e2fsprogs.mo
        │   ├── nl
        │   │   └── LC_MESSAGES
        │   │       └── e2fsprogs.mo
        │   ├── pl
        │   │   └── LC_MESSAGES
        │   │       └── e2fsprogs.mo
        │   ├── pt
        │   │   └── LC_MESSAGES
        │   │       └── e2fsprogs.mo
        │   ├── ro
        │   │   └── LC_MESSAGES
        │   │       └── e2fsprogs.mo
        │   ├── sr
        │   │   └── LC_MESSAGES
        │   │       └── e2fsprogs.mo
        │   ├── sv
        │   │   └── LC_MESSAGES
        │   │       └── e2fsprogs.mo
        │   ├── tr
        │   │   └── LC_MESSAGES
        │   │       └── e2fsprogs.mo
        │   ├── uk
        │   │   └── LC_MESSAGES
        │   │       └── e2fsprogs.mo
        │   ├── vi
        │   │   └── LC_MESSAGES
        │   │       └── e2fsprogs.mo
        │   └── zh_CN
        │       └── LC_MESSAGES
        │           └── e2fsprogs.mo
        ├── man
        │   ├── man1
        │   │   ├── chattr.1
        │   │   ├── compile_et.1
        │   │   ├── lsattr.1
        │   │   ├── mk_cmds.1
        │   │   └── uuidgen.1
        │   ├── man3
        │   │   ├── com_err.3
        │   │   ├── libblkid.3
        │   │   ├── uuid.3
        │   │   ├── uuid_clear.3
        │   │   ├── uuid_compare.3
        │   │   ├── uuid_copy.3
        │   │   ├── uuid_generate.3
        │   │   ├── uuid_generate_random.3
        │   │   ├── uuid_generate_time.3
        │   │   ├── uuid_is_null.3
        │   │   ├── uuid_parse.3
        │   │   ├── uuid_time.3
        │   │   └── uuid_unparse.3
        │   ├── man5
        │   │   ├── e2fsck.conf.5
        │   │   ├── ext2.5
        │   │   ├── ext3.5
        │   │   ├── ext4.5
        │   │   └── mke2fs.conf.5
        │   └── man8
        │       ├── badblocks.8
        │       ├── blkid.8
        │       ├── debugfs.8
        │       ├── dumpe2fs.8
        │       ├── e2freefrag.8
        │       ├── e2fsck.8
        │       ├── e2image.8
        │       ├── e2label.8
        │       ├── e2mmpstatus.8
        │       ├── e2scrub.8
        │       ├── e2scrub_all.8
        │       ├── e2undo.8
        │       ├── e4crypt.8
        │       ├── e4defrag.8
        │       ├── filefrag.8
        │       ├── findfs.8
        │       ├── fsck.8
        │       ├── fsck.ext2.8
        │       ├── fsck.ext3.8
        │       ├── fsck.ext4.8
        │       ├── logsave.8
        │       ├── mke2fs.8
        │       ├── mkfs.ext2.8
        │       ├── mkfs.ext3.8
        │       ├── mkfs.ext4.8
        │       ├── mklost+found.8
        │       ├── resize2fs.8
        │       └── tune2fs.8
        └── ss
            ├── ct_c.awk
            └── ct_c.sed

70 directories, 163 files
```