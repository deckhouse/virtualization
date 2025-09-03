# krb5
/libkrb5
```
└── [drwxr-xr-x           4]  usr
    ├── [drwxr-xr-x          13]  include
    │   ├── [drwxr-xr-x           8]  gssapi
    │   │   ├── [-rw-r--r--       30083]  gssapi.h
    │   │   ├── [-rw-r--r--        2640]  gssapi_alloc.h
    │   │   ├── [-rw-r--r--       21165]  gssapi_ext.h
    │   │   ├── [-rw-r--r--        2217]  gssapi_generic.h
    │   │   ├── [-rw-r--r--       12027]  gssapi_krb5.h
    │   │   └── [-rw-r--r--        1652]  mechglue.h
    │   ├── [-rw-r--r--         181]  gssapi.h
    │   ├── [drwxr-xr-x          18]  gssrpc
    │   │   ├── [-rw-r--r--        6441]  auth.h
    │   │   ├── [-rw-r--r--        4840]  auth_gss.h
    │   │   ├── [-rw-r--r--        4333]  auth_gssapi.h
    │   │   ├── [-rw-r--r--        2896]  auth_unix.h
    │   │   ├── [-rw-r--r--        9654]  clnt.h
    │   │   ├── [-rw-r--r--        2442]  netdb.h
    │   │   ├── [-rw-r--r--        3429]  pmap_clnt.h
    │   │   ├── [-rw-r--r--        3841]  pmap_prot.h
    │   │   ├── [-rw-r--r--        2303]  pmap_rmt.h
    │   │   ├── [-rw-r--r--       10034]  rename.h
    │   │   ├── [-rw-r--r--        3143]  rpc.h
    │   │   ├── [-rw-r--r--        5107]  rpc_msg.h
    │   │   ├── [-rw-r--r--       11402]  svc.h
    │   │   ├── [-rw-r--r--        3976]  svc_auth.h
    │   │   ├── [-rw-r--r--        3628]  types.h
    │   │   └── [-rw-r--r--       11781]  xdr.h
    │   ├── [drwxr-xr-x           5]  kadm5
    │   │   ├── [-rw-r--r--       20688]  admin.h
    │   │   ├── [-rw-r--r--        1548]  chpass_util_strings.h
    │   │   └── [-rw-r--r--        4345]  kadm_err.h
    │   ├── [-rw-r--r--       64259]  kdb.h
    │   ├── [-rw-r--r--        8933]  krad.h
    │   ├── [drwxr-xr-x          16]  krb5
    │   │   ├── [-rw-r--r--        4213]  ccselect_plugin.h
    │   │   ├── [-rw-r--r--        5864]  certauth_plugin.h
    │   │   ├── [-rw-r--r--       15529]  clpreauth_plugin.h
    │   │   ├── [-rw-r--r--        5460]  hostrealm_plugin.h
    │   │   ├── [-rw-r--r--       12482]  kadm5_auth_plugin.h
    │   │   ├── [-rw-r--r--        6161]  kadm5_hook_plugin.h
    │   │   ├── [-rw-r--r--        5320]  kdcpolicy_plugin.h
    │   │   ├── [-rw-r--r--       18241]  kdcpreauth_plugin.h
    │   │   ├── [-rw-r--r--      348689]  krb5.h
    │   │   ├── [-rw-r--r--        5881]  localauth_plugin.h
    │   │   ├── [-rw-r--r--        2686]  locate_plugin.h
    │   │   ├── [-rw-r--r--        2090]  plugin.h
    │   │   ├── [-rw-r--r--        1774]  preauth_plugin.h
    │   │   └── [-rw-r--r--        4426]  pwqual_plugin.h
    │   ├── [-rw-r--r--         402]  krb5.h
    │   ├── [-rw-r--r--       12154]  profile.h
    │   ├── [-rw-r--r--        6640]  verto-module.h
    │   └── [-rw-r--r--       19437]  verto.h
    └── [drwxr-xr-x          22]  lib64
        ├── [drwxr-xr-x           3]  krb5
        │   └── [drwxr-xr-x           7]  plugins
        │       ├── [drwxr-xr-x           2]  authdata
        │       ├── [drwxr-xr-x           2]  kdb
        │       ├── [drwxr-xr-x           2]  libkrb5
        │       ├── [drwxr-xr-x           2]  preauth
        │       └── [drwxr-xr-x           2]  tls
        ├── [-rw-r--r--      750918]  libgssapi_krb5.a
        ├── [-rw-r--r--      266928]  libgssrpc.a
        ├── [-rw-r--r--      365058]  libk5crypto.a
        ├── [lrwxrwxrwx          18]  libkadm5clnt.a -> libkadm5clnt_mit.a
        ├── [-rw-r--r--      130084]  libkadm5clnt_mit.a
        ├── [lrwxrwxrwx          17]  libkadm5srv.a -> libkadm5srv_mit.a
        ├── [-rw-r--r--      177916]  libkadm5srv_mit.a
        ├── [-rw-r--r--      108234]  libkdb5.a
        ├── [-rw-r--r--       48664]  libkrad.a
        ├── [-rw-r--r--     1706266]  libkrb5.a
        ├── [-rw-r--r--      196998]  libkrb5_db2.a
        ├── [-rw-r--r--       16534]  libkrb5_k5tls.a
        ├── [-rw-r--r--       23284]  libkrb5_otp.a
        ├── [-rw-r--r--      194888]  libkrb5_pkinit.a
        ├── [-rw-r--r--       98422]  libkrb5_spake.a
        ├── [-rw-r--r--       17942]  libkrb5_test.a
        ├── [-rw-r--r--       94434]  libkrb5support.a
        ├── [-rw-r--r--       49220]  libverto.a
        └── [drwxr-xr-x          10]  pkgconfig
            ├── [-rw-r--r--         246]  gssrpc.pc
            ├── [-rw-r--r--         267]  kadm-client.pc
            ├── [-rw-r--r--         263]  kadm-server.pc
            ├── [-rw-r--r--         298]  kdb.pc
            ├── [-rw-r--r--         204]  krb5-gssapi.pc
            ├── [-rw-r--r--         329]  krb5.pc
            ├── [-rw-r--r--         254]  mit-krb5-gssapi.pc
            └── [-rw-r--r--         401]  mit-krb5.pc

16 directories, 72 files
```