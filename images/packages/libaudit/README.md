# libaudit
/libaudit
```
.
`-- usr
    |-- bin
    |   |-- aulast
    |   |-- aulastlog
    |   `-- ausyscall
    |-- etc
    |   |-- audit
    |   |   |-- audisp-filter.conf
    |   |   |-- audisp-remote.conf
    |   |   |-- audit-stop.rules
    |   |   |-- auditd.conf
    |   |   `-- plugins.d
    |   |       |-- af_unix.conf
    |   |       |-- au-remote.conf
    |   |       |-- filter.conf
    |   |       `-- syslog.conf
    |   `-- libaudit.conf
    |-- include
    |   |-- audit-records.h
    |   |-- audit_logging.h
    |   |-- auparse-defs.h
    |   |-- auparse.h
    |   `-- libaudit.h
    |-- lib
    |   |-- systemd
    |   |   `-- system
    |   |       |-- audit-rules.service
    |   |       `-- auditd.service
    |   `-- tmpfiles.d
    |       `-- audit.conf
    |-- lib64
    |   |-- libaudit.la
    |   |-- libaudit.so -> libaudit.so.1.0.0
    |   |-- libaudit.so.1 -> libaudit.so.1.0.0
    |   |-- libaudit.so.1.0.0
    |   |-- libauparse.la
    |   |-- libauparse.so -> libauparse.so.0.0.0
    |   |-- libauparse.so.0 -> libauparse.so.0.0.0
    |   |-- libauparse.so.0.0.0
    |   `-- pkgconfig
    |       |-- audit.pc
    |       `-- auparse.pc
    |-- libexec
    |   `-- initscripts
    |       `-- legacy-actions
    |           `-- auditd
    |               |-- condrestart
    |               |-- reload
    |               |-- restart
    |               |-- resume
    |               |-- rotate
    |               |-- state
    |               `-- stop
    |-- sbin
    |   |-- audisp-af_unix
    |   |-- audisp-filter
    |   |-- audisp-remote
    |   |-- audisp-syslog
    |   |-- auditctl
    |   |-- auditd
    |   |-- augenrules
    |   |-- aureport
    |   `-- ausearch
    `-- share
        |-- aclocal
        |   `-- audit.m4
        |-- audit-rules
        |   |-- 10-base-config.rules
        |   |-- 10-no-audit.rules
        |   |-- 11-loginuid.rules
        |   |-- 12-cont-fail.rules
        |   |-- 12-ignore-error.rules
        |   |-- 20-dont-audit.rules
        |   |-- 21-no32bit.rules
        |   |-- 22-ignore-chrony.rules
        |   |-- 23-ignore-filesystems.rules
        |   |-- 30-ospp-v42-1-create-failed.rules
        |   |-- 30-ospp-v42-1-create-success.rules
        |   |-- 30-ospp-v42-2-modify-failed.rules
        |   |-- 30-ospp-v42-2-modify-success.rules
        |   |-- 30-ospp-v42-3-access-failed.rules
        |   |-- 30-ospp-v42-3-access-success.rules
        |   |-- 30-ospp-v42-4-delete-failed.rules
        |   |-- 30-ospp-v42-4-delete-success.rules
        |   |-- 30-ospp-v42-5-perm-change-failed.rules
        |   |-- 30-ospp-v42-5-perm-change-success.rules
        |   |-- 30-ospp-v42-6-owner-change-failed.rules
        |   |-- 30-ospp-v42-6-owner-change-success.rules
        |   |-- 30-ospp-v42.rules
        |   |-- 30-pci-dss-v31.rules
        |   |-- 30-stig.rules
        |   |-- 31-privileged.rules
        |   |-- 32-power-abuse.rules
        |   |-- 40-local.rules
        |   |-- 41-containers.rules
        |   |-- 42-injection.rules
        |   |-- 43-module-load.rules
        |   |-- 44-installers.rules
        |   |-- 70-einval.rules
        |   |-- 71-networking.rules
        |   |-- 99-finalize.rules
        |   `-- README-rules
        `-- man
            |-- man3
            |   |-- audit_add_rule_data.3
            |   |-- audit_add_watch.3
            |   |-- audit_close.3
            |   |-- audit_delete_rule_data.3
            |   |-- audit_detect_machine.3
            |   |-- audit_encode_nv_string.3
            |   |-- audit_encode_value.3
            |   |-- audit_flag_to_name.3
            |   |-- audit_fstype_to_name.3
            |   |-- audit_get_reply.3
            |   |-- audit_get_session.3
            |   |-- audit_getloginuid.3
            |   |-- audit_is_enabled.3
            |   |-- audit_log_acct_message.3
            |   |-- audit_log_semanage_message.3
            |   |-- audit_log_user_avc_message.3
            |   |-- audit_log_user_comm_message.3
            |   |-- audit_log_user_command.3
            |   |-- audit_log_user_message.3
            |   |-- audit_name_to_action.3
            |   |-- audit_name_to_errno.3
            |   |-- audit_name_to_flag.3
            |   |-- audit_name_to_fstype.3
            |   |-- audit_name_to_syscall.3
            |   |-- audit_open.3
            |   |-- audit_request_rules_list_data.3
            |   |-- audit_request_signal_info.3
            |   |-- audit_request_status.3
            |   |-- audit_set_backlog_limit.3
            |   |-- audit_set_backlog_wait_time.3
            |   |-- audit_set_enabled.3
            |   |-- audit_set_failure.3
            |   |-- audit_set_pid.3
            |   |-- audit_set_rate_limit.3
            |   |-- audit_setloginuid.3
            |   |-- audit_syscall_to_name.3
            |   |-- audit_update_watch_perms.3
            |   |-- audit_value_needs_encoding.3
            |   |-- auparse_add_callback.3
            |   |-- auparse_destroy.3
            |   |-- auparse_feed.3
            |   |-- auparse_feed_age_events.3
            |   |-- auparse_feed_has_data.3
            |   |-- auparse_find_field.3
            |   |-- auparse_find_field_next.3
            |   |-- auparse_first_field.3
            |   |-- auparse_first_record.3
            |   |-- auparse_flush_feed.3
            |   |-- auparse_get_field_int.3
            |   |-- auparse_get_field_name.3
            |   |-- auparse_get_field_num.3
            |   |-- auparse_get_field_str.3
            |   |-- auparse_get_field_type.3
            |   |-- auparse_get_filename.3
            |   |-- auparse_get_line_number.3
            |   |-- auparse_get_milli.3
            |   |-- auparse_get_node.3
            |   |-- auparse_get_num_fields.3
            |   |-- auparse_get_num_records.3
            |   |-- auparse_get_record_num.3
            |   |-- auparse_get_record_text.3
            |   |-- auparse_get_serial.3
            |   |-- auparse_get_time.3
            |   |-- auparse_get_timestamp.3
            |   |-- auparse_get_type.3
            |   |-- auparse_get_type_name.3
            |   |-- auparse_goto_field_num.3
            |   |-- auparse_goto_record_num.3
            |   |-- auparse_init.3
            |   |-- auparse_interpret_field.3
            |   |-- auparse_metrics.3
            |   |-- auparse_new_buffer.3
            |   |-- auparse_next_event.3
            |   |-- auparse_next_field.3
            |   |-- auparse_next_record.3
            |   |-- auparse_node_compare.3
            |   |-- auparse_normalize.3
            |   |-- auparse_normalize_functions.3
            |   |-- auparse_reset.3
            |   |-- auparse_set_eoe_timeout.3
            |   |-- auparse_set_escape_mode.3
            |   |-- auparse_timestamp_compare.3
            |   |-- ausearch_add_expression.3
            |   |-- ausearch_add_interpreted_item.3
            |   |-- ausearch_add_item.3
            |   |-- ausearch_add_regex.3
            |   |-- ausearch_add_timestamp_item.3
            |   |-- ausearch_add_timestamp_item_ex.3
            |   |-- ausearch_clear.3
            |   |-- ausearch_cur_event.3
            |   |-- ausearch_next_event.3
            |   |-- ausearch_set_stop.3
            |   |-- get_auditfail_action.3
            |   `-- set_aumessage_mode.3
            |-- man5
            |   |-- audisp-remote.conf.5
            |   |-- auditd-plugins.5
            |   |-- auditd.conf.5
            |   |-- ausearch-expression.5
            |   |-- libaudit.conf.5
            |   `-- zos-remote.conf.5
            |-- man7
            |   `-- audit.rules.7
            `-- man8
                |-- audisp-af_unix.8
                |-- audisp-filter.8
                |-- audisp-remote.8
                |-- audisp-syslog.8
                |-- audispd-zos-remote.8
                |-- auditctl.8
                |-- auditd.8
                |-- augenrules.8
                |-- aulast.8
                |-- aulastlog.8
                |-- aureport.8
                |-- ausearch.8
                `-- ausyscall.8

26 directories, 196 files
```