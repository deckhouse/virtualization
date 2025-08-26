# nbdkit
/nbdkit
```
.
`-- usr
    |-- lib64
    |   |-- nbdkit
    |   |   |-- filters
    |   |   |   |-- nbdkit-blocksize-filter.la
    |   |   |   |-- nbdkit-blocksize-filter.so
    |   |   |   |-- nbdkit-blocksize-policy-filter.la
    |   |   |   |-- nbdkit-blocksize-policy-filter.so
    |   |   |   |-- nbdkit-cache-filter.la
    |   |   |   |-- nbdkit-cache-filter.so
    |   |   |   |-- nbdkit-cacheextents-filter.la
    |   |   |   |-- nbdkit-cacheextents-filter.so
    |   |   |   |-- nbdkit-checkwrite-filter.la
    |   |   |   |-- nbdkit-checkwrite-filter.so
    |   |   |   |-- nbdkit-cow-filter.la
    |   |   |   |-- nbdkit-cow-filter.so
    |   |   |   |-- nbdkit-ddrescue-filter.la
    |   |   |   |-- nbdkit-ddrescue-filter.so
    |   |   |   |-- nbdkit-delay-filter.la
    |   |   |   |-- nbdkit-delay-filter.so
    |   |   |   |-- nbdkit-error-filter.la
    |   |   |   |-- nbdkit-error-filter.so
    |   |   |   |-- nbdkit-evil-filter.la
    |   |   |   |-- nbdkit-evil-filter.so
    |   |   |   |-- nbdkit-exitlast-filter.la
    |   |   |   |-- nbdkit-exitlast-filter.so
    |   |   |   |-- nbdkit-exitwhen-filter.la
    |   |   |   |-- nbdkit-exitwhen-filter.so
    |   |   |   |-- nbdkit-exportname-filter.la
    |   |   |   |-- nbdkit-exportname-filter.so
    |   |   |   |-- nbdkit-extentlist-filter.la
    |   |   |   |-- nbdkit-extentlist-filter.so
    |   |   |   |-- nbdkit-fua-filter.la
    |   |   |   |-- nbdkit-fua-filter.so
    |   |   |   |-- nbdkit-ip-filter.la
    |   |   |   |-- nbdkit-ip-filter.so
    |   |   |   |-- nbdkit-limit-filter.la
    |   |   |   |-- nbdkit-limit-filter.so
    |   |   |   |-- nbdkit-log-filter.la
    |   |   |   |-- nbdkit-log-filter.so
    |   |   |   |-- nbdkit-multi-conn-filter.la
    |   |   |   |-- nbdkit-multi-conn-filter.so
    |   |   |   |-- nbdkit-nocache-filter.la
    |   |   |   |-- nbdkit-nocache-filter.so
    |   |   |   |-- nbdkit-noextents-filter.la
    |   |   |   |-- nbdkit-noextents-filter.so
    |   |   |   |-- nbdkit-nofilter-filter.la
    |   |   |   |-- nbdkit-nofilter-filter.so
    |   |   |   |-- nbdkit-noparallel-filter.la
    |   |   |   |-- nbdkit-noparallel-filter.so
    |   |   |   |-- nbdkit-nozero-filter.la
    |   |   |   |-- nbdkit-nozero-filter.so
    |   |   |   |-- nbdkit-offset-filter.la
    |   |   |   |-- nbdkit-offset-filter.so
    |   |   |   |-- nbdkit-partition-filter.la
    |   |   |   |-- nbdkit-partition-filter.so
    |   |   |   |-- nbdkit-pause-filter.la
    |   |   |   |-- nbdkit-pause-filter.so
    |   |   |   |-- nbdkit-protect-filter.la
    |   |   |   |-- nbdkit-protect-filter.so
    |   |   |   |-- nbdkit-qcow2dec-filter.la
    |   |   |   |-- nbdkit-qcow2dec-filter.so
    |   |   |   |-- nbdkit-rate-filter.la
    |   |   |   |-- nbdkit-rate-filter.so
    |   |   |   |-- nbdkit-readahead-filter.la
    |   |   |   |-- nbdkit-readahead-filter.so
    |   |   |   |-- nbdkit-readonly-filter.la
    |   |   |   |-- nbdkit-readonly-filter.so
    |   |   |   |-- nbdkit-retry-filter.la
    |   |   |   |-- nbdkit-retry-filter.so
    |   |   |   |-- nbdkit-retry-request-filter.la
    |   |   |   |-- nbdkit-retry-request-filter.so
    |   |   |   |-- nbdkit-rotational-filter.la
    |   |   |   |-- nbdkit-rotational-filter.so
    |   |   |   |-- nbdkit-scan-filter.la
    |   |   |   |-- nbdkit-scan-filter.so
    |   |   |   |-- nbdkit-spinning-filter.la
    |   |   |   |-- nbdkit-spinning-filter.so
    |   |   |   |-- nbdkit-swab-filter.la
    |   |   |   |-- nbdkit-swab-filter.so
    |   |   |   |-- nbdkit-tar-filter.la
    |   |   |   |-- nbdkit-tar-filter.so
    |   |   |   |-- nbdkit-tls-fallback-filter.la
    |   |   |   |-- nbdkit-tls-fallback-filter.so
    |   |   |   |-- nbdkit-truncate-filter.la
    |   |   |   `-- nbdkit-truncate-filter.so
    |   |   `-- plugins
    |   |       |-- nbdkit-cc-plugin.la
    |   |       |-- nbdkit-cc-plugin.so
    |   |       |-- nbdkit-cdi-plugin.la
    |   |       |-- nbdkit-cdi-plugin.so
    |   |       |-- nbdkit-data-plugin.la
    |   |       |-- nbdkit-data-plugin.so
    |   |       |-- nbdkit-eval-plugin.la
    |   |       |-- nbdkit-eval-plugin.so
    |   |       |-- nbdkit-example1-plugin.la
    |   |       |-- nbdkit-example1-plugin.so
    |   |       |-- nbdkit-example2-plugin.la
    |   |       |-- nbdkit-example2-plugin.so
    |   |       |-- nbdkit-example3-plugin.la
    |   |       |-- nbdkit-example3-plugin.so
    |   |       |-- nbdkit-file-plugin.la
    |   |       |-- nbdkit-file-plugin.so
    |   |       |-- nbdkit-floppy-plugin.la
    |   |       |-- nbdkit-floppy-plugin.so
    |   |       |-- nbdkit-full-plugin.la
    |   |       |-- nbdkit-full-plugin.so
    |   |       |-- nbdkit-info-plugin.la
    |   |       |-- nbdkit-info-plugin.so
    |   |       |-- nbdkit-linuxdisk-plugin.la
    |   |       |-- nbdkit-linuxdisk-plugin.so
    |   |       |-- nbdkit-memory-plugin.la
    |   |       |-- nbdkit-memory-plugin.so
    |   |       |-- nbdkit-null-plugin.la
    |   |       |-- nbdkit-null-plugin.so
    |   |       |-- nbdkit-ondemand-plugin.la
    |   |       |-- nbdkit-ondemand-plugin.so
    |   |       |-- nbdkit-ones-plugin.la
    |   |       |-- nbdkit-ones-plugin.so
    |   |       |-- nbdkit-partitioning-plugin.la
    |   |       |-- nbdkit-partitioning-plugin.so
    |   |       |-- nbdkit-pattern-plugin.la
    |   |       |-- nbdkit-pattern-plugin.so
    |   |       |-- nbdkit-random-plugin.la
    |   |       |-- nbdkit-random-plugin.so
    |   |       |-- nbdkit-sh-plugin.la
    |   |       |-- nbdkit-sh-plugin.so
    |   |       |-- nbdkit-sparse-random-plugin.la
    |   |       |-- nbdkit-sparse-random-plugin.so
    |   |       |-- nbdkit-split-plugin.la
    |   |       |-- nbdkit-split-plugin.so
    |   |       |-- nbdkit-tmpdisk-plugin.la
    |   |       |-- nbdkit-tmpdisk-plugin.so
    |   |       |-- nbdkit-vddk-plugin.la
    |   |       |-- nbdkit-vddk-plugin.so
    |   |       |-- nbdkit-zero-plugin.la
    |   |       `-- nbdkit-zero-plugin.so
    |   `-- pkgconfig
    |       `-- nbdkit.pc
    `-- sbin
        `-- nbdkit

8 directories, 134 files
```