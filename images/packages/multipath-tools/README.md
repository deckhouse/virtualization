# multipath-tools
/multipath-tools
```
[drwxr-xr-x  4.0K]  ./
`-- [drwxr-xr-x  4.0K]  usr/
    |-- [drwxr-xr-x  4.0K]  include/
    |   |-- [drwxr-xr-x  4.0K]  libdmmp/
    |   |   `-- [-rw-r--r--   20K]  libdmmp.h
    |   |-- [-rw-r--r--  4.1K]  mpath_cmd.h
    |   |-- [-rw-r--r--   11K]  mpath_persist.h
    |   `-- [-rw-r--r--  5.1K]  mpath_valid.h
    |-- [drwxr-xr-x  4.0K]  lib/
    |   |-- [drwxr-xr-x  4.0K]  modules-load.d/
    |   |-- [drwxr-xr-x  4.0K]  systemd/
    |   |   `-- [drwxr-xr-x  4.0K]  system/
    |   |       |-- [-rw-r--r--   829]  multipathd.service
    |   |       `-- [-rw-r--r--   402]  multipathd.socket
    |   |-- [drwxr-xr-x  4.0K]  tmpfiles.d/
    |   |   `-- [-rw-r--r--    34]  multipath.conf
    |   `-- [drwxr-xr-x  4.0K]  udev/
    |       |-- [-rwxr-xr-x  2.4K]  kpartx_id*
    |       `-- [drwxr-xr-x  4.0K]  rules.d/
    |           |-- [-rw-r--r--  7.3K]  11-dm-mpath.rules
    |           |-- [-rw-r--r--  1.4K]  11-dm-parts.rules
    |           |-- [-rw-r--r--  4.3K]  56-multipath.rules
    |           |-- [-rw-r--r--  1.5K]  66-kpartx.rules
    |           |-- [-rw-r--r--  1.1K]  68-del-part-nodes.rules
    |           `-- [-rw-r--r--   299]  99-z-dm-mpath-late.rules
    |-- [drwxr-xr-x  4.0K]  lib64/
    |   |-- [lrwxrwxrwx    16]  libdmmp.so -> libdmmp.so.0.2.0*
    |   |-- [-rwxr-xr-x   99K]  libdmmp.so.0.2.0*
    |   |-- [lrwxrwxrwx    16]  libmpathcmd.so -> libmpathcmd.so.0*
    |   |-- [-rwxr-xr-x   26K]  libmpathcmd.so.0*
    |   |-- [lrwxrwxrwx    20]  libmpathpersist.so -> libmpathpersist.so.0*
    |   |-- [-rwxr-xr-x  132K]  libmpathpersist.so.0*
    |   |-- [lrwxrwxrwx    17]  libmpathutil.so -> libmpathutil.so.0*
    |   |-- [-rwxr-xr-x  150K]  libmpathutil.so.0*
    |   |-- [lrwxrwxrwx    18]  libmpathvalid.so -> libmpathvalid.so.0*
    |   |-- [-rwxr-xr-x   32K]  libmpathvalid.so.0*
    |   |-- [lrwxrwxrwx    17]  libmultipath.so -> libmultipath.so.0*
    |   |-- [-rwxr-xr-x  1.6M]  libmultipath.so.0*
    |   |-- [drwxr-xr-x  4.0K]  multipath/
    |   |   |-- [-rwxr-xr-x   23K]  libcheckcciss_tur.so*
    |   |   |-- [-rwxr-xr-x   35K]  libcheckdirectio.so*
    |   |   |-- [-rwxr-xr-x   24K]  libcheckemc_clariion.so*
    |   |   |-- [-rwxr-xr-x   22K]  libcheckhp_sw.so*
    |   |   |-- [-rwxr-xr-x   24K]  libcheckrdac.so*
    |   |   |-- [-rwxr-xr-x   18K]  libcheckreadsector0.so*
    |   |   |-- [-rwxr-xr-x   39K]  libchecktur.so*
    |   |   |-- [-rwxr-xr-x   83K]  libforeign-nvme.so*
    |   |   |-- [-rwxr-xr-x   27K]  libprioalua.so*
    |   |   |-- [-rwxr-xr-x   34K]  libprioana.so*
    |   |   |-- [-rwxr-xr-x   16K]  libprioconst.so*
    |   |   |-- [-rwxr-xr-x   27K]  libpriodatacore.so*
    |   |   |-- [-rwxr-xr-x   26K]  libprioemc.so*
    |   |   |-- [-rwxr-xr-x   29K]  libpriohds.so*
    |   |   |-- [-rwxr-xr-x   26K]  libpriohp_sw.so*
    |   |   |-- [-rwxr-xr-x   30K]  libprioiet.so*
    |   |   |-- [-rwxr-xr-x   38K]  libprioontap.so*
    |   |   |-- [-rwxr-xr-x   35K]  libpriopath_latency.so*
    |   |   |-- [-rwxr-xr-x   17K]  libpriorandom.so*
    |   |   |-- [-rwxr-xr-x   26K]  libpriordac.so*
    |   |   |-- [-rwxr-xr-x   25K]  libpriosysfs.so*
    |   |   `-- [-rwxr-xr-x   30K]  libprioweightedpath.so*
    |   `-- [drwxr-xr-x  4.0K]  pkgconfig/
    |       `-- [-rw-r--r--   188]  libdmmp.pc
    `-- [drwxr-xr-x  4.0K]  sbin/
        |-- [-rwxr-xr-x  188K]  kpartx*
        |-- [-rwxr-xr-x   81K]  mpathpersist*
        |-- [-rwxr-xr-x  100K]  multipath*
        |-- [-rwxr-xr-x   57K]  multipathc*
        `-- [-rwxr-xr-x  522K]  multipathd*

15 directories, 54 files
```
