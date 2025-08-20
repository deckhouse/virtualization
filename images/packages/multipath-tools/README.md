# multipath-tools
/multipath-tools
```
`-- usr
    |-- include
    |   |-- libdmmp
    |   |   `-- libdmmp.h
    |   |-- mpath_cmd.h
    |   |-- mpath_persist.h
    |   `-- mpath_valid.h
    |-- lib
    |   |-- modules-load.d
    |   |-- systemd
    |   |   `-- system
    |   |       |-- multipathd.service
    |   |       `-- multipathd.socket
    |   |-- tmpfiles.d
    |   |   `-- multipath.conf
    |   `-- udev
    |       |-- kpartx_id
    |       `-- rules.d
    |           |-- 11-dm-mpath.rules
    |           |-- 11-dm-parts.rules
    |           |-- 56-multipath.rules
    |           |-- 66-kpartx.rules
    |           |-- 68-del-part-nodes.rules
    |           `-- 99-z-dm-mpath-late.rules
    |-- lib64
    |   |-- libdmmp.so -> libdmmp.so.0.2.0
    |   |-- libdmmp.so.0.2.0
    |   |-- libmpathcmd.so -> libmpathcmd.so.0
    |   |-- libmpathcmd.so.0
    |   |-- libmpathpersist.so -> libmpathpersist.so.0
    |   |-- libmpathpersist.so.0
    |   |-- libmpathutil.so -> libmpathutil.so.0
    |   |-- libmpathutil.so.0
    |   |-- libmpathvalid.so -> libmpathvalid.so.0
    |   |-- libmpathvalid.so.0
    |   |-- libmultipath.so -> libmultipath.so.0
    |   |-- libmultipath.so.0
    |   |-- multipath
    |   |   |-- libcheckcciss_tur.so
    |   |   |-- libcheckdirectio.so
    |   |   |-- libcheckemc_clariion.so
    |   |   |-- libcheckhp_sw.so
    |   |   |-- libcheckrdac.so
    |   |   |-- libcheckreadsector0.so
    |   |   |-- libchecktur.so
    |   |   |-- libforeign-nvme.so
    |   |   |-- libprioalua.so
    |   |   |-- libprioana.so
    |   |   |-- libprioconst.so
    |   |   |-- libpriodatacore.so
    |   |   |-- libprioemc.so
    |   |   |-- libpriohds.so
    |   |   |-- libpriohp_sw.so
    |   |   |-- libprioiet.so
    |   |   |-- libprioontap.so
    |   |   |-- libpriopath_latency.so
    |   |   |-- libpriorandom.so
    |   |   |-- libpriordac.so
    |   |   |-- libpriosysfs.so
    |   |   `-- libprioweightedpath.so
    |   `-- pkgconfig
    |       `-- libdmmp.pc
    |-- sbin
    |   |-- kpartx
    |   |-- mpathpersist
    |   |-- multipath
    |   |-- multipathc
    |   `-- multipathd
    `-- share
        `-- man
            |-- man3
            |   |-- dmmp_context_free.3
            |   |-- dmmp_context_log_func_set.3
            |   |-- dmmp_context_log_priority_get.3
            |   |-- dmmp_context_log_priority_set.3
            |   |-- dmmp_context_new.3
            |   |-- dmmp_context_timeout_get.3
            |   |-- dmmp_context_timeout_set.3
            |   |-- dmmp_context_userdata_get.3
            |   |-- dmmp_context_userdata_set.3
            |   |-- dmmp_flush_mpath.3
            |   |-- dmmp_last_error_msg.3
            |   |-- dmmp_log_priority_str.3
            |   |-- dmmp_mpath_array_free.3
            |   |-- dmmp_mpath_array_get.3
            |   |-- dmmp_mpath_kdev_name_get.3
            |   |-- dmmp_mpath_name_get.3
            |   |-- dmmp_mpath_wwid_get.3
            |   |-- dmmp_path_array_get.3
            |   |-- dmmp_path_blk_name_get.3
            |   |-- dmmp_path_group_array_get.3
            |   |-- dmmp_path_group_id_get.3
            |   |-- dmmp_path_group_priority_get.3
            |   |-- dmmp_path_group_selector_get.3
            |   |-- dmmp_path_group_status_get.3
            |   |-- dmmp_path_group_status_str.3
            |   |-- dmmp_path_status_get.3
            |   |-- dmmp_path_status_str.3
            |   |-- dmmp_reconfig.3
            |   |-- dmmp_strerror.3
            |   |-- libdmmp.h.3
            |   |-- mpath_persistent_reserve_in.3
            |   `-- mpath_persistent_reserve_out.3
            |-- man5
            |   `-- multipath.conf.5
            `-- man8
                |-- kpartx.8
                |-- mpathpersist.8
                |-- multipath.8
                |-- multipathc.8
                `-- multipathd.8

20 directories, 92 files
```