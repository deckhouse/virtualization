# libcap
```
└── [drwxr-xr-x          15]  usr
    ├── [drwxr-xr-x           3]  include
    │   └── [drwxr-xr-x           4]  sys
    │       ├── [-rw-r--r--        8681]  capability.h
    │       └── [-rw-r--r--        3511]  psx_syscall.h
    ├── [-rw-r--r--       57160]  libcap.a
    ├── [lrwxrwxrwx          11]  libcap.so -> libcap.so.2
    ├── [lrwxrwxrwx          14]  libcap.so.2 -> libcap.so.2.69
    ├── [-rwxr-xr-x       53528]  libcap.so.2.69
    ├── [-rw-r--r--       18620]  libpsx.a
    ├── [lrwxrwxrwx          11]  libpsx.so -> libpsx.so.2
    ├── [lrwxrwxrwx          14]  libpsx.so.2 -> libpsx.so.2.69
    ├── [-rwxr-xr-x       29840]  libpsx.so.2.69
    ├── [drwxr-xr-x           4]  pkgconfig
    │   ├── [-rw-r--r--         198]  libcap.pc
    │   └── [-rw-r--r--         243]  libpsx.pc
    ├── [drwxr-xr-x           6]  sbin
    │   ├── [-rwxr-xr-x       91592]  capsh
    │   ├── [-rwxr-xr-x       53240]  getcap
    │   ├── [-rwxr-xr-x       52520]  getpcaps
    │   └── [-rwxr-xr-x       53144]  setcap
    ├── [drwxr-xr-x           3]  security
    │   └── [-rwxr-xr-x       17048]  pam_cap.so
    └── [drwxr-xr-x           3]  share
        └── [drwxr-xr-x           5]  man
            ├── [drwxr-xr-x           3]  man1
            │   └── [-rw-r--r--       10124]  capsh.1
            ├── [drwxr-xr-x          75]  man3
            │   ├── [-rw-r--r--          18]  __psx_syscall.3
            │   ├── [-rw-r--r--        4301]  cap_clear.3
            │   ├── [-rw-r--r--          21]  cap_clear_flag.3
            │   ├── [-rw-r--r--          21]  cap_compare.3
            │   ├── [-rw-r--r--        3770]  cap_copy_ext.3
            │   ├── [-rw-r--r--          24]  cap_copy_int.3
            │   ├── [-rw-r--r--          24]  cap_copy_int_check.3
            │   ├── [-rw-r--r--          24]  cap_drop_bound.3
            │   ├── [-rw-r--r--          20]  cap_dup.3
            │   ├── [-rw-r--r--          21]  cap_fill.3
            │   ├── [-rw-r--r--          21]  cap_fill_flag.3
            │   ├── [-rw-r--r--          20]  cap_free.3
            │   ├── [-rw-r--r--          25]  cap_from_name.3
            │   ├── [-rw-r--r--        7382]  cap_from_text.3
            │   ├── [-rw-r--r--          22]  cap_func_launcher.3
            │   ├── [-rw-r--r--          24]  cap_get_bound.3
            │   ├── [-rw-r--r--          24]  cap_get_fd.3
            │   ├── [-rw-r--r--        3940]  cap_get_file.3
            │   ├── [-rw-r--r--          21]  cap_get_flag.3
            │   ├── [-rw-r--r--          24]  cap_get_mode.3
            │   ├── [-rw-r--r--          24]  cap_get_nsowner.3
            │   ├── [-rw-r--r--          24]  cap_get_pid.3
            │   ├── [-rw-r--r--       12761]  cap_get_proc.3
            │   ├── [-rw-r--r--          24]  cap_get_secbits.3
            │   ├── [-rw-r--r--        7917]  cap_iab.3
            │   ├── [-rw-r--r--          19]  cap_iab_compare.3
            │   ├── [-rw-r--r--          19]  cap_iab_dup.3
            │   ├── [-rw-r--r--          19]  cap_iab_fill.3
            │   ├── [-rw-r--r--          19]  cap_iab_from_text.3
            │   ├── [-rw-r--r--          19]  cap_iab_get_pid.3
            │   ├── [-rw-r--r--          19]  cap_iab_get_proc.3
            │   ├── [-rw-r--r--          19]  cap_iab_get_vector.3
            │   ├── [-rw-r--r--          19]  cap_iab_init.3
            │   ├── [-rw-r--r--          19]  cap_iab_set_proc.3
            │   ├── [-rw-r--r--          19]  cap_iab_set_vector.3
            │   ├── [-rw-r--r--          19]  cap_iab_to_text.3
            │   ├── [-rw-r--r--        2354]  cap_init.3
            │   ├── [-rw-r--r--        6700]  cap_launch.3
            │   ├── [-rw-r--r--          22]  cap_launcher_callback.3
            │   ├── [-rw-r--r--          22]  cap_launcher_set_chroot.3
            │   ├── [-rw-r--r--          22]  cap_launcher_set_iab.3
            │   ├── [-rw-r--r--          22]  cap_launcher_set_mode.3
            │   ├── [-rw-r--r--          22]  cap_launcher_setgroups.3
            │   ├── [-rw-r--r--          22]  cap_launcher_setuid.3
            │   ├── [-rw-r--r--          21]  cap_max_bits.3
            │   ├── [-rw-r--r--          24]  cap_mode.3
            │   ├── [-rw-r--r--          24]  cap_mode_name.3
            │   ├── [-rw-r--r--          22]  cap_new_launcher.3
            │   ├── [-rw-r--r--          24]  cap_prctl.3
            │   ├── [-rw-r--r--          24]  cap_prctlw.3
            │   ├── [-rw-r--r--          19]  cap_proc_root.3
            │   ├── [-rw-r--r--          24]  cap_set_fd.3
            │   ├── [-rw-r--r--          24]  cap_set_file.3
            │   ├── [-rw-r--r--          21]  cap_set_flag.3
            │   ├── [-rw-r--r--          24]  cap_set_mode.3
            │   ├── [-rw-r--r--          24]  cap_set_nsowner.3
            │   ├── [-rw-r--r--          24]  cap_set_proc.3
            │   ├── [-rw-r--r--          24]  cap_set_secbits.3
            │   ├── [-rw-r--r--          18]  cap_set_syscall.3
            │   ├── [-rw-r--r--          24]  cap_setgroups.3
            │   ├── [-rw-r--r--          24]  cap_setuid.3
            │   ├── [-rw-r--r--          24]  cap_size.3
            │   ├── [-rw-r--r--          25]  cap_to_name.3
            │   ├── [-rw-r--r--          25]  cap_to_text.3
            │   ├── [-rw-r--r--          24]  capgetp.3
            │   ├── [-rw-r--r--          24]  capsetp.3
            │   ├── [-rw-r--r--        6492]  libcap.3
            │   ├── [-rw-r--r--        4458]  libpsx.3
            │   ├── [-rw-r--r--          18]  psx_load_syscalls.3
            │   ├── [-rw-r--r--          18]  psx_set_sensitivity.3
            │   ├── [-rw-r--r--          18]  psx_syscall.3
            │   ├── [-rw-r--r--          18]  psx_syscall3.3
            │   └── [-rw-r--r--          18]  psx_syscall6.3
            └── [drwxr-xr-x           6]  man8
                ├── [-rw-r--r--        2360]  captree.8
                ├── [-rw-r--r--         912]  getcap.8
                ├── [-rw-r--r--        1548]  getpcaps.8
                └── [-rw-r--r--        1813]  setcap.8

12 directories, 95 files
```