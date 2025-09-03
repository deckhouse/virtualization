# openldap
```
└── [drwxr-xr-x  4.0K]  usr
    ├── [drwxr-xr-x  4.0K]  bin
    │   ├── [lrwxrwxrwx    10]  ldapadd -> ldapmodify
    │   ├── [-rwxr-xr-x   71K]  ldapcompare
    │   ├── [-rwxr-xr-x   75K]  ldapdelete
    │   ├── [-rwxr-xr-x   75K]  ldapexop
    │   ├── [-rwxr-xr-x   75K]  ldapmodify
    │   ├── [-rwxr-xr-x   71K]  ldapmodrdn
    │   ├── [-rwxr-xr-x   75K]  ldappasswd
    │   ├── [-rwxr-xr-x  103K]  ldapsearch
    │   ├── [-rwxr-xr-x   23K]  ldapurl
    │   ├── [-rwxr-xr-x   75K]  ldapvc
    │   └── [-rwxr-xr-x   67K]  ldapwhoami
    ├── [drwxr-xr-x  4.0K]  etc
    │   └── [drwxr-xr-x  4.0K]  openldap
    │       ├── [-rw-r--r--   247]  ldap.conf
    │       ├── [-rw-r--r--   247]  ldap.conf.default
    │       ├── [drwxr-xr-x  4.0K]  schema
    │       │   ├── [-r--r--r--  3.6K]  README
    │       │   ├── [-r--r--r--  2.0K]  collective.ldif
    │       │   ├── [-r--r--r--  6.0K]  collective.schema
    │       │   ├── [-r--r--r--  1.8K]  corba.ldif
    │       │   ├── [-r--r--r--  7.9K]  corba.schema
    │       │   ├── [-r--r--r--   20K]  core.ldif
    │       │   ├── [-r--r--r--   20K]  core.schema
    │       │   ├── [-r--r--r--   12K]  cosine.ldif
    │       │   ├── [-r--r--r--   72K]  cosine.schema
    │       │   ├── [-r--r--r--  3.5K]  dsee.ldif
    │       │   ├── [-r--r--r--  3.3K]  dsee.schema
    │       │   ├── [-r--r--r--  4.7K]  duaconf.ldif
    │       │   ├── [-r--r--r--   10K]  duaconf.schema
    │       │   ├── [-r--r--r--  3.4K]  dyngroup.ldif
    │       │   ├── [-r--r--r--  3.4K]  dyngroup.schema
    │       │   ├── [-r--r--r--  3.4K]  inetorgperson.ldif
    │       │   ├── [-r--r--r--  6.1K]  inetorgperson.schema
    │       │   ├── [-r--r--r--  2.9K]  java.ldif
    │       │   ├── [-r--r--r--   14K]  java.schema
    │       │   ├── [-r--r--r--  2.0K]  misc.ldif
    │       │   ├── [-r--r--r--  2.3K]  misc.schema
    │       │   ├── [-r--r--r--  119K]  msuser.ldif
    │       │   ├── [-r--r--r--  111K]  msuser.schema
    │       │   ├── [-r--r--r--  1.2K]  namedobject.ldif
    │       │   ├── [-r--r--r--  1.5K]  namedobject.schema
    │       │   ├── [-r--r--r--  6.6K]  nis.ldif
    │       │   ├── [-r--r--r--  7.5K]  nis.schema
    │       │   ├── [-r--r--r--  3.2K]  openldap.ldif
    │       │   ├── [-r--r--r--  1.5K]  openldap.schema
    │       │   ├── [-r--r--r--  6.7K]  pmi.ldif
    │       │   └── [-r--r--r--   20K]  pmi.schema
    │       ├── [-rw-------  2.6K]  slapd.conf
    │       ├── [-rw-------  2.6K]  slapd.conf.default
    │       ├── [-rw-------  2.6K]  slapd.ldif
    │       └── [-rw-------  2.6K]  slapd.ldif.default
    │   │   ├── [-rwxr-xr-x  787K]  back_asyncmeta.so.2.0.200
    │   │   ├── [-rwxr-xr-x  1018]  back_dnssrv.la
    │   │   ├── [lrwxrwxrwx    22]  back_dnssrv.so -> back_dnssrv.so.2.0.200
    │   │   ├── [lrwxrwxrwx    22]  back_dnssrv.so.2 -> back_dnssrv.so.2.0.200
    │   │   ├── [-rwxr-xr-x  121K]  back_dnssrv.so.2.0.200
    │   │   ├── [-rwxr-xr-x  1006]  back_ldap.la
    │   │   ├── [lrwxrwxrwx    20]  back_ldap.so -> back_ldap.so.2.0.200
    │   │   ├── [lrwxrwxrwx    20]  back_ldap.so.2 -> back_ldap.so.2.0.200
    │   │   ├── [-rwxr-xr-x  743K]  back_ldap.so.2.0.200
    │   │   ├── [-rwxr-xr-x  1006]  back_meta.la
    │   │   ├── [lrwxrwxrwx    20]  back_meta.so -> back_meta.so.2.0.200
    │   │   ├── [lrwxrwxrwx    20]  back_meta.so.2 -> back_meta.so.2.0.200
    │   │   ├── [-rwxr-xr-x  732K]  back_meta.so.2.0.200
    │   │   ├── [-rwxr-xr-x  1006]  back_null.la
    │   │   ├── [lrwxrwxrwx    20]  back_null.so -> back_null.so.2.0.200
    │   │   ├── [lrwxrwxrwx    20]  back_null.so.2 -> back_null.so.2.0.200
    │   │   ├── [-rwxr-xr-x   63K]  back_null.so.2.0.200
    │   │   ├── [-rwxr-xr-x  1018]  back_passwd.la
    │   │   ├── [lrwxrwxrwx    22]  back_passwd.so -> back_passwd.so.2.0.200
    │   │   ├── [lrwxrwxrwx    22]  back_passwd.so.2 -> back_passwd.so.2.0.200
    │   │   ├── [-rwxr-xr-x  103K]  back_passwd.so.2.0.200
    │   │   ├── [-rwxr-xr-x  1012]  back_relay.la
    │   │   ├── [lrwxrwxrwx    21]  back_relay.so -> back_relay.so.2.0.200
    │   │   ├── [lrwxrwxrwx    21]  back_relay.so.2 -> back_relay.so.2.0.200
    │   │   ├── [-rwxr-xr-x   88K]  back_relay.so.2.0.200
    │   │   ├── [-rwxr-xr-x  1006]  back_sock.la
    │   │   ├── [lrwxrwxrwx    20]  back_sock.so -> back_sock.so.2.0.200
    │   │   ├── [lrwxrwxrwx    20]  back_sock.so.2 -> back_sock.so.2.0.200
    │   │   ├── [-rwxr-xr-x  301K]  back_sock.so.2.0.200
    │   │   ├── [-rwxr-xr-x   994]  collect.la
    │   │   ├── [lrwxrwxrwx    18]  collect.so -> collect.so.2.0.200
    │   │   ├── [lrwxrwxrwx    18]  collect.so.2 -> collect.so.2.0.200
    │   │   ├── [-rwxr-xr-x   58K]  collect.so.2.0.200
    │   │   ├── [-rwxr-xr-x  1012]  constraint.la
    │   │   ├── [lrwxrwxrwx    21]  constraint.so -> constraint.so.2.0.200
    │   │   ├── [lrwxrwxrwx    21]  constraint.so.2 -> constraint.so.2.0.200
    │   │   ├── [-rwxr-xr-x   95K]  constraint.so.2.0.200
    │   │   ├── [-rwxr-xr-x   970]  dds.la
    │   │   ├── [lrwxrwxrwx    14]  dds.so -> dds.so.2.0.200
    │   │   ├── [lrwxrwxrwx    14]  dds.so.2 -> dds.so.2.0.200
    │   │   ├── [-rwxr-xr-x  119K]  dds.so.2.0.200
    │   │   ├── [-rwxr-xr-x   982]  deref.la
    │   │   ├── [lrwxrwxrwx    16]  deref.so -> deref.so.2.0.200
    │   │   ├── [lrwxrwxrwx    16]  deref.so.2 -> deref.so.2.0.200
    │   │   ├── [-rwxr-xr-x   68K]  deref.so.2.0.200
    │   │   ├── [-rwxr-xr-x  1000]  dyngroup.la
    │   │   ├── [lrwxrwxrwx    19]  dyngroup.so -> dyngroup.so.2.0.200
    │   │   ├── [lrwxrwxrwx    19]  dyngroup.so.2 -> dyngroup.so.2.0.200
    │   │   ├── [-rwxr-xr-x   52K]  dyngroup.so.2.0.200
    │   │   ├── [-rwxr-xr-x   994]  dynlist.la
    │   │   ├── [lrwxrwxrwx    18]  dynlist.so -> dynlist.so.2.0.200
    │   │   ├── [lrwxrwxrwx    18]  dynlist.so.2 -> dynlist.so.2.0.200
    │   │   ├── [-rwxr-xr-x  158K]  dynlist.so.2.0.200
    │   │   ├── [-rwxr-xr-x   994]  homedir.la
    │   │   ├── [lrwxrwxrwx    18]  homedir.so -> homedir.so.2.0.200
    │   │   ├── [lrwxrwxrwx    18]  homedir.so.2 -> homedir.so.2.0.200
    │   │   ├── [-rwxr-xr-x  164K]  homedir.so.2.0.200
    │   │   ├── [-rwxr-xr-x   944]  lloadd.la
    │   │   ├── [lrwxrwxrwx    17]  lloadd.so -> lloadd.so.2.0.200
    │   │   ├── [lrwxrwxrwx    17]  lloadd.so.2 -> lloadd.so.2.0.200
    │   │   ├── [-rwxr-xr-x  1.0M]  lloadd.so.2.0.200
    │   │   ├── [-rwxr-xr-x  1000]  memberof.la
    │   │   ├── [lrwxrwxrwx    19]  memberof.so -> memberof.so.2.0.200
    │   │   ├── [lrwxrwxrwx    19]  memberof.so.2 -> memberof.so.2.0.200
    │   │   ├── [-rwxr-xr-x  123K]  memberof.so.2.0.200
    │   │   ├── [-rwxr-xr-x  1006]  nestgroup.la
    │   │   ├── [lrwxrwxrwx    20]  nestgroup.so -> nestgroup.so.2.0.200
    │   │   ├── [lrwxrwxrwx    20]  nestgroup.so.2 -> nestgroup.so.2.0.200
    │   │   ├── [-rwxr-xr-x   86K]  nestgroup.so.2.0.200
    │   │   ├── [-rwxr-xr-x   970]  otp.la
    │   │   ├── [lrwxrwxrwx    14]  otp.so -> otp.so.2.0.200
    │   │   ├── [lrwxrwxrwx    14]  otp.so.2 -> otp.so.2.0.200
    │   │   ├── [-rwxr-xr-x   84K]  otp.so.2.0.200
    │   │   ├── [-rwxr-xr-x   988]  pcache.la
    │   │   ├── [lrwxrwxrwx    17]  pcache.so -> pcache.so.2.0.200
    │   │   ├── [lrwxrwxrwx    17]  pcache.so.2 -> pcache.so.2.0.200
    │   │   ├── [-rwxr-xr-x  284K]  pcache.so.2.0.200
    │   │   ���── [-rwxr-xr-x  1001]  ppolicy.la
    │   │   ├── [lrwxrwxrwx    18]  ppolicy.so -> ppolicy.so.2.0.200
    │   │   ├── [lrwxrwxrwx    18]  ppolicy.so.2 -> ppolicy.so.2.0.200
    │   │   ├── [-rwxr-xr-x  184K]  ppolicy.so.2.0.200
    │   │   ├── [-rwxr-xr-x   988]  refint.la
    │   │   ├── [lrwxrwxrwx    17]  refint.so -> refint.so.2.0.200
    │   │   ├── [lrwxrwxrwx    17]  refint.so.2 -> refint.so.2.0.200
    │   │   ├── [-rwxr-xr-x   87K]  refint.so.2.0.200
    │   │   ├── [-rwxr-xr-x  1012]  remoteauth.la
    │   │   ├── [lrwxrwxrwx    21]  remoteauth.so -> remoteauth.so.2.0.200
    │   │   ├── [lrwxrwxrwx    21]  remoteauth.so.2 -> remoteauth.so.2.0.200
    │   │   ├── [-rwxr-xr-x   98K]  remoteauth.so.2.0.200
    │   │   ├── [-rwxr-xr-x   994]  retcode.la
    │   │   ├── [lrwxrwxrwx    18]  retcode.so -> retcode.so.2.0.200
    │   │   ├── [lrwxrwxrwx    18]  retcode.so.2 -> retcode.so.2.0.200
    │   │   ├── [-rwxr-xr-x  102K]  retcode.so.2.0.200
    │   │   ├── [-rwxr-xr-x   970]  rwm.la
    │   │   ├── [lrwxrwxrwx    14]  rwm.so -> rwm.so.2.0.200
    │   │   ├── [lrwxrwxrwx    14]  rwm.so.2 -> rwm.so.2.0.200
    │   │   ├── [-rwxr-xr-x  264K]  rwm.so.2.0.200
    │   │   ├── [-rwxr-xr-x   988]  seqmod.la
    │   │   ├── [lrwxrwxrwx    17]  seqmod.so -> seqmod.so.2.0.200
    │   │   ├── [lrwxrwxrwx    17]  seqmod.so.2 -> seqmod.so.2.0.200
    │   │   ├── [-rwxr-xr-x   52K]  seqmod.so.2.0.200
    │   │   ├── [-rwxr-xr-x   988]  sssvlv.la
    │   │   ├── [lrwxrwxrwx    17]  sssvlv.so -> sssvlv.so.2.0.200
    │   │   ├── [lrwxrwxrwx    17]  sssvlv.so.2 -> sssvlv.so.2.0.200
    │   │   ├── [-rwxr-xr-x  102K]  sssvlv.so.2.0.200
    │   │   ├── [-rwxr-xr-x  1000]  syncprov.la
    │   │   ├── [lrwxrwxrwx    19]  syncprov.so -> syncprov.so.2.0.200
    │   │   ├── [lrwxrwxrwx    19]  syncprov.so.2 -> syncprov.so.2.0.200
    │   │   ├── [-rwxr-xr-x  236K]  syncprov.so.2.0.200
    │   │   ├── [-rwxr-xr-x  1018]  translucent.la
    │   │   ├── [lrwxrwxrwx    22]  translucent.so -> translucent.so.2.0.200
    │   │   ├── [lrwxrwxrwx    22]  translucent.so.2 -> translucent.so.2.0.200
    │   │   ├── [-rwxr-xr-x  109K]  translucent.so.2.0.200
    │   │   ├── [-rwxr-xr-x   988]  unique.la
    │   │   ├── [lrwxrwxrwx    17]  unique.so -> unique.so.2.0.200
    │   │   ├── [lrwxrwxrwx    17]  unique.so.2 -> unique.so.2.0.200
    │   │   ├── [-rwxr-xr-x  123K]  unique.so.2.0.200
    │   │   ├── [-rwxr-xr-x   994]  valsort.la
    │   │   ├── [lrwxrwxrwx    18]  valsort.so -> valsort.so.2.0.200
    │   │   ├── [lrwxrwxrwx    18]  valsort.so.2 -> valsort.so.2.0.200
    │   │   └── [-rwxr-xr-x   76K]  valsort.so.2.0.200
    │   └── [-rwxr-xr-x  1.6M]  slapd
    └── [drwxr-xr-x  4.0K]  sbin
        ├── [lrwxrwxrwx    16]  slapacl -> ../libexec/slapd
        ├── [lrwxrwxrwx    16]  slapadd -> ../libexec/slapd
        ├── [lrwxrwxrwx    16]  slapauth -> ../libexec/slapd
        ├── [lrwxrwxrwx    16]  slapcat -> ../libexec/slapd
        ├── [lrwxrwxrwx    16]  slapdn -> ../libexec/slapd
        ├── [lrwxrwxrwx    16]  slapindex -> ../libexec/slapd
        ├── [lrwxrwxrwx    16]  slapmodify -> ../libexec/slapd
        ├── [lrwxrwxrwx    16]  slappasswd -> ../libexec/slapd
        ├── [lrwxrwxrwx    16]  slapschema -> ../libexec/slapd
        └── [lrwxrwxrwx    16]  slaptest -> ../libexec/slapd

15 directories, 222 files
```