# linux-pam
/linux-pam
```
[drwxr-xr-x  4.0K]  ./
├── [drwxr-xr-x  4.0K]  etc/
│   ├── [-rw-r--r--    97]  environment
│   └── [drwxr-xr-x  4.0K]  security/
│       ├── [-rw-r--r--  4.6K]  access.conf
│       ├── [-rw-r--r--  2.2K]  faillock.conf
│       ├── [-rw-r--r--  3.5K]  group.conf
│       ├── [-rw-r--r--  2.4K]  limits.conf
│       ├── [drwxr-xr-x  4.0K]  limits.d/
│       ├── [-rw-r--r--  1.6K]  namespace.conf
│       ├── [drwxr-xr-x  4.0K]  namespace.d/
│       ├── [-rwxr-xr-x  1.9K]  namespace.init*
│       ├── [-rw-r--r--  2.9K]  pam_env.conf
│       ├── [-rw-r--r--   512]  pwhistory.conf
│       ├── [-rw-r--r--   418]  sepermit.conf
│       └── [-rw-r--r--  2.1K]  time.conf
├── [drwxr-xr-x  4.0K]  usr/
│   ├── [drwxr-xr-x  4.0K]  include/
│   │   └── [drwxr-xr-x  4.0K]  security/
│   │       ├── [-rw-r--r--  2.9K]  _pam_compat.h
│   │       ├── [-rw-r--r--  6.5K]  _pam_macros.h
│   │       ├── [-rw-r--r--   13K]  _pam_types.h
│   │       ├── [-rw-r--r--  3.4K]  pam_appl.h
│   │       ├── [-rw-r--r--  7.1K]  pam_client.h
│   │       ├── [-rw-r--r--  3.5K]  pam_ext.h
│   │       ├── [-rw-r--r--  1.1K]  pam_filter.h
│   │       ├── [-rw-r--r--  1.5K]  pam_misc.h
│   │       ├── [-rw-r--r--  4.6K]  pam_modules.h
│   │       └── [-rw-r--r--  5.8K]  pam_modutil.h
│   ├── [drwxr-xr-x  4.0K]  lib/
│   │   └── [drwxr-xr-x  4.0K]  systemd/
│   │       └── [drwxr-xr-x  4.0K]  system/
│   │           └── [-rw-r--r--   331]  pam_namespace.service
│   ├── [drwxr-xr-x  4.0K]  lib64/
│   │   ├── [lrwxrwxrwx    11]  libpam.so -> libpam.so.0*
│   │   ├── [lrwxrwxrwx    16]  libpam.so.0 -> libpam.so.0.85.1*
│   │   ├── [-rwxr-xr-x   71K]  libpam.so.0.85.1*
│   │   ├── [lrwxrwxrwx    16]  libpam_misc.so -> libpam_misc.so.0*
│   │   ├── [lrwxrwxrwx    21]  libpam_misc.so.0 -> libpam_misc.so.0.82.1*
│   │   ├── [-rwxr-xr-x   18K]  libpam_misc.so.0.82.1*
│   │   ├── [lrwxrwxrwx    12]  libpamc.so -> libpamc.so.0*
│   │   ├── [lrwxrwxrwx    17]  libpamc.so.0 -> libpamc.so.0.82.1*
│   │   ├── [-rwxr-xr-x   18K]  libpamc.so.0.82.1*
│   │   ├── [drwxr-xr-x  4.0K]  pkgconfig/
│   │   │   ├── [-rw-r--r--   267]  pam.pc
│   │   │   ├── [-rw-r--r--   276]  pam_misc.pc
│   │   │   └── [-rw-r--r--   254]  pamc.pc
│   │   └── [drwxr-xr-x  4.0K]  security/
│   │       ├── [-rwxr-xr-x   26K]  pam_access.so*
│   │       ├── [-rwxr-xr-x   14K]  pam_canonicalize_user.so*
│   │       ├── [-rwxr-xr-x   14K]  pam_debug.so*
│   │       ├── [-rwxr-xr-x   14K]  pam_deny.so*
│   │       ├── [-rwxr-xr-x   14K]  pam_echo.so*
│   │       ├── [-rwxr-xr-x   18K]  pam_env.so*
│   │       ├── [-rwxr-xr-x   22K]  pam_exec.so*
│   │       ├── [-rwxr-xr-x   14K]  pam_faildelay.so*
│   │       ├── [-rwxr-xr-x   22K]  pam_faillock.so*
│   │       ├── [drwxr-xr-x  4.0K]  pam_filter/
│   │       │   └── [-rwxr-xr-x   15K]  upperLOWER*
│   │       ├── [-rwxr-xr-x   18K]  pam_filter.so*
│   │       ├── [-rwxr-xr-x   14K]  pam_ftp.so*
│   │       ├── [-rwxr-xr-x   18K]  pam_group.so*
│   │       ├── [-rwxr-xr-x   14K]  pam_issue.so*
│   │       ├── [-rwxr-xr-x   14K]  pam_keyinit.so*
│   │       ├── [-rwxr-xr-x   26K]  pam_limits.so*
│   │       ├── [-rwxr-xr-x   14K]  pam_listfile.so*
│   │       ├── [-rwxr-xr-x   14K]  pam_localuser.so*
│   │       ├── [-rwxr-xr-x   14K]  pam_loginuid.so*
│   │       ├── [-rwxr-xr-x   18K]  pam_mail.so*
│   │       ├── [-rwxr-xr-x   14K]  pam_mkhomedir.so*
│   │       ├── [-rwxr-xr-x   18K]  pam_motd.so*
│   │       ├── [-rwxr-xr-x   47K]  pam_namespace.so*
│   │       ├── [-rwxr-xr-x   14K]  pam_nologin.so*
│   │       ├── [-rwxr-xr-x   14K]  pam_permit.so*
│   │       ├── [-rwxr-xr-x   23K]  pam_pwhistory.so*
│   │       ├── [-rwxr-xr-x   14K]  pam_rhosts.so*
│   │       ├── [-rwxr-xr-x   14K]  pam_rootok.so*
│   │       ├── [-rwxr-xr-x   14K]  pam_securetty.so*
│   │       ├── [-rwxr-xr-x   26K]  pam_selinux.so*
│   │       ├── [-rwxr-xr-x   18K]  pam_sepermit.so*
│   │       ├── [-rwxr-xr-x   18K]  pam_setquota.so*
│   │       ├── [-rwxr-xr-x   14K]  pam_shells.so*
│   │       ├── [-rwxr-xr-x   18K]  pam_stress.so*
│   │       ├── [-rwxr-xr-x   18K]  pam_succeed_if.so*
│   │       ├── [-rwxr-xr-x   18K]  pam_time.so*
│   │       ├── [-rwxr-xr-x   22K]  pam_timestamp.so*
│   │       ├── [-rwxr-xr-x   14K]  pam_tty_audit.so*
│   │       ├── [-rwxr-xr-x   14K]  pam_umask.so*
│   │       ├── [-rwxr-xr-x   59K]  pam_unix.so*
│   │       ├── [-rwxr-xr-x   18K]  pam_userdb.so*
│   │       ├── [-rwxr-xr-x   18K]  pam_usertype.so*
│   │       ├── [-rwxr-xr-x   14K]  pam_warn.so*
│   │       ├── [-rwxr-xr-x   14K]  pam_wheel.so*
│   │       └── [-rwxr-xr-x   23K]  pam_xauth.so*
│   └── [drwxr-xr-x  4.0K]  sbin/
│       ├── [-rwxr-xr-x   23K]  faillock*
│       ├── [-rwxr-xr-x   15K]  mkhomedir_helper*
│       ├── [-rwxr-xr-x   467]  pam_namespace_helper*
│       ├── [-rwxr-xr-x   15K]  pam_timestamp_check*
│       ├── [-rwxr-xr-x   23K]  pwhistory_helper*
│       ├── [-rwxr-xr-x   43K]  unix_chkpwd*
│       └── [-rwxr-xr-x   43K]  unix_update*
└── [drwxr-xr-x  4.0K]  var/
    └── [drwxr-xr-x  4.0K]  run/
        └── [drwxr-xr-x  4.0K]  sepermit/

19 directories, 86 files
```
