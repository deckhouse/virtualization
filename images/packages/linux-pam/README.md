```
# linux-pam
/linux-pam
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
│   ├── [drwxr-xr-x  4.0K]  sbin/
│   │   ├── [-rwxr-xr-x   23K]  faillock*
│   │   ├── [-rwxr-xr-x   15K]  mkhomedir_helper*
│   │   ├── [-rwxr-xr-x   467]  pam_namespace_helper*
│   │   ├── [-rwxr-xr-x   15K]  pam_timestamp_check*
│   │   ├── [-rwxr-xr-x   23K]  pwhistory_helper*
│   │   ├── [-rwxr-xr-x   43K]  unix_chkpwd*
│   │   └── [-rwxr-xr-x   43K]  unix_update*
│   └── [drwxr-xr-x  4.0K]  share/
│       ├── [drwxr-xr-x  4.0K]  doc/
│       │   └── [drwxr-xr-x  4.0K]  Linux-PAM/
│       │       ├── [-rw-r--r--  8.4K]  Linux-PAM_ADG.html
│       │       ├── [-rw-r--r--   62K]  Linux-PAM_ADG.txt
│       │       ├── [-rw-r--r--  8.6K]  Linux-PAM_MWG.html
│       │       ├── [-rw-r--r--   51K]  Linux-PAM_MWG.txt
│       │       ├── [-rw-r--r--  9.3K]  Linux-PAM_SAG.html
│       │       ├── [-rw-r--r--  169K]  Linux-PAM_SAG.txt
│       │       ├── [-rw-r--r--  3.0K]  adg-author.html
│       │       ├── [-rw-r--r--  3.5K]  adg-copyright.html
│       │       ├── [-rw-r--r--  3.6K]  adg-example.html
│       │       ├── [-rw-r--r--  2.1K]  adg-files.html
│       │       ├── [-rw-r--r--  3.2K]  adg-glossary.html
│       │       ├── [-rw-r--r--   63K]  adg-interface-by-app-expected.html
│       │       ├── [-rw-r--r--  8.3K]  adg-interface-of-app-expected.html
│       │       ├── [-rw-r--r--  2.6K]  adg-interface-programming-notes.html
│       │       ├── [-rw-r--r--  4.9K]  adg-interface.html
│       │       ├── [-rw-r--r--  3.3K]  adg-introduction-description.html
│       │       ├── [-rw-r--r--  2.6K]  adg-introduction-synopsis.html
│       │       ├── [-rw-r--r--  2.0K]  adg-introduction.html
│       │       ├── [-rw-r--r--   13K]  adg-libpam-functions.html
│       │       ├── [-rw-r--r--  3.3K]  adg-libpam_misc.html
│       │       ├── [-rw-r--r--  8.2K]  adg-overview.html
│       │       ├── [-rw-r--r--  4.2K]  adg-porting.html
│       │       ├── [-rw-r--r--  2.3K]  adg-security-conv-function.html
│       │       ├── [-rw-r--r--  3.1K]  adg-security-library-calls.html
│       │       ├── [-rw-r--r--  2.8K]  adg-security-resources.html
│       │       ├── [-rw-r--r--  4.4K]  adg-security-service-name.html
│       │       ├── [-rw-r--r--  5.4K]  adg-security-user-identity.html
│       │       ├── [-rw-r--r--  3.7K]  adg-security.html
│       │       ├── [-rw-r--r--  2.2K]  adg-see-also.html
│       │       ├── [-rw-r--r--   31K]  draft-morgan-pam-current.txt
│       │       ├── [-rw-r--r--   561]  index.html
│       │       ├── [drwxr-xr-x  4.0K]  modules/
│       │       │   ├── [-rw-r--r--  6.2K]  pam_access.txt
│       │       │   ├── [-rw-r--r--   796]  pam_canonicalize_user.txt
│       │       │   ├── [-rw-r--r--  1.9K]  pam_debug.txt
│       │       │   ├── [-rw-r--r--  1.0K]  pam_deny.txt
│       │       │   ├── [-rw-r--r--  1.1K]  pam_echo.txt
│       │       │   ├── [-rw-r--r--  5.4K]  pam_env.txt
│       │       │   ├── [-rw-r--r--  2.4K]  pam_exec.txt
│       │       │   ├── [-rw-r--r--   827]  pam_faildelay.txt
│       │       │   ├── [-rw-r--r--  6.1K]  pam_faillock.txt
│       │       │   ├── [-rw-r--r--  3.0K]  pam_filter.txt
│       │       │   ├── [-rw-r--r--  1.7K]  pam_ftp.txt
│       │       │   ├── [-rw-r--r--  2.2K]  pam_group.txt
│       │       │   ├── [-rw-r--r--  1.3K]  pam_issue.txt
│       │       │   ├── [-rw-r--r--  2.3K]  pam_keyinit.txt
│       │       │   ├── [-rw-r--r--  3.3K]  pam_limits.txt
│       │       │   ├── [-rw-r--r--  3.6K]  pam_listfile.txt
│       │       │   ├── [-rw-r--r--  1.1K]  pam_localuser.txt
│       │       │   ├── [-rw-r--r--  1.1K]  pam_loginuid.txt
│       │       │   ├── [-rw-r--r--  2.0K]  pam_mail.txt
│       │       │   ├── [-rw-r--r--  1.3K]  pam_mkhomedir.txt
│       │       │   ├── [-rw-r--r--  2.7K]  pam_motd.txt
│       │       │   ├── [-rw-r--r--   12K]  pam_namespace.txt
│       │       │   ├── [-rw-r--r--  1.3K]  pam_nologin.txt
│       │       │   ├── [-rw-r--r--   907]  pam_permit.txt
│       │       │   ├── [-rw-r--r--  2.4K]  pam_pwhistory.txt
│       │       │   ├── [-rw-r--r--  1.8K]  pam_rhosts.txt
│       │       │   ├── [-rw-r--r--  1.1K]  pam_rootok.txt
│       │       │   ├── [-rw-r--r--  1.4K]  pam_securetty.txt
│       │       │   ├── [-rw-r--r--  2.8K]  pam_selinux.txt
│       │       │   ├── [-rw-r--r--  1.8K]  pam_sepermit.txt
│       │       │   ├── [-rw-r--r--  2.9K]  pam_setquota.txt
│       │       │   ├── [-rw-r--r--   810]  pam_shells.txt
│       │       │   ├── [-rw-r--r--  1.6K]  pam_stress.txt
│       │       │   ├── [-rw-r--r--  2.9K]  pam_succeed_if.txt
│       │       │   ├── [-rw-r--r--  1.5K]  pam_time.txt
│       │       │   ├── [-rw-r--r--  1.7K]  pam_timestamp.txt
│       │       │   ├── [-rw-r--r--  2.7K]  pam_tty_audit.txt
│       │       │   ├── [-rw-r--r--  2.0K]  pam_umask.txt
│       │       │   ├── [-rw-r--r--  7.3K]  pam_unix.txt
│       │       │   ├── [-rw-r--r--  2.8K]  pam_userdb.txt
│       │       │   ├── [-rw-r--r--  1.2K]  pam_usertype.txt
│       │       │   ├── [-rw-r--r--  1.2K]  pam_warn.txt
│       │       │   ├── [-rw-r--r--  1.9K]  pam_wheel.txt
│       │       │   └── [-rw-r--r--  3.6K]  pam_xauth.txt
│       │       ├── [-rw-r--r--  3.0K]  mwg-author.html
│       │       ├── [-rw-r--r--  3.5K]  mwg-copyright.html
│       │       ├── [-rw-r--r--  2.0K]  mwg-example.html
│       │       ├── [-rw-r--r--   46K]  mwg-expected-by-module-item.html
│       │       ├── [-rw-r--r--  8.5K]  mwg-expected-by-module-other.html
│       │       ├── [-rw-r--r--  4.0K]  mwg-expected-by-module.html
│       │       ├── [-rw-r--r--  5.6K]  mwg-expected-of-module-acct.html
│       │       ├── [-rw-r--r--   10K]  mwg-expected-of-module-auth.html
│       │       ├── [-rw-r--r--  7.4K]  mwg-expected-of-module-chauthtok.html
│       │       ├── [-rw-r--r--  6.3K]  mwg-expected-of-module-overview.html
│       │       ├── [-rw-r--r--  6.5K]  mwg-expected-of-module-session.html
│       │       ├── [-rw-r--r--  4.3K]  mwg-expected-of-module.html
│       │       ├── [-rw-r--r--  3.9K]  mwg-introduction-description.html
│       │       ├── [-rw-r--r--  2.0K]  mwg-introduction-synopsis.html
│       │       ├── [-rw-r--r--  2.0K]  mwg-introduction.html
│       │       ├── [-rw-r--r--  2.2K]  mwg-see-also.html
│       │       ├── [-rw-r--r--  2.9K]  mwg-see-options.html
│       │       ├── [-rw-r--r--  2.9K]  mwg-see-programming-libs.html
│       │       ├── [-rw-r--r--  8.9K]  mwg-see-programming-sec.html
│       │       ├── [-rw-r--r--  4.6K]  mwg-see-programming-syslog.html
│       │       ├── [-rw-r--r--  3.0K]  mwg-see-programming.html
│       │       ├── [-rw-r--r--   66K]  rfc86.0.txt
│       │       ├── [-rw-r--r--  3.0K]  sag-author.html
│       │       ├── [-rw-r--r--  3.4K]  sag-configuration-directory.html
│       │       ├── [-rw-r--r--  5.4K]  sag-configuration-example.html
│       │       ├── [-rw-r--r--   18K]  sag-configuration-file.html
│       │       ├── [-rw-r--r--  3.1K]  sag-configuration.html
│       │       ├── [-rw-r--r--  3.5K]  sag-copyright.html
│       │       ├── [-rw-r--r--  4.3K]  sag-introduction.html
│       │       ├── [-rw-r--r--   39K]  sag-module-reference.html
│       │       ├── [-rw-r--r--  7.8K]  sag-overview.html
│       │       ├── [-rw-r--r--   20K]  sag-pam_access.html
│       │       ├── [-rw-r--r--  4.7K]  sag-pam_canonicalize_user.html
│       │       ├── [-rw-r--r--  7.4K]  sag-pam_debug.html
│       │       ├── [-rw-r--r--  4.6K]  sag-pam_deny.html
│       │       ├── [-rw-r--r--  5.3K]  sag-pam_echo.html
│       │       ├── [-rw-r--r--   16K]  sag-pam_env.html
│       │       ├── [-rw-r--r--  8.7K]  sag-pam_exec.html
│       │       ├── [-rw-r--r--  4.5K]  sag-pam_faildelay.html
│       │       ├── [-rw-r--r--   12K]  sag-pam_faillock.html
│       │       ├── [-rw-r--r--  9.1K]  sag-pam_filter.html
│       │       ├── [-rw-r--r--  5.9K]  sag-pam_ftp.html
│       │       ├── [-rw-r--r--   10K]  sag-pam_group.html
│       │       ├── [-rw-r--r--  5.7K]  sag-pam_issue.html
│       │       ├── [-rw-r--r--  7.2K]  sag-pam_keyinit.html
│       │       ├── [-rw-r--r--  8.5K]  sag-pam_lastlog.html
│       │       ├── [-rw-r--r--   19K]  sag-pam_limits.html
│       │       ├── [-rw-r--r--   10K]  sag-pam_listfile.html
│       │       ├── [-rw-r--r--  5.7K]  sag-pam_localuser.html
│       │       ├── [-rw-r--r--  5.1K]  sag-pam_loginuid.html
│       │       ├── [-rw-r--r--  7.2K]  sag-pam_mail.html
│       │       ├── [-rw-r--r--  6.5K]  sag-pam_mkhomedir.html
│       │       ├── [-rw-r--r--  7.9K]  sag-pam_motd.html
│       │       ├── [-rw-r--r--   22K]  sag-pam_namespace.html
│       │       ├── [-rw-r--r--  5.1K]  sag-pam_nologin.html
│       │       ├── [-rw-r--r--  4.2K]  sag-pam_permit.html
│       │       ├── [-rw-r--r--  8.7K]  sag-pam_pwhistory.html
│       │       ├── [-rw-r--r--  6.1K]  sag-pam_rhosts.html
│       │       ├── [-rw-r--r--  5.0K]  sag-pam_rootok.html
│       │       ├── [-rw-r--r--  6.7K]  sag-pam_securetty.html
│       │       ├── [-rw-r--r--  7.9K]  sag-pam_selinux.html
│       │       ├── [-rw-r--r--  6.6K]  sag-pam_sepermit.html
│       │       ├── [-rw-r--r--  9.9K]  sag-pam_setquota.html
│       │       ├── [-rw-r--r--  4.6K]  sag-pam_shells.html
│       │       ├── [-rw-r--r--  8.4K]  sag-pam_succeed_if.html
│       │       ├── [-rw-r--r--   10K]  sag-pam_time.html
│       │       ├── [-rw-r--r--  6.7K]  sag-pam_timestamp.html
│       │       ├── [-rw-r--r--  7.7K]  sag-pam_tty_audit.html
│       │       ├── [-rw-r--r--  7.0K]  sag-pam_umask.html
│       │       ├── [-rw-r--r--   15K]  sag-pam_unix.html
│       │       ├── [-rw-r--r--  8.1K]  sag-pam_userdb.html
│       │       ├── [-rw-r--r--  4.5K]  sag-pam_warn.html
│       │       ├── [-rw-r--r--  6.8K]  sag-pam_wheel.html
│       │       ├── [-rw-r--r--  8.0K]  sag-pam_xauth.html
│       │       ├── [-rw-r--r--  2.9K]  sag-security-issues-other.html
│       │       ├── [-rw-r--r--  2.9K]  sag-security-issues-wrong.html
│       │       ├── [-rw-r--r--  2.1K]  sag-security-issues.html
│       │       ├── [-rw-r--r--  2.2K]  sag-see-also.html
│       │       └── [-rw-r--r--  3.1K]  sag-text-conventions.html
│       ├── [drwxr-xr-x  4.0K]  locale/
│       │   ├── [drwxr-xr-x  4.0K]  af/
│       │   │   └── [drwxr-xr-x  4.0K]  LC_MESSAGES/
│       │   │       └── [-rw-r--r--   494]  Linux-PAM.mo
│       │   ├── [drwxr-xr-x  4.0K]  am/
│       │   │   └── [drwxr-xr-x  4.0K]  LC_MESSAGES/
│       │   │       └── [-rw-r--r--   491]  Linux-PAM.mo
│       │   ├── [drwxr-xr-x  4.0K]  ar/
│       │   │   └── [drwxr-xr-x  4.0K]  LC_MESSAGES/
│       │   │       └── [-rw-r--r--  6.4K]  Linux-PAM.mo
│       │   ├── [drwxr-xr-x  4.0K]  as/
│       │   │   └── [drwxr-xr-x  4.0K]  LC_MESSAGES/
│       │   │       └── [-rw-r--r--   10K]  Linux-PAM.mo
│       │   ├── [drwxr-xr-x  4.0K]  az/
│       │   │   └── [drwxr-xr-x  4.0K]  LC_MESSAGES/
│       │   │       └── [-rw-r--r--  1.9K]  Linux-PAM.mo
│       │   ├── [drwxr-xr-x  4.0K]  be/
│       │   │   └── [drwxr-xr-x  4.0K]  LC_MESSAGES/
│       │   │       └── [-rw-r--r--   569]  Linux-PAM.mo
│       │   ├── [drwxr-xr-x  4.0K]  bg/
│       │   │   └── [drwxr-xr-x  4.0K]  LC_MESSAGES/
│       │   │       └── [-rw-r--r--   12K]  Linux-PAM.mo
│       │   ├── [drwxr-xr-x  4.0K]  bn/
│       │   │   └── [drwxr-xr-x  4.0K]  LC_MESSAGES/
│       │   │       └── [-rw-r--r--   11K]  Linux-PAM.mo
│       │   ├── [drwxr-xr-x  4.0K]  bn_IN/
│       │   │   └── [drwxr-xr-x  4.0K]  LC_MESSAGES/
│       │   │       └── [-rw-r--r--   11K]  Linux-PAM.mo
│       │   ├── [drwxr-xr-x  4.0K]  bs/
│       │   │   └── [drwxr-xr-x  4.0K]  LC_MESSAGES/
│       │   │       └── [-rw-r--r--   566]  Linux-PAM.mo
│       │   ├── [drwxr-xr-x  4.0K]  ca/
│       │   │   └── [drwxr-xr-x  4.0K]  LC_MESSAGES/
│       │   │       └── [-rw-r--r--  9.9K]  Linux-PAM.mo
│       │   ├── [drwxr-xr-x  4.0K]  cs/
│       │   │   └── [drwxr-xr-x  4.0K]  LC_MESSAGES/
│       │   │       └── [-rw-r--r-- 10.0K]  Linux-PAM.mo
│       │   ├── [drwxr-xr-x  4.0K]  cy/
│       │   │   └── [drwxr-xr-x  4.0K]  LC_MESSAGES/
│       │   │       └── [-rw-r--r--   535]  Linux-PAM.mo
│       │   ├── [drwxr-xr-x  4.0K]  da/
│       │   │   └── [drwxr-xr-x  4.0K]  LC_MESSAGES/
│       │   │       └── [-rw-r--r--  9.8K]  Linux-PAM.mo
│       │   ├── [drwxr-xr-x  4.0K]  de/
│       │   │   └── [drwxr-xr-x  4.0K]  LC_MESSAGES/
│       │   │       └── [-rw-r--r--   10K]  Linux-PAM.mo
│       │   ├── [drwxr-xr-x  4.0K]  de_CH/
│       │   │   └── [drwxr-xr-x  4.0K]  LC_MESSAGES/
│       │   │       └── [-rw-r--r--   511]  Linux-PAM.mo
│       │   ├── [drwxr-xr-x  4.0K]  el/
│       │   │   └── [drwxr-xr-x  4.0K]  LC_MESSAGES/
│       │   │       └── [-rw-r--r--   467]  Linux-PAM.mo
│       │   ├── [drwxr-xr-x  4.0K]  eo/
│       │   │   └── [drwxr-xr-x  4.0K]  LC_MESSAGES/
│       │   │       └── [-rw-r--r--  3.7K]  Linux-PAM.mo
│       │   ├── [drwxr-xr-x  4.0K]  es/
│       │   │   └── [drwxr-xr-x  4.0K]  LC_MESSAGES/
│       │   │       └── [-rw-r--r--  8.5K]  Linux-PAM.mo
│       │   ├── [drwxr-xr-x  4.0K]  et/
│       │   │   └── [drwxr-xr-x  4.0K]  LC_MESSAGES/
│       │   │       └── [-rw-r--r--  2.3K]  Linux-PAM.mo
│       │   ├── [drwxr-xr-x  4.0K]  eu/
│       │   │   └── [drwxr-xr-x  4.0K]  LC_MESSAGES/
│       │   │       └── [-rw-r--r--   968]  Linux-PAM.mo
│       │   ├── [drwxr-xr-x  4.0K]  fa/
│       │   │   └── [drwxr-xr-x  4.0K]  LC_MESSAGES/
│       │   │       └── [-rw-r--r--   485]  Linux-PAM.mo
│       │   ├── [drwxr-xr-x  4.0K]  fi/
│       │   │   └── [drwxr-xr-x  4.0K]  LC_MESSAGES/
│       │   │       └── [-rw-r--r-- 10.0K]  Linux-PAM.mo
│       │   ├── [drwxr-xr-x  4.0K]  fr/
│       │   │   └── [drwxr-xr-x  4.0K]  LC_MESSAGES/
│       │   │       └── [-rw-r--r--   11K]  Linux-PAM.mo
│       │   ├── [drwxr-xr-x  4.0K]  ga/
│       │   │   └── [drwxr-xr-x  4.0K]  LC_MESSAGES/
│       │   │       └── [-rw-r--r--   11K]  Linux-PAM.mo
│       │   ├── [drwxr-xr-x  4.0K]  gl/
│       │   │   └── [drwxr-xr-x  4.0K]  LC_MESSAGES/
│       │   │       └── [-rw-r--r--   493]  Linux-PAM.mo
│       │   ├── [drwxr-xr-x  4.0K]  gu/
│       │   │   └── [drwxr-xr-x  4.0K]  LC_MESSAGES/
│       │   │       └── [-rw-r--r--   10K]  Linux-PAM.mo
│       │   ├── [drwxr-xr-x  4.0K]  he/
│       │   │   └── [drwxr-xr-x  4.0K]  LC_MESSAGES/
│       │   │       └── [-rw-r--r--   11K]  Linux-PAM.mo
│       │   ├── [drwxr-xr-x  4.0K]  hi/
│       │   │   └── [drwxr-xr-x  4.0K]  LC_MESSAGES/
│       │   │       └── [-rw-r--r--  9.6K]  Linux-PAM.mo
│       │   ├── [drwxr-xr-x  4.0K]  hr/
│       │   │   └── [drwxr-xr-x  4.0K]  LC_MESSAGES/
│       │   │       └── [-rw-r--r--  9.9K]  Linux-PAM.mo
│       │   ├── [drwxr-xr-x  4.0K]  hu/
│       │   │   └── [drwxr-xr-x  4.0K]  LC_MESSAGES/
│       │   │       └── [-rw-r--r--   10K]  Linux-PAM.mo
│       │   ├── [drwxr-xr-x  4.0K]  ia/
│       │   │   └── [drwxr-xr-x  4.0K]  LC_MESSAGES/
│       │   │       └── [-rw-r--r--  6.9K]  Linux-PAM.mo
│       │   ├── [drwxr-xr-x  4.0K]  id/
│       │   │   └── [drwxr-xr-x  4.0K]  LC_MESSAGES/
│       │   │       └── [-rw-r--r--  9.7K]  Linux-PAM.mo
│       │   ├── [drwxr-xr-x  4.0K]  is/
│       │   │   └── [drwxr-xr-x  4.0K]  LC_MESSAGES/
│       │   │       └── [-rw-r--r--   494]  Linux-PAM.mo
│       │   ├── [drwxr-xr-x  4.0K]  it/
│       │   │   └── [drwxr-xr-x  4.0K]  LC_MESSAGES/
│       │   │       └── [-rw-r--r--  9.6K]  Linux-PAM.mo
│       │   ├── [drwxr-xr-x  4.0K]  ja/
│       │   │   └── [drwxr-xr-x  4.0K]  LC_MESSAGES/
│       │   │       └── [-rw-r--r--   11K]  Linux-PAM.mo
│       │   ├── [drwxr-xr-x  4.0K]  ka/
│       │   │   └── [drwxr-xr-x  4.0K]  LC_MESSAGES/
│       │   │       └── [-rw-r--r--   15K]  Linux-PAM.mo
│       │   ├── [drwxr-xr-x  4.0K]  kk/
│       │   │   └── [drwxr-xr-x  4.0K]  LC_MESSAGES/
│       │   │       └── [-rw-r--r--   13K]  Linux-PAM.mo
│       │   ├── [drwxr-xr-x  4.0K]  km/
│       │   │   └── [drwxr-xr-x  4.0K]  LC_MESSAGES/
│       │   │       └── [-rw-r--r--  8.7K]  Linux-PAM.mo
│       │   ├── [drwxr-xr-x  4.0K]  kn/
│       │   │   └── [drwxr-xr-x  4.0K]  LC_MESSAGES/
│       │   │       └── [-rw-r--r--   11K]  Linux-PAM.mo
│       │   ├── [drwxr-xr-x  4.0K]  ko/
│       │   │   └── [drwxr-xr-x  4.0K]  LC_MESSAGES/
│       │   │       └── [-rw-r--r--   10K]  Linux-PAM.mo
│       │   ├── [drwxr-xr-x  4.0K]  kw_GB/
│       │   │   └── [drwxr-xr-x  4.0K]  LC_MESSAGES/
│       │   │       └── [-rw-r--r--   448]  Linux-PAM.mo
│       │   ├── [drwxr-xr-x  4.0K]  ky/
│       │   │   └── [drwxr-xr-x  4.0K]  LC_MESSAGES/
│       │   │       └── [-rw-r--r--   484]  Linux-PAM.mo
│       │   ├── [drwxr-xr-x  4.0K]  lt/
│       │   │   └── [drwxr-xr-x  4.0K]  LC_MESSAGES/
│       │   │       └── [-rw-r--r--   558]  Linux-PAM.mo
│       │   ├── [drwxr-xr-x  4.0K]  lv/
│       │   │   └── [drwxr-xr-x  4.0K]  LC_MESSAGES/
│       │   │       └── [-rw-r--r--   527]  Linux-PAM.mo
│       │   ├── [drwxr-xr-x  4.0K]  mk/
│       │   │   └── [drwxr-xr-x  4.0K]  LC_MESSAGES/
│       │   │       └── [-rw-r--r--   525]  Linux-PAM.mo
│       │   ├── [drwxr-xr-x  4.0K]  ml/
│       │   │   └── [drwxr-xr-x  4.0K]  LC_MESSAGES/
│       │   │       └── [-rw-r--r--   12K]  Linux-PAM.mo
│       │   ├── [drwxr-xr-x  4.0K]  mn/
│       │   │   └── [drwxr-xr-x  4.0K]  LC_MESSAGES/
│       │   │       └── [-rw-r--r--   494]  Linux-PAM.mo
│       │   ├── [drwxr-xr-x  4.0K]  mr/
│       │   │   └── [drwxr-xr-x  4.0K]  LC_MESSAGES/
│       │   │       └── [-rw-r--r--   10K]  Linux-PAM.mo
│       │   ├── [drwxr-xr-x  4.0K]  ms/
│       │   │   └── [drwxr-xr-x  4.0K]  LC_MESSAGES/
│       │   │       └── [-rw-r--r--   532]  Linux-PAM.mo
│       │   ├── [drwxr-xr-x  4.0K]  my/
│       │   │   └── [drwxr-xr-x  4.0K]  LC_MESSAGES/
│       │   │       └── [-rw-r--r--   485]  Linux-PAM.mo
│       │   ├── [drwxr-xr-x  4.0K]  nb/
│       │   │   └── [drwxr-xr-x  4.0K]  LC_MESSAGES/
│       │   │       └── [-rw-r--r--  9.1K]  Linux-PAM.mo
│       │   ├── [drwxr-xr-x  4.0K]  ne/
│       │   │   └── [drwxr-xr-x  4.0K]  LC_MESSAGES/
│       │   │       └── [-rw-r--r--   491]  Linux-PAM.mo
│       │   ├── [drwxr-xr-x  4.0K]  nl/
│       │   │   └── [drwxr-xr-x  4.0K]  LC_MESSAGES/
│       │   │       └── [-rw-r--r--  9.6K]  Linux-PAM.mo
│       │   ├── [drwxr-xr-x  4.0K]  nn/
│       │   │   └── [drwxr-xr-x  4.0K]  LC_MESSAGES/
│       │   │       └── [-rw-r--r--  9.2K]  Linux-PAM.mo
│       │   ├── [drwxr-xr-x  4.0K]  or/
│       │   │   └── [drwxr-xr-x  4.0K]  LC_MESSAGES/
│       │   │       └── [-rw-r--r--   15K]  Linux-PAM.mo
│       │   ├── [drwxr-xr-x  4.0K]  pa/
│       │   │   └── [drwxr-xr-x  4.0K]  LC_MESSAGES/
│       │   │       └── [-rw-r--r--   14K]  Linux-PAM.mo
│       │   ├── [drwxr-xr-x  4.0K]  pl/
│       │   │   └── [drwxr-xr-x  4.0K]  LC_MESSAGES/
│       │   │       └── [-rw-r--r--   10K]  Linux-PAM.mo
│       │   ├── [drwxr-xr-x  4.0K]  pt/
│       │   │   └── [drwxr-xr-x  4.0K]  LC_MESSAGES/
│       │   │       └── [-rw-r--r--  9.9K]  Linux-PAM.mo
│       │   ├── [drwxr-xr-x  4.0K]  pt_BR/
│       │   │   └── [drwxr-xr-x  4.0K]  LC_MESSAGES/
│       │   │       └── [-rw-r--r--  9.7K]  Linux-PAM.mo
│       │   ├── [drwxr-xr-x  4.0K]  ro/
│       │   │   └── [drwxr-xr-x  4.0K]  LC_MESSAGES/
│       │   │       └── [-rw-r--r--   10K]  Linux-PAM.mo
│       │   ├── [drwxr-xr-x  4.0K]  ru/
│       │   │   └── [drwxr-xr-x  4.0K]  LC_MESSAGES/
│       │   │       └── [-rw-r--r--   13K]  Linux-PAM.mo
│       │   ├── [drwxr-xr-x  4.0K]  si/
│       │   │   └── [drwxr-xr-x  4.0K]  LC_MESSAGES/
│       │   │       └── [-rw-r--r--  8.4K]  Linux-PAM.mo
│       │   ├── [drwxr-xr-x  4.0K]  sk/
│       │   │   └── [drwxr-xr-x  4.0K]  LC_MESSAGES/
│       │   │       └── [-rw-r--r--   10K]  Linux-PAM.mo
│       │   ├── [drwxr-xr-x  4.0K]  sl/
│       │   │   └── [drwxr-xr-x  4.0K]  LC_MESSAGES/
│       │   │       └── [-rw-r--r--   10K]  Linux-PAM.mo
│       │   ├── [drwxr-xr-x  4.0K]  sq/
│       │   │   └── [drwxr-xr-x  4.0K]  LC_MESSAGES/
│       │   │       └── [-rw-r--r--   493]  Linux-PAM.mo
│       │   ├── [drwxr-xr-x  4.0K]  sr/
│       │   │   └── [drwxr-xr-x  4.0K]  LC_MESSAGES/
│       │   │       └── [-rw-r--r--  8.8K]  Linux-PAM.mo
│       │   ├── [drwxr-xr-x  4.0K]  sr@latin/
│       │   │   └── [drwxr-xr-x  4.0K]  LC_MESSAGES/
│       │   │       └── [-rw-r--r--  6.8K]  Linux-PAM.mo
│       │   ├── [drwxr-xr-x  4.0K]  sv/
│       │   │   └── [drwxr-xr-x  4.0K]  LC_MESSAGES/
│       │   │       └── [-rw-r--r--  9.8K]  Linux-PAM.mo
│       │   ├── [drwxr-xr-x  4.0K]  ta/
│       │   │   └── [drwxr-xr-x  4.0K]  LC_MESSAGES/
│       │   │       └── [-rw-r--r--   11K]  Linux-PAM.mo
│       │   ├── [drwxr-xr-x  4.0K]  te/
│       │   │   └── [drwxr-xr-x  4.0K]  LC_MESSAGES/
│       │   │       └── [-rw-r--r--   11K]  Linux-PAM.mo
│       │   ├── [drwxr-xr-x  4.0K]  tg/
│       │   │   └── [drwxr-xr-x  4.0K]  LC_MESSAGES/
│       │   │       └── [-rw-r--r--   490]  Linux-PAM.mo
│       │   ├── [drwxr-xr-x  4.0K]  th/
│       │   │   └── [drwxr-xr-x  4.0K]  LC_MESSAGES/
│       │   │       └── [-rw-r--r--   482]  Linux-PAM.mo
│       │   ├── [drwxr-xr-x  4.0K]  tr/
│       │   │   └── [drwxr-xr-x  4.0K]  LC_MESSAGES/
│       │   │       └── [-rw-r--r--  9.9K]  Linux-PAM.mo
│       │   ├── [drwxr-xr-x  4.0K]  uk/
│       │   │   └── [drwxr-xr-x  4.0K]  LC_MESSAGES/
│       │   │       └── [-rw-r--r--   13K]  Linux-PAM.mo
│       │   ├── [drwxr-xr-x  4.0K]  ur/
│       │   │   └── [drwxr-xr-x  4.0K]  LC_MESSAGES/
│       │   │       └── [-rw-r--r--   468]  Linux-PAM.mo
│       │   ├── [drwxr-xr-x  4.0K]  vi/
│       │   │   └── [drwxr-xr-x  4.0K]  LC_MESSAGES/
│       │   │       └── [-rw-r--r--  7.2K]  Linux-PAM.mo
│       │   ├── [drwxr-xr-x  4.0K]  yo/
│       │   │   └── [drwxr-xr-x  4.0K]  LC_MESSAGES/
│       │   │       └── [-rw-r--r--   392]  Linux-PAM.mo
│       │   ├── [drwxr-xr-x  4.0K]  zh_CN/
│       │   │   └── [drwxr-xr-x  4.0K]  LC_MESSAGES/
│       │   │       └── [-rw-r--r--  8.9K]  Linux-PAM.mo
│       │   ├── [drwxr-xr-x  4.0K]  zh_HK/
│       │   │   └── [drwxr-xr-x  4.0K]  LC_MESSAGES/
│       │   │       └── [-rw-r--r--   570]  Linux-PAM.mo
│       │   ├── [drwxr-xr-x  4.0K]  zh_TW/
│       │   │   └── [drwxr-xr-x  4.0K]  LC_MESSAGES/
│       │   │       └── [-rw-r--r--  8.6K]  Linux-PAM.mo
│       │   └── [drwxr-xr-x  4.0K]  zu/
│       │       └── [drwxr-xr-x  4.0K]  LC_MESSAGES/
│       │           └── [-rw-r--r--  5.6K]  Linux-PAM.mo
│       └── [drwxr-xr-x  4.0K]  man/
│           ├── [drwxr-xr-x  4.0K]  man3/
│           │   ├── [-rw-r--r--  4.9K]  misc_conv.3
│           │   ├── [-rw-r--r--  7.1K]  pam.3
│           │   ├── [-rw-r--r--  2.9K]  pam_acct_mgmt.3
│           │   ├── [-rw-r--r--  3.2K]  pam_authenticate.3
│           │   ├── [-rw-r--r--  3.1K]  pam_chauthtok.3
│           │   ├── [-rw-r--r--  2.3K]  pam_close_session.3
│           │   ├── [-rw-r--r--  5.7K]  pam_conv.3
│           │   ├── [-rw-r--r--  3.1K]  pam_end.3
│           │   ├── [-rw-r--r--  2.3K]  pam_error.3
│           │   ├── [-rw-r--r--  5.9K]  pam_fail_delay.3
│           │   ├── [-rw-r--r--  5.4K]  pam_get_authtok.3
│           │   ├── [-rw-r--r--    22]  pam_get_authtok_noverify.3
│           │   ├── [-rw-r--r--    22]  pam_get_authtok_verify.3
│           │   ├── [-rw-r--r--  2.4K]  pam_get_data.3
│           │   ├── [-rw-r--r--  6.2K]  pam_get_item.3
│           │   ├── [-rw-r--r--  3.1K]  pam_get_user.3
│           │   ├── [-rw-r--r--  1.9K]  pam_getenv.3
│           │   ├── [-rw-r--r--  2.5K]  pam_getenvlist.3
│           │   ├── [-rw-r--r--  2.2K]  pam_info.3
│           │   ├── [-rw-r--r--  1.9K]  pam_misc_drop_env.3
│           │   ├── [-rw-r--r--  1.9K]  pam_misc_paste_env.3
│           │   ├── [-rw-r--r--  2.1K]  pam_misc_setenv.3
│           │   ├── [-rw-r--r--  2.3K]  pam_open_session.3
│           │   ├── [-rw-r--r--  2.4K]  pam_prompt.3
│           │   ├── [-rw-r--r--  3.0K]  pam_putenv.3
│           │   ├── [-rw-r--r--  4.1K]  pam_set_data.3
│           │   ├── [-rw-r--r--  6.3K]  pam_set_item.3
│           │   ├── [-rw-r--r--  3.5K]  pam_setcred.3
│           │   ├── [-rw-r--r--  3.1K]  pam_sm_acct_mgmt.3
│           │   ├── [-rw-r--r--  3.0K]  pam_sm_authenticate.3
│           │   ├── [-rw-r--r--  4.2K]  pam_sm_chauthtok.3
│           │   ├── [-rw-r--r--  2.2K]  pam_sm_close_session.3
│           │   ├── [-rw-r--r--  2.1K]  pam_sm_open_session.3
│           │   ├── [-rw-r--r--  3.8K]  pam_sm_setcred.3
│           │   ├── [-rw-r--r--  4.2K]  pam_start.3
│           │   ├── [-rw-r--r--  1.9K]  pam_strerror.3
│           │   ├── [-rw-r--r--  2.3K]  pam_syslog.3
│           │   ├── [-rw-r--r--    16]  pam_verror.3
│           │   ├── [-rw-r--r--    15]  pam_vinfo.3
│           │   ├── [-rw-r--r--    17]  pam_vprompt.3
│           │   ├── [-rw-r--r--    17]  pam_vsyslog.3
│           │   └── [-rw-r--r--  2.8K]  pam_xauth_data.3
│           ├── [drwxr-xr-x  4.0K]  man5/
│           │   ├── [-rw-r--r--  7.8K]  access.conf.5
│           │   ├── [-rw-r--r--    19]  environment.5
│           │   ├── [-rw-r--r--  5.1K]  faillock.conf.5
│           │   ├── [-rw-r--r--  4.9K]  group.conf.5
│           │   ├── [-rw-r--r--  8.2K]  limits.conf.5
│           │   ├── [-rw-r--r--  8.3K]  namespace.conf.5
│           │   ├── [-rw-r--r--   14K]  pam.conf.5
│           │   ├── [-rw-r--r--    15]  pam.d.5
│           │   ├── [-rw-r--r--  4.5K]  pam_env.conf.5
│           │   ├── [-rw-r--r--  2.8K]  pwhistory.conf.5
│           │   ├── [-rw-r--r--  2.8K]  sepermit.conf.5
│           │   └── [-rw-r--r--  4.7K]  time.conf.5
│           └── [drwxr-xr-x  4.0K]  man8/
│               ├── [-rw-r--r--  5.8K]  PAM.8
│               ├── [-rw-r--r--  2.9K]  faillock.8
│               ├── [-rw-r--r--  2.2K]  mkhomedir_helper.8
│               ├── [-rw-r--r--    10]  pam.8
│               ├── [-rw-r--r--  5.5K]  pam_access.8
│               ├── [-rw-r--r--  2.8K]  pam_canonicalize_user.8
│               ├── [-rw-r--r--  3.7K]  pam_debug.8
│               ├── [-rw-r--r--  2.7K]  pam_deny.8
│               ├── [-rw-r--r--  2.9K]  pam_echo.8
│               ├── [-rw-r--r--  5.0K]  pam_env.8
│               ├── [-rw-r--r--  4.7K]  pam_exec.8
│               ├── [-rw-r--r--  2.3K]  pam_faildelay.8
│               ├── [-rw-r--r--  9.0K]  pam_faillock.8
│               ├── [-rw-r--r--  4.8K]  pam_filter.8
│               ├── [-rw-r--r--  3.2K]  pam_ftp.8
│               ├── [-rw-r--r--  3.3K]  pam_group.8
│               ├── [-rw-r--r--  3.0K]  pam_issue.8
│               ├── [-rw-r--r--  4.4K]  pam_keyinit.8
│               ├── [-rw-r--r--  4.3K]  pam_limits.8
│               ├── [-rw-r--r--  5.7K]  pam_listfile.8
│               ├── [-rw-r--r--  3.1K]  pam_localuser.8
│               ├── [-rw-r--r--  3.0K]  pam_loginuid.8
│               ├── [-rw-r--r--  3.8K]  pam_mail.8
│               ├── [-rw-r--r--  3.8K]  pam_mkhomedir.8
│               ├── [-rw-r--r--  4.5K]  pam_motd.8
│               ├── [-rw-r--r--  7.5K]  pam_namespace.8
│               ├── [-rw-r--r--  2.0K]  pam_namespace_helper.8
│               ├── [-rw-r--r--  3.2K]  pam_nologin.8
│               ├── [-rw-r--r--  2.3K]  pam_permit.8
│               ├── [-rw-r--r--  4.5K]  pam_pwhistory.8
│               ├── [-rw-r--r--  3.5K]  pam_rhosts.8
│               ├── [-rw-r--r--  2.6K]  pam_rootok.8
│               ├── [-rw-r--r--  3.8K]  pam_securetty.8
│               ├── [-rw-r--r--  4.6K]  pam_selinux.8
│               ├── [-rw-r--r--  3.6K]  pam_sepermit.8
│               ├── [-rw-r--r--  5.3K]  pam_setquota.8
│               ├── [-rw-r--r--  2.3K]  pam_shells.8
│               ├── [-rw-r--r--  4.5K]  pam_stress.8
│               ├── [-rw-r--r--  4.9K]  pam_succeed_if.8
│               ├── [-rw-r--r--  3.1K]  pam_time.8
│               ├── [-rw-r--r--  3.4K]  pam_timestamp.8
│               ├── [-rw-r--r--  3.1K]  pam_timestamp_check.8
│               ├── [-rw-r--r--  4.4K]  pam_tty_audit.8
│               ├── [-rw-r--r--  4.1K]  pam_umask.8
│               ├── [-rw-r--r--  9.1K]  pam_unix.8
│               ├── [-rw-r--r--  4.7K]  pam_userdb.8
│               ├── [-rw-r--r--  3.2K]  pam_usertype.8
│               ├── [-rw-r--r--  2.6K]  pam_warn.8
│               ├── [-rw-r--r--  3.7K]  pam_wheel.8
│               ├── [-rw-r--r--  5.6K]  pam_xauth.8
│               ├── [-rw-r--r--  2.2K]  pwhistory_helper.8
│               ├── [-rw-r--r--  2.0K]  unix_chkpwd.8
│               └── [-rw-r--r--  2.0K]  unix_update.8
└── [drwxr-xr-x  4.0K]  var/
    └── [drwxr-xr-x  4.0K]  run/
        └── [drwxr-xr-x  4.0K]  sepermit/

192 directories, 429 files
```
