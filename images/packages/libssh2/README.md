```
# libssh2
/libssh2
[drwxr-xr-x  4.0K]  ./
└── [drwxr-xr-x  4.0K]  usr/
    ├── [drwxr-xr-x  4.0K]  include/
    │   ├── [-rw-r--r--   59K]  libssh2.h
    │   ├── [-rw-r--r--  4.8K]  libssh2_publickey.h
    │   └── [-rw-r--r--   17K]  libssh2_sftp.h
    ├── [drwxr-xr-x  4.0K]  lib64/
    │   ├── [drwxr-xr-x  4.0K]  cmake/
    │   │   └── [drwxr-xr-x  4.0K]  libssh2/
    │   │       ├── [-rw-r--r--  2.0K]  FindLibgcrypt.cmake
    │   │       ├── [-rw-r--r--  2.3K]  FindMbedTLS.cmake
    │   │       ├── [-rw-r--r--  2.0K]  FindWolfSSL.cmake
    │   │       ├── [-rw-r--r--  2.7K]  libssh2-config-version.cmake
    │   │       ├── [-rw-r--r--   830]  libssh2-config.cmake
    │   │       ├── [-rw-r--r--   893]  libssh2-targets-release.cmake
    │   │       └── [-rw-r--r--  4.4K]  libssh2-targets.cmake
    │   ├── [lrwxrwxrwx    12]  libssh2.so -> libssh2.so.1*
    │   ├── [lrwxrwxrwx    16]  libssh2.so.1 -> libssh2.so.1.0.1*
    │   ├── [-rwxr-xr-x  284K]  libssh2.so.1.0.1*
    │   └── [drwxr-xr-x  4.0K]  pkgconfig/
    │       └── [-rw-r--r--   614]  libssh2.pc
    └── [drwxr-xr-x  4.0K]  share/
        ├── [drwxr-xr-x  4.0K]  doc/
        │   └── [drwxr-xr-x  4.0K]  libssh2/
        │       ├── [-rw-r--r--  1.2K]  AUTHORS
        │       ├── [-rw-r--r--   847]  BINDINGS.md
        │       ├── [-rw-r--r--  1.9K]  COPYING
        │       ├── [-rw-r--r--   267]  HACKING.md
        │       ├── [-rw-r--r--    86]  NEWS
        │       ├── [-rw-r--r--   465]  README
        │       └── [-rw-r--r--   21K]  RELEASE-NOTES
        └── [drwxr-xr-x  4.0K]  man/
            └── [drwxr-xr-x   12K]  man3/
                ├── [-rw-r--r--   620]  libssh2_agent_connect.3
                ├── [-rw-r--r--   533]  libssh2_agent_disconnect.3
                ├── [-rw-r--r--   514]  libssh2_agent_free.3
                ├── [-rw-r--r--  1.2K]  libssh2_agent_get_identity.3
                ├── [-rw-r--r--   613]  libssh2_agent_get_identity_path.3
                ├── [-rw-r--r--   902]  libssh2_agent_init.3
                ├── [-rw-r--r--   714]  libssh2_agent_list_identities.3
                ├── [-rw-r--r--   599]  libssh2_agent_set_identity_path.3
                ├── [-rw-r--r--  1.8K]  libssh2_agent_sign.3
                ├── [-rw-r--r--   955]  libssh2_agent_userauth.3
                ├── [-rw-r--r--  1.2K]  libssh2_banner_set.3
                ├── [-rw-r--r--  1.0K]  libssh2_base64_decode.3
                ├── [-rw-r--r--  1.1K]  libssh2_channel_close.3
                ├── [-rw-r--r--  1.3K]  libssh2_channel_direct_streamlocal_ex.3
                ├── [-rw-r--r--   782]  libssh2_channel_direct_tcpip.3
                ├── [-rw-r--r--  1.4K]  libssh2_channel_direct_tcpip_ex.3
                ├── [-rw-r--r--   609]  libssh2_channel_eof.3
                ├── [-rw-r--r--   708]  libssh2_channel_exec.3
                ├── [-rw-r--r--   655]  libssh2_channel_flush.3
                ├── [-rw-r--r--  1.1K]  libssh2_channel_flush_ex.3
                ├── [-rw-r--r--   676]  libssh2_channel_flush_stderr.3
                ├── [-rw-r--r--   839]  libssh2_channel_forward_accept.3
                ├── [-rw-r--r--   983]  libssh2_channel_forward_cancel.3
                ├── [-rw-r--r--   737]  libssh2_channel_forward_listen.3
                ├── [-rw-r--r--  1.9K]  libssh2_channel_forward_listen_ex.3
                ├── [-rw-r--r--   884]  libssh2_channel_free.3
                ├── [-rw-r--r--  1.6K]  libssh2_channel_get_exit_signal.3
                ├── [-rw-r--r--   744]  libssh2_channel_get_exit_status.3
                ├── [-rw-r--r--  1.3K]  libssh2_channel_handle_extended_data.3
                ├── [-rw-r--r--  1.3K]  libssh2_channel_handle_extended_data2.3
                ├── [-rw-r--r--   933]  libssh2_channel_ignore_extended_data.3
                ├── [-rw-r--r--  2.0K]  libssh2_channel_open_ex.3
                ├── [-rw-r--r--   685]  libssh2_channel_open_session.3
                ├── [-rw-r--r--  1.4K]  libssh2_channel_process_startup.3
                ├── [-rw-r--r--   698]  libssh2_channel_read.3
                ├── [-rw-r--r--  1.8K]  libssh2_channel_read_ex.3
                ├── [-rw-r--r--   726]  libssh2_channel_read_stderr.3
                ├── [-rw-r--r--  1.4K]  libssh2_channel_receive_window_adjust.3
                ├── [-rw-r--r--  1.2K]  libssh2_channel_receive_window_adjust2.3
                ├── [-rw-r--r--   966]  libssh2_channel_request_auth_agent.3
                ├── [-rw-r--r--   721]  libssh2_channel_request_pty.3
                ├── [-rw-r--r--  1.7K]  libssh2_channel_request_pty_ex.3
                ├── [-rw-r--r--   799]  libssh2_channel_request_pty_size.3
                ├── [-rw-r--r--   307]  libssh2_channel_request_pty_size_ex.3
                ├── [-rw-r--r--   850]  libssh2_channel_send_eof.3
                ├── [-rw-r--r--   821]  libssh2_channel_set_blocking.3
                ├── [-rw-r--r--   726]  libssh2_channel_setenv.3
                ├── [-rw-r--r--  1.5K]  libssh2_channel_setenv_ex.3
                ├── [-rw-r--r--   690]  libssh2_channel_shell.3
                ├── [-rw-r--r--  1.1K]  libssh2_channel_signal_ex.3
                ├── [-rw-r--r--   725]  libssh2_channel_subsystem.3
                ├── [-rw-r--r--   856]  libssh2_channel_wait_closed.3
                ├── [-rw-r--r--   687]  libssh2_channel_wait_eof.3
                ├── [-rw-r--r--   713]  libssh2_channel_window_read.3
                ├── [-rw-r--r--  1.0K]  libssh2_channel_window_read_ex.3
                ├── [-rw-r--r--   721]  libssh2_channel_window_write.3
                ├── [-rw-r--r--   912]  libssh2_channel_window_write_ex.3
                ├── [-rw-r--r--   707]  libssh2_channel_write.3
                ├── [-rw-r--r--  2.1K]  libssh2_channel_write_ex.3
                ├── [-rw-r--r--   735]  libssh2_channel_write_stderr.3
                ├── [-rw-r--r--   690]  libssh2_channel_x11_req.3
                ├── [-rw-r--r--  1.6K]  libssh2_channel_x11_req_ex.3
                ├── [-rw-r--r--   432]  libssh2_crypto_engine.3
                ├── [-rw-r--r--   431]  libssh2_exit.3
                ├── [-rw-r--r--   706]  libssh2_free.3
                ├── [-rw-r--r--  1.1K]  libssh2_hostkey_hash.3
                ├── [-rw-r--r--   672]  libssh2_init.3
                ├── [-rw-r--r--  1.0K]  libssh2_keepalive_config.3
                ├── [-rw-r--r--   701]  libssh2_keepalive_send.3
                ├── [-rw-r--r--  2.5K]  libssh2_knownhost_add.3
                ├── [-rw-r--r--  2.6K]  libssh2_knownhost_addc.3
                ├── [-rw-r--r--  2.2K]  libssh2_knownhost_check.3
                ├── [-rw-r--r--  2.4K]  libssh2_knownhost_checkp.3
                ├── [-rw-r--r--   848]  libssh2_knownhost_del.3
                ├── [-rw-r--r--   519]  libssh2_knownhost_free.3
                ├── [-rw-r--r--  1.1K]  libssh2_knownhost_get.3
                ├── [-rw-r--r--   874]  libssh2_knownhost_init.3
                ├── [-rw-r--r--  1005]  libssh2_knownhost_readfile.3
                ├── [-rw-r--r--   998]  libssh2_knownhost_readline.3
                ├── [-rw-r--r--   895]  libssh2_knownhost_writefile.3
                ├── [-rw-r--r--  1.6K]  libssh2_knownhost_writeline.3
                ├── [-rw-r--r--  1.0K]  libssh2_poll.3
                ├── [-rw-r--r--   754]  libssh2_poll_channel_read.3
                ├── [-rw-r--r--   904]  libssh2_publickey_add.3
                ├── [-rw-r--r--   867]  libssh2_publickey_add_ex.3
                ├── [-rw-r--r--   321]  libssh2_publickey_init.3
                ├── [-rw-r--r--   333]  libssh2_publickey_list_fetch.3
                ├── [-rw-r--r--   331]  libssh2_publickey_list_free.3
                ├── [-rw-r--r--   830]  libssh2_publickey_remove.3
                ├── [-rw-r--r--   346]  libssh2_publickey_remove_ex.3
                ├── [-rw-r--r--   334]  libssh2_publickey_shutdown.3
                ├── [-rw-r--r--  1.1K]  libssh2_scp_recv.3
                ├── [-rw-r--r--  1.0K]  libssh2_scp_recv2.3
                ├── [-rw-r--r--   687]  libssh2_scp_send.3
                ├── [-rw-r--r--  1.6K]  libssh2_scp_send64.3
                ├── [-rw-r--r--  1.6K]  libssh2_scp_send_ex.3
                ├── [-rw-r--r--   832]  libssh2_session_abstract.3
                ├── [-rw-r--r--   897]  libssh2_session_banner_get.3
                ├── [-rw-r--r--  1.2K]  libssh2_session_banner_set.3
                ├── [-rw-r--r--  1.3K]  libssh2_session_block_directions.3
                ├── [-rw-r--r--  1005]  libssh2_session_callback_set.3
                ├── [-rw-r--r--  5.0K]  libssh2_session_callback_set2.3
                ├── [-rw-r--r--   720]  libssh2_session_disconnect.3
                ├── [-rw-r--r--  1.5K]  libssh2_session_disconnect_ex.3
                ├── [-rw-r--r--  1.0K]  libssh2_session_flag.3
                ├── [-rw-r--r--   766]  libssh2_session_free.3
                ├── [-rw-r--r--   578]  libssh2_session_get_blocking.3
                ├── [-rw-r--r--   738]  libssh2_session_get_read_timeout.3
                ├── [-rw-r--r--   744]  libssh2_session_get_timeout.3
                ├── [-rw-r--r--  1.4K]  libssh2_session_handshake.3
                ├── [-rw-r--r--   802]  libssh2_session_hostkey.3
                ├── [-rw-r--r--   641]  libssh2_session_init.3
                ├── [-rw-r--r--  1.9K]  libssh2_session_init_ex.3
                ├── [-rw-r--r--   652]  libssh2_session_last_errno.3
                ├── [-rw-r--r--  1.2K]  libssh2_session_last_error.3
                ├── [-rw-r--r--  1.5K]  libssh2_session_method_pref.3
                ├── [-rw-r--r--  1.1K]  libssh2_session_methods.3
                ├── [-rw-r--r--  1.1K]  libssh2_session_set_blocking.3
                ├── [-rw-r--r--  1.1K]  libssh2_session_set_last_error.3
                ├── [-rw-r--r--   759]  libssh2_session_set_read_timeout.3
                ├── [-rw-r--r--   750]  libssh2_session_set_timeout.3
                ├── [-rw-r--r--  1.4K]  libssh2_session_startup.3
                ├── [-rw-r--r--  2.8K]  libssh2_session_supported_algs.3
                ├── [-rw-r--r--   680]  libssh2_sftp_close.3
                ├── [-rw-r--r--  1.5K]  libssh2_sftp_close_handle.3
                ├── [-rw-r--r--   688]  libssh2_sftp_closedir.3
                ├── [-rw-r--r--   723]  libssh2_sftp_fsetstat.3
                ├── [-rw-r--r--   711]  libssh2_sftp_fstat.3
                ├── [-rw-r--r--  3.6K]  libssh2_sftp_fstat_ex.3
                ├── [-rw-r--r--   134]  libssh2_sftp_fstatvfs.3
                ├── [-rw-r--r--  1.4K]  libssh2_sftp_fsync.3
                ├── [-rw-r--r--   649]  libssh2_sftp_get_channel.3
                ├── [-rw-r--r--  1.4K]  libssh2_sftp_init.3
                ├── [-rw-r--r--   851]  libssh2_sftp_last_error.3
                ├── [-rw-r--r--   715]  libssh2_sftp_lstat.3
                ├── [-rw-r--r--   699]  libssh2_sftp_mkdir.3
                ├── [-rw-r--r--  1.5K]  libssh2_sftp_mkdir_ex.3
                ├── [-rw-r--r--   751]  libssh2_sftp_open.3
                ├── [-rw-r--r--  2.4K]  libssh2_sftp_open_ex.3
                ├── [-rw-r--r--  2.7K]  libssh2_sftp_open_ex_r.3
                ├── [-rw-r--r--   824]  libssh2_sftp_open_r.3
                ├── [-rw-r--r--   688]  libssh2_sftp_opendir.3
                ├── [-rw-r--r--   816]  libssh2_sftp_posix_rename.3
                ├── [-rw-r--r--  2.1K]  libssh2_sftp_posix_rename_ex.3
                ├── [-rw-r--r--  1.6K]  libssh2_sftp_read.3
                ├── [-rw-r--r--   786]  libssh2_sftp_readdir.3
                ├── [-rw-r--r--  2.7K]  libssh2_sftp_readdir_ex.3
                ├── [-rw-r--r--   841]  libssh2_sftp_readlink.3
                ├── [-rw-r--r--   841]  libssh2_sftp_realpath.3
                ├── [-rw-r--r--   762]  libssh2_sftp_rename.3
                ├── [-rw-r--r--  2.2K]  libssh2_sftp_rename_ex.3
                ├── [-rw-r--r--   653]  libssh2_sftp_rewind.3
                ├── [-rw-r--r--   699]  libssh2_sftp_rmdir.3
                ├── [-rw-r--r--  1.3K]  libssh2_sftp_rmdir_ex.3
                ├── [-rw-r--r--  1.0K]  libssh2_sftp_seek.3
                ├── [-rw-r--r--  1.1K]  libssh2_sftp_seek64.3
                ├── [-rw-r--r--   722]  libssh2_sftp_setstat.3
                ├── [-rw-r--r--   764]  libssh2_sftp_shutdown.3
                ├── [-rw-r--r--   716]  libssh2_sftp_stat.3
                ├── [-rw-r--r--  2.4K]  libssh2_sftp_stat_ex.3
                ├── [-rw-r--r--  2.7K]  libssh2_sftp_statvfs.3
                ├── [-rw-r--r--   810]  libssh2_sftp_symlink.3
                ├── [-rw-r--r--  2.8K]  libssh2_sftp_symlink_ex.3
                ├── [-rw-r--r--   755]  libssh2_sftp_tell.3
                ├── [-rw-r--r--   721]  libssh2_sftp_tell64.3
                ├── [-rw-r--r--   681]  libssh2_sftp_unlink.3
                ├── [-rw-r--r--  1.3K]  libssh2_sftp_unlink_ex.3
                ├── [-rw-r--r--  3.0K]  libssh2_sftp_write.3
                ├── [-rw-r--r--  3.1K]  libssh2_sign_sk.3
                ├── [-rw-r--r--  1.1K]  libssh2_trace.3
                ├── [-rw-r--r--  1.4K]  libssh2_trace_sethandler.3
                ├── [-rw-r--r--   631]  libssh2_userauth_authenticated.3
                ├── [-rw-r--r--  1.2K]  libssh2_userauth_banner.3
                ├── [-rw-r--r--  1.0K]  libssh2_userauth_hostbased_fromfile.3
                ├── [-rw-r--r--   318]  libssh2_userauth_hostbased_fromfile_ex.3
                ├── [-rw-r--r--   921]  libssh2_userauth_keyboard_interactive.3
                ├── [-rw-r--r--  2.4K]  libssh2_userauth_keyboard_interactive_ex.3
                ├── [-rw-r--r--  1.7K]  libssh2_userauth_list.3
                ├── [-rw-r--r--   783]  libssh2_userauth_password.3
                ├── [-rw-r--r--  2.3K]  libssh2_userauth_password_ex.3
                ├── [-rw-r--r--  1007]  libssh2_userauth_publickey.3
                ├── [-rw-r--r--  1004]  libssh2_userauth_publickey_fromfile.3
                ├── [-rw-r--r--  2.2K]  libssh2_userauth_publickey_fromfile_ex.3
                ├── [-rw-r--r--  2.4K]  libssh2_userauth_publickey_frommemory.3
                ├── [-rw-r--r--  5.3K]  libssh2_userauth_publickey_sk.3
                └── [-rw-r--r--  1.3K]  libssh2_version.3

12 directories, 207 files
```
