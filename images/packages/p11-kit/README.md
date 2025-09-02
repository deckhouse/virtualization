```
# p11-kit
/p11-kit
[drwxr-xr-x  4.0K]  ./
├── [drwxr-xr-x  4.0K]  etc/
│   └── [drwxr-xr-x  4.0K]  pkcs11/
│       └── [-rw-r--r--   390]  pkcs11.conf.example
└── [drwxr-xr-x  4.0K]  usr/
    ├── [drwxr-xr-x  4.0K]  bin/
    │   └── [-rwxr-xr-x  171K]  p11-kit*
    ├── [drwxr-xr-x  4.0K]  include/
    │   └── [drwxr-xr-x  4.0K]  p11-kit-1/
    │       └── [drwxr-xr-x  4.0K]  p11-kit/
    │           ├── [-rw-r--r--  3.6K]  deprecated.h
    │           ├── [-rw-r--r--  5.7K]  iter.h
    │           ├── [-rw-r--r--  5.2K]  p11-kit.h
    │           ├── [-rw-r--r--  4.8K]  pin.h
    │           ├── [-rw-r--r--   65K]  pkcs11.h
    │           ├── [-rw-r--r--   11K]  pkcs11x.h
    │           ├── [-rw-r--r--  2.4K]  remote.h
    │           ├── [-rw-r--r--  8.1K]  uri.h
    │           └── [-rw-r--r--  2.2K]  version.h
    ├── [drwxr-xr-x  4.0K]  lib64/
    │   ├── [lrwxrwxrwx    15]  libp11-kit.so -> libp11-kit.so.0*
    │   ├── [lrwxrwxrwx    19]  libp11-kit.so.0 -> libp11-kit.so.0.4.1*
    │   ├── [-rwxr-xr-x  1.6M]  libp11-kit.so.0.4.1*
    │   ├── [lrwxrwxrwx    15]  p11-kit-proxy.so -> libp11-kit.so.0*
    │   ├── [drwxr-xr-x  4.0K]  pkcs11/
    │   │   └── [-rwxr-xr-x  1.3M]  p11-kit-client.so*
    │   └── [drwxr-xr-x  4.0K]  pkgconfig/
    │       └── [-rw-r--r--   420]  p11-kit-1.pc
    ├── [drwxr-xr-x  4.0K]  libexec/
    │   └── [drwxr-xr-x  4.0K]  p11-kit/
    │       ├── [-rwxr-xr-x   31K]  p11-kit-remote*
    │       └── [-rwxr-xr-x   43K]  p11-kit-server*
    └── [drwxr-xr-x  4.0K]  share/
        ├── [drwxr-xr-x  4.0K]  locale/
        │   ├── [drwxr-xr-x  4.0K]  ar/
        │   │   └── [drwxr-xr-x  4.0K]  LC_MESSAGES/
        │   │       └── [-rw-r--r--   545]  p11-kit.mo
        │   ├── [drwxr-xr-x  4.0K]  as/
        │   │   └── [drwxr-xr-x  4.0K]  LC_MESSAGES/
        │   │       └── [-rw-r--r--   464]  p11-kit.mo
        │   ├── [drwxr-xr-x  4.0K]  ast/
        │   │   └── [drwxr-xr-x  4.0K]  LC_MESSAGES/
        │   │       └── [-rw-r--r--  7.8K]  p11-kit.mo
        │   ├── [drwxr-xr-x  4.0K]  az/
        │   │   └── [drwxr-xr-x  4.0K]  LC_MESSAGES/
        │   │       └── [-rw-r--r--   467]  p11-kit.mo
        │   ├── [drwxr-xr-x  4.0K]  bg/
        │   │   └── [drwxr-xr-x  4.0K]  LC_MESSAGES/
        │   │       └── [-rw-r--r--   465]  p11-kit.mo
        │   ├── [drwxr-xr-x  4.0K]  bn_IN/
        │   │   └── [drwxr-xr-x  4.0K]  LC_MESSAGES/
        │   │       └── [-rw-r--r--   477]  p11-kit.mo
        │   ├── [drwxr-xr-x  4.0K]  ca/
        │   │   └── [drwxr-xr-x  4.0K]  LC_MESSAGES/
        │   │       └── [-rw-r--r--  8.0K]  p11-kit.mo
        │   ├── [drwxr-xr-x  4.0K]  ca@valencia/
        │   │   └── [drwxr-xr-x  4.0K]  LC_MESSAGES/
        │   │       └── [-rw-r--r--   493]  p11-kit.mo
        │   ├── [drwxr-xr-x  4.0K]  cs/
        │   │   └── [drwxr-xr-x  4.0K]  LC_MESSAGES/
        │   │       └── [-rw-r--r--  7.7K]  p11-kit.mo
        │   ├── [drwxr-xr-x  4.0K]  cy/
        │   │   └── [drwxr-xr-x  4.0K]  LC_MESSAGES/
        │   │       └── [-rw-r--r--   506]  p11-kit.mo
        │   ├── [drwxr-xr-x  4.0K]  da/
        │   │   └── [drwxr-xr-x  4.0K]  LC_MESSAGES/
        │   │       └── [-rw-r--r--   29K]  p11-kit.mo
        │   ├── [drwxr-xr-x  4.0K]  de/
        │   │   └── [drwxr-xr-x  4.0K]  LC_MESSAGES/
        │   │       └── [-rw-r--r--   19K]  p11-kit.mo
        │   ├── [drwxr-xr-x  4.0K]  el/
        │   │   └── [drwxr-xr-x  4.0K]  LC_MESSAGES/
        │   │       └── [-rw-r--r--   11K]  p11-kit.mo
        │   ├── [drwxr-xr-x  4.0K]  en_GB/
        │   │   └── [drwxr-xr-x  4.0K]  LC_MESSAGES/
        │   │       └── [-rw-r--r--  7.5K]  p11-kit.mo
        │   ├── [drwxr-xr-x  4.0K]  eo/
        │   │   └── [drwxr-xr-x  4.0K]  LC_MESSAGES/
        │   │       └── [-rw-r--r--  1.5K]  p11-kit.mo
        │   ├── [drwxr-xr-x  4.0K]  es/
        │   │   └── [drwxr-xr-x  4.0K]  LC_MESSAGES/
        │   │       └── [-rw-r--r--   30K]  p11-kit.mo
        │   ├── [drwxr-xr-x  4.0K]  et/
        │   │   └── [drwxr-xr-x  4.0K]  LC_MESSAGES/
        │   │       └── [-rw-r--r--   464]  p11-kit.mo
        │   ├── [drwxr-xr-x  4.0K]  eu/
        │   │   └── [drwxr-xr-x  4.0K]  LC_MESSAGES/
        │   │       └── [-rw-r--r--  2.1K]  p11-kit.mo
        │   ├── [drwxr-xr-x  4.0K]  fa/
        │   │   └── [drwxr-xr-x  4.0K]  LC_MESSAGES/
        │   │       └── [-rw-r--r--   462]  p11-kit.mo
        │   ├── [drwxr-xr-x  4.0K]  fi/
        │   │   └── [drwxr-xr-x  4.0K]  LC_MESSAGES/
        │   │       └── [-rw-r--r--   28K]  p11-kit.mo
        │   ├── [drwxr-xr-x  4.0K]  fo/
        │   │   └── [drwxr-xr-x  4.0K]  LC_MESSAGES/
        │   │       └── [-rw-r--r--   463]  p11-kit.mo
        │   ├── [drwxr-xr-x  4.0K]  fr/
        │   │   └── [drwxr-xr-x  4.0K]  LC_MESSAGES/
        │   │       └── [-rw-r--r--  8.0K]  p11-kit.mo
        │   ├── [drwxr-xr-x  4.0K]  fur/
        │   │   └── [drwxr-xr-x  4.0K]  LC_MESSAGES/
        │   │       └── [-rw-r--r--  7.9K]  p11-kit.mo
        │   ├── [drwxr-xr-x  4.0K]  ga/
        │   │   └── [drwxr-xr-x  4.0K]  LC_MESSAGES/
        │   │       └── [-rw-r--r--   499]  p11-kit.mo
        │   ├── [drwxr-xr-x  4.0K]  gl/
        │   │   └── [drwxr-xr-x  4.0K]  LC_MESSAGES/
        │   │       └── [-rw-r--r--  8.0K]  p11-kit.mo
        │   ├── [drwxr-xr-x  4.0K]  gu/
        │   │   └── [drwxr-xr-x  4.0K]  LC_MESSAGES/
        │   │       └── [-rw-r--r--   464]  p11-kit.mo
        │   ├── [drwxr-xr-x  4.0K]  he/
        │   │   └── [drwxr-xr-x  4.0K]  LC_MESSAGES/
        │   │       └── [-rw-r--r--   556]  p11-kit.mo
        │   ├── [drwxr-xr-x  4.0K]  hi/
        │   │   └── [drwxr-xr-x  4.0K]  LC_MESSAGES/
        │   │       └── [-rw-r--r--   461]  p11-kit.mo
        │   ├── [drwxr-xr-x  4.0K]  hr/
        │   │   └── [drwxr-xr-x  4.0K]  LC_MESSAGES/
        │   │       └── [-rw-r--r--   12K]  p11-kit.mo
        │   ├── [drwxr-xr-x  4.0K]  hu/
        │   │   └── [drwxr-xr-x  4.0K]  LC_MESSAGES/
        │   │       └── [-rw-r--r--   32K]  p11-kit.mo
        │   ├── [drwxr-xr-x  4.0K]  ia/
        │   │   └── [drwxr-xr-x  4.0K]  LC_MESSAGES/
        │   │       └── [-rw-r--r--   467]  p11-kit.mo
        │   ├── [drwxr-xr-x  4.0K]  id/
        │   │   └── [drwxr-xr-x  4.0K]  LC_MESSAGES/
        │   │       └── [-rw-r--r--   28K]  p11-kit.mo
        │   ├── [drwxr-xr-x  4.0K]  it/
        │   │   └── [drwxr-xr-x  4.0K]  LC_MESSAGES/
        │   │       └── [-rw-r--r--  8.0K]  p11-kit.mo
        │   ├── [drwxr-xr-x  4.0K]  ja/
        │   │   └── [drwxr-xr-x  4.0K]  LC_MESSAGES/
        │   │       └── [-rw-r--r--  8.8K]  p11-kit.mo
        │   ├── [drwxr-xr-x  4.0K]  ka/
        │   │   └── [drwxr-xr-x  4.0K]  LC_MESSAGES/
        │   │       └── [-rw-r--r--  7.8K]  p11-kit.mo
        │   ├── [drwxr-xr-x  4.0K]  kk/
        │   │   └── [drwxr-xr-x  4.0K]  LC_MESSAGES/
        │   │       └── [-rw-r--r--   821]  p11-kit.mo
        │   ├── [drwxr-xr-x  4.0K]  kn/
        │   │   └── [drwxr-xr-x  4.0K]  LC_MESSAGES/
        │   │       └── [-rw-r--r--   462]  p11-kit.mo
        │   ├── [drwxr-xr-x  4.0K]  ko/
        │   │   └── [drwxr-xr-x  4.0K]  LC_MESSAGES/
        │   │       └── [-rw-r--r--   20K]  p11-kit.mo
        │   ├── [drwxr-xr-x  4.0K]  lt/
        │   │   └── [drwxr-xr-x  4.0K]  LC_MESSAGES/
        │   │       └── [-rw-r--r--  2.3K]  p11-kit.mo
        │   ├── [drwxr-xr-x  4.0K]  lv/
        │   │   └── [drwxr-xr-x  4.0K]  LC_MESSAGES/
        │   │       └── [-rw-r--r--  7.5K]  p11-kit.mo
        │   ├── [drwxr-xr-x  4.0K]  ml/
        │   │   └── [drwxr-xr-x  4.0K]  LC_MESSAGES/
        │   │       └── [-rw-r--r--   465]  p11-kit.mo
        │   ├── [drwxr-xr-x  4.0K]  mr/
        │   │   └── [drwxr-xr-x  4.0K]  LC_MESSAGES/
        │   │       └── [-rw-r--r--   463]  p11-kit.mo
        │   ├── [drwxr-xr-x  4.0K]  ms/
        │   │   └── [drwxr-xr-x  4.0K]  LC_MESSAGES/
        │   │       └── [-rw-r--r--   454]  p11-kit.mo
        │   ├── [drwxr-xr-x  4.0K]  nb/
        │   │   └── [drwxr-xr-x  4.0K]  LC_MESSAGES/
        │   │       └── [-rw-r--r--   473]  p11-kit.mo
        │   ├── [drwxr-xr-x  4.0K]  nl/
        │   │   └── [drwxr-xr-x  4.0K]  LC_MESSAGES/
        │   │       └── [-rw-r--r--  7.8K]  p11-kit.mo
        │   ├── [drwxr-xr-x  4.0K]  nn/
        │   │   └── [drwxr-xr-x  4.0K]  LC_MESSAGES/
        │   │       └── [-rw-r--r--   473]  p11-kit.mo
        │   ├── [drwxr-xr-x  4.0K]  oc/
        │   │   └── [drwxr-xr-x  4.0K]  LC_MESSAGES/
        │   │       └── [-rw-r--r--  7.9K]  p11-kit.mo
        │   ├── [drwxr-xr-x  4.0K]  or/
        │   │   └── [drwxr-xr-x  4.0K]  LC_MESSAGES/
        │   │       └── [-rw-r--r--   460]  p11-kit.mo
        │   ├── [drwxr-xr-x  4.0K]  pa/
        │   │   └── [drwxr-xr-x  4.0K]  LC_MESSAGES/
        │   │       └── [-rw-r--r--   11K]  p11-kit.mo
        │   ├── [drwxr-xr-x  4.0K]  pl/
        │   │   └── [drwxr-xr-x  4.0K]  LC_MESSAGES/
        │   │       └── [-rw-r--r--  9.8K]  p11-kit.mo
        │   ├── [drwxr-xr-x  4.0K]  pt/
        │   │   └── [drwxr-xr-x  4.0K]  LC_MESSAGES/
        │   │       └── [-rw-r--r--  7.8K]  p11-kit.mo
        │   ├── [drwxr-xr-x  4.0K]  pt_BR/
        │   │   └── [drwxr-xr-x  4.0K]  LC_MESSAGES/
        │   │       └── [-rw-r--r--   30K]  p11-kit.mo
        │   ├── [drwxr-xr-x  4.0K]  ro/
        │   │   └── [drwxr-xr-x  4.0K]  LC_MESSAGES/
        │   │       └── [-rw-r--r--   505]  p11-kit.mo
        │   ├── [drwxr-xr-x  4.0K]  ru/
        │   │   └── [drwxr-xr-x  4.0K]  LC_MESSAGES/
        │   │       └── [-rw-r--r--   10K]  p11-kit.mo
        │   ├── [drwxr-xr-x  4.0K]  si/
        │   │   └── [drwxr-xr-x  4.0K]  LC_MESSAGES/
        │   │       └── [-rw-r--r--   44K]  p11-kit.mo
        │   ├── [drwxr-xr-x  4.0K]  sk/
        │   │   └── [drwxr-xr-x  4.0K]  LC_MESSAGES/
        │   │       └── [-rw-r--r--  7.9K]  p11-kit.mo
        │   ├── [drwxr-xr-x  4.0K]  sl/
        │   │   └── [drwxr-xr-x  4.0K]  LC_MESSAGES/
        │   │       └── [-rw-r--r--  7.5K]  p11-kit.mo
        │   ├── [drwxr-xr-x  4.0K]  sq/
        │   │   └── [drwxr-xr-x  4.0K]  LC_MESSAGES/
        │   │       └── [-rw-r--r--   464]  p11-kit.mo
        │   ├── [drwxr-xr-x  4.0K]  sr/
        │   │   └── [drwxr-xr-x  4.0K]  LC_MESSAGES/
        │   │       └── [-rw-r--r--  9.6K]  p11-kit.mo
        │   ├── [drwxr-xr-x  4.0K]  sr@latin/
        │   │   └── [drwxr-xr-x  4.0K]  LC_MESSAGES/
        │   │       └── [-rw-r--r--   557]  p11-kit.mo
        │   ├── [drwxr-xr-x  4.0K]  sv/
        │   │   └── [drwxr-xr-x  4.0K]  LC_MESSAGES/
        │   │       └── [-rw-r--r--   29K]  p11-kit.mo
        │   ├── [drwxr-xr-x  4.0K]  ta/
        │   │   └── [drwxr-xr-x  4.0K]  LC_MESSAGES/
        │   │       └── [-rw-r--r--   461]  p11-kit.mo
        │   ├── [drwxr-xr-x  4.0K]  te/
        │   │   └── [drwxr-xr-x  4.0K]  LC_MESSAGES/
        │   │       └── [-rw-r--r--   462]  p11-kit.mo
        │   ├── [drwxr-xr-x  4.0K]  th/
        │   │   └── [drwxr-xr-x  4.0K]  LC_MESSAGES/
        │   │       └── [-rw-r--r--   453]  p11-kit.mo
        │   ├── [drwxr-xr-x  4.0K]  tr/
        │   │   └── [drwxr-xr-x  4.0K]  LC_MESSAGES/
        │   │       └── [-rw-r--r--  7.3K]  p11-kit.mo
        │   ├── [drwxr-xr-x  4.0K]  uk/
        │   │   └── [drwxr-xr-x  4.0K]  LC_MESSAGES/
        │   │       └── [-rw-r--r--   40K]  p11-kit.mo
        │   ├── [drwxr-xr-x  4.0K]  vi/
        │   │   └── [drwxr-xr-x  4.0K]  LC_MESSAGES/
        │   │       └── [-rw-r--r--   459]  p11-kit.mo
        │   ├── [drwxr-xr-x  4.0K]  wa/
        │   │   └── [drwxr-xr-x  4.0K]  LC_MESSAGES/
        │   │       └── [-rw-r--r--   462]  p11-kit.mo
        │   ├── [drwxr-xr-x  4.0K]  zh_CN/
        │   │   └── [drwxr-xr-x  4.0K]  LC_MESSAGES/
        │   │       └── [-rw-r--r--  7.2K]  p11-kit.mo
        │   ├── [drwxr-xr-x  4.0K]  zh_HK/
        │   │   └── [drwxr-xr-x  4.0K]  LC_MESSAGES/
        │   │       └── [-rw-r--r--   474]  p11-kit.mo
        │   └── [drwxr-xr-x  4.0K]  zh_TW/
        │       └── [drwxr-xr-x  4.0K]  LC_MESSAGES/
        │           └── [-rw-r--r--  6.8K]  p11-kit.mo
        └── [drwxr-xr-x  4.0K]  p11-kit/
            └── [drwxr-xr-x  4.0K]  modules/

159 directories, 90 files
```
