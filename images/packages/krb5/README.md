# krb5
/libkrb5
```
`-- usr
    |-- bin
    |   |-- gss-client
    |   |-- k5srvutil
    |   |-- kadmin
    |   |-- kdestroy
    |   |-- kinit
    |   |-- klist
    |   |-- kpasswd
    |   |-- krb5-config
    |   |-- ksu
    |   |-- kswitch
    |   |-- ktutil
    |   |-- kvno
    |   |-- sclient
    |   |-- sim_client
    |   `-- uuclient
    |-- include
    |   |-- gssapi
    |   |   |-- gssapi.h
    |   |   |-- gssapi_alloc.h
    |   |   |-- gssapi_ext.h
    |   |   |-- gssapi_generic.h
    |   |   |-- gssapi_krb5.h
    |   |   `-- mechglue.h
    |   |-- gssapi.h
    |   |-- gssrpc
    |   |   |-- auth.h
    |   |   |-- auth_gss.h
    |   |   |-- auth_gssapi.h
    |   |   |-- auth_unix.h
    |   |   |-- clnt.h
    |   |   |-- netdb.h
    |   |   |-- pmap_clnt.h
    |   |   |-- pmap_prot.h
    |   |   |-- pmap_rmt.h
    |   |   |-- rename.h
    |   |   |-- rpc.h
    |   |   |-- rpc_msg.h
    |   |   |-- svc.h
    |   |   |-- svc_auth.h
    |   |   |-- types.h
    |   |   `-- xdr.h
    |   |-- kadm5
    |   |   |-- admin.h
    |   |   |-- chpass_util_strings.h
    |   |   `-- kadm_err.h
    |   |-- kdb.h
    |   |-- krad.h
    |   |-- krb5
    |   |   |-- ccselect_plugin.h
    |   |   |-- certauth_plugin.h
    |   |   |-- clpreauth_plugin.h
    |   |   |-- hostrealm_plugin.h
    |   |   |-- kadm5_auth_plugin.h
    |   |   |-- kadm5_hook_plugin.h
    |   |   |-- kdcpolicy_plugin.h
    |   |   |-- kdcpreauth_plugin.h
    |   |   |-- krb5.h
    |   |   |-- localauth_plugin.h
    |   |   |-- locate_plugin.h
    |   |   |-- plugin.h
    |   |   |-- preauth_plugin.h
    |   |   `-- pwqual_plugin.h
    |   |-- krb5.h
    |   |-- profile.h
    |   |-- verto-module.h
    |   `-- verto.h
    |-- lib64
    |   |-- krb5
    |   |   `-- plugins
    |   |       |-- authdata
    |   |       |-- kdb
    |   |       |-- libkrb5
    |   |       |-- preauth
    |   |       `-- tls
    |   |-- libgssapi_krb5.a
    |   |-- libgssrpc.a
    |   |-- libk5crypto.a
    |   |-- libkadm5clnt.a -> libkadm5clnt_mit.a
    |   |-- libkadm5clnt_mit.a
    |   |-- libkadm5srv.a -> libkadm5srv_mit.a
    |   |-- libkadm5srv_mit.a
    |   |-- libkdb5.a
    |   |-- libkrad.a
    |   |-- libkrb5.a
    |   |-- libkrb5_db2.a
    |   |-- libkrb5_k5tls.a
    |   |-- libkrb5_otp.a
    |   |-- libkrb5_pkinit.a
    |   |-- libkrb5_spake.a
    |   |-- libkrb5_test.a
    |   |-- libkrb5support.a
    |   |-- libverto.a
    |   `-- pkgconfig
    |       |-- gssrpc.pc
    |       |-- kadm-client.pc
    |       |-- kadm-server.pc
    |       |-- kdb.pc
    |       |-- krb5-gssapi.pc
    |       |-- krb5.pc
    |       |-- mit-krb5-gssapi.pc
    |       `-- mit-krb5.pc
    |-- sbin
    |   |-- gss-server
    |   |-- kadmin.local
    |   |-- kadmind
    |   |-- kdb5_util
    |   |-- kprop
    |   |-- kpropd
    |   |-- kproplog
    |   |-- krb5-send-pr
    |   |-- krb5kdc
    |   |-- sim_server
    |   |-- sserver
    |   `-- uuserver
    |-- share
    |   |-- examples
    |   |   `-- krb5
    |   |       |-- kdc.conf
    |   |       |-- krb5.conf
    |   |       `-- services.append
    |   `-- man
    |       |-- cat1
    |       |-- cat5
    |       |-- cat7
    |       |-- cat8
    |       |-- man1
    |       |   |-- k5srvutil.1
    |       |   |-- kadmin.1
    |       |   |-- kdestroy.1
    |       |   |-- kinit.1
    |       |   |-- klist.1
    |       |   |-- kpasswd.1
    |       |   |-- krb5-config.1
    |       |   |-- ksu.1
    |       |   |-- kswitch.1
    |       |   |-- ktutil.1
    |       |   |-- kvno.1
    |       |   `-- sclient.1
    |       |-- man5
    |       |   |-- k5identity.5
    |       |   |-- k5login.5
    |       |   |-- kadm5.acl.5
    |       |   |-- kdc.conf.5
    |       |   `-- krb5.conf.5
    |       |-- man7
    |       |   `-- kerberos.7
    |       `-- man8
    |           |-- kadmin.local.8
    |           |-- kadmind.8
    |           |-- kdb5_ldap_util.8
    |           |-- kdb5_util.8
    |           |-- kprop.8
    |           |-- kpropd.8
    |           |-- kproplog.8
    |           |-- krb5kdc.8
    |           `-- sserver.8
    `-- var
        |-- krb5kdc
        `-- run
            `-- krb5kdc
```