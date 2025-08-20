# libnbd
/libnbd
```
`-- usr
    |-- bin
    |   |-- nbdcopy
    |   |-- nbddump
    |   |-- nbdfuse
    |   |-- nbdinfo
    |   `-- nbdublk
    |-- include
    |   `-- libnbd.h
    |-- lib64
    |   |-- libnbd.la
    |   |-- libnbd.so -> libnbd.so.0.0.0
    |   |-- libnbd.so.0 -> libnbd.so.0.0.0
    |   |-- libnbd.so.0.0.0
    |   |-- ocaml
    |   |   |-- nbd
    |   |   |   |-- META
    |   |   |   |-- NBD.cmi
    |   |   |   |-- NBD.cmx
    |   |   |   |-- NBD.mli
    |   |   |   |-- libmlnbd.a
    |   |   |   |-- mlnbd.a
    |   |   |   |-- mlnbd.cma
    |   |   |   `-- mlnbd.cmxa
    |   |   `-- stublibs
    |   |       |-- dllmlnbd.so
    |   |       `-- dllmlnbd.so.owner
    |   `-- pkgconfig
    |       `-- libnbd.pc
    `-- share
        |-- bash-completion
        |   `-- completions
        |       |-- nbdcopy
        |       |-- nbddump
        |       |-- nbdfuse
        |       |-- nbdinfo
        |       |-- nbdsh
        |       `-- nbdublk
        `-- man
            |-- man1
            |   |-- libnbd-release-notes-1.10.1
            |   |-- libnbd-release-notes-1.12.1
            |   |-- libnbd-release-notes-1.14.1
            |   |-- libnbd-release-notes-1.16.1
            |   |-- libnbd-release-notes-1.18.1
            |   |-- libnbd-release-notes-1.2.1
            |   |-- libnbd-release-notes-1.20.1
            |   |-- libnbd-release-notes-1.22.1
            |   |-- libnbd-release-notes-1.4.1
            |   |-- libnbd-release-notes-1.6.1
            |   |-- libnbd-release-notes-1.8.1
            |   |-- nbdcopy.1
            |   |-- nbddump.1
            |   |-- nbdfuse.1
            |   |-- nbdinfo.1
            |   `-- nbdublk.1
            `-- man3
                |-- NBD.3
                |-- NBD.ALLOW_TRANSPORT.3
                |-- NBD.Buffer.3
                |-- NBD.CMD_FLAG.3
                |-- NBD.HANDSHAKE_FLAG.3
                |-- NBD.SHUTDOWN.3
                |-- NBD.SIZE.3
                |-- NBD.STRICT.3
                |-- NBD.TLS.3
                |-- libnbd-golang.3
                |-- libnbd-ocaml.3
                |-- libnbd-security.3
                |-- libnbd.3
                |-- nbd_add_meta_context.3
                |-- nbd_aio_block_status.3
                |-- nbd_aio_block_status_64.3
                |-- nbd_aio_block_status_filter.3
                |-- nbd_aio_cache.3
                |-- nbd_aio_command_completed.3
                |-- nbd_aio_connect.3
                |-- nbd_aio_connect_command.3
                |-- nbd_aio_connect_socket.3
                |-- nbd_aio_connect_systemd_socket_activation.3
                |-- nbd_aio_connect_tcp.3
                |-- nbd_aio_connect_unix.3
                |-- nbd_aio_connect_uri.3
                |-- nbd_aio_connect_vsock.3
                |-- nbd_aio_disconnect.3
                |-- nbd_aio_flush.3
                |-- nbd_aio_get_direction.3
                |-- nbd_aio_get_fd.3
                |-- nbd_aio_in_flight.3
                |-- nbd_aio_is_closed.3
                |-- nbd_aio_is_connecting.3
                |-- nbd_aio_is_created.3
                |-- nbd_aio_is_dead.3
                |-- nbd_aio_is_negotiating.3
                |-- nbd_aio_is_processing.3
                |-- nbd_aio_is_ready.3
                |-- nbd_aio_notify_read.3
                |-- nbd_aio_notify_write.3
                |-- nbd_aio_opt_abort.3
                |-- nbd_aio_opt_extended_headers.3
                |-- nbd_aio_opt_go.3
                |-- nbd_aio_opt_info.3
                |-- nbd_aio_opt_list.3
                |-- nbd_aio_opt_list_meta_context.3
                |-- nbd_aio_opt_list_meta_context_queries.3
                |-- nbd_aio_opt_set_meta_context.3
                |-- nbd_aio_opt_set_meta_context_queries.3
                |-- nbd_aio_opt_starttls.3
                |-- nbd_aio_opt_structured_reply.3
                |-- nbd_aio_peek_command_completed.3
                |-- nbd_aio_pread.3
                |-- nbd_aio_pread_structured.3
                |-- nbd_aio_pwrite.3
                |-- nbd_aio_trim.3
                |-- nbd_aio_zero.3
                |-- nbd_block_status.3
                |-- nbd_block_status_64.3
                |-- nbd_block_status_filter.3
                |-- nbd_cache.3
                |-- nbd_can_block_status_payload.3
                |-- nbd_can_cache.3
                |-- nbd_can_df.3
                |-- nbd_can_fast_zero.3
                |-- nbd_can_flush.3
                |-- nbd_can_fua.3
                |-- nbd_can_meta_context.3
                |-- nbd_can_multi_conn.3
                |-- nbd_can_trim.3
                |-- nbd_can_zero.3
                |-- nbd_clear_debug_callback.3
                |-- nbd_clear_meta_contexts.3
                |-- nbd_close.3
                |-- nbd_connect_command.3
                |-- nbd_connect_socket.3
                |-- nbd_connect_systemd_socket_activation.3
                |-- nbd_connect_tcp.3
                |-- nbd_connect_unix.3
                |-- nbd_connect_uri.3
                |-- nbd_connect_vsock.3
                |-- nbd_connection_state.3
                |-- nbd_create.3
                |-- nbd_flush.3
                |-- nbd_get_block_size.3
                |-- nbd_get_canonical_export_name.3
                |-- nbd_get_debug.3
                |-- nbd_get_errno.3
                |-- nbd_get_error.3
                |-- nbd_get_export_description.3
                |-- nbd_get_export_name.3
                |-- nbd_get_extended_headers_negotiated.3
                |-- nbd_get_full_info.3
                |-- nbd_get_handle_name.3
                |-- nbd_get_handle_size.3
                |-- nbd_get_handshake_flags.3
                |-- nbd_get_meta_context.3
                |-- nbd_get_nr_meta_contexts.3
                |-- nbd_get_opt_mode.3
                |-- nbd_get_package_name.3
                |-- nbd_get_pread_initialize.3
                |-- nbd_get_private_data.3
                |-- nbd_get_protocol.3
                |-- nbd_get_request_block_size.3
                |-- nbd_get_request_extended_headers.3
                |-- nbd_get_request_meta_context.3
                |-- nbd_get_request_structured_replies.3
                |-- nbd_get_size.3
                |-- nbd_get_socket_activation_name.3
                |-- nbd_get_strict_mode.3
                |-- nbd_get_structured_replies_negotiated.3
                |-- nbd_get_subprocess_pid.3
                |-- nbd_get_tls.3
                |-- nbd_get_tls_hostname.3
                |-- nbd_get_tls_negotiated.3
                |-- nbd_get_tls_username.3
                |-- nbd_get_tls_verify_peer.3
                |-- nbd_get_uri.3
                |-- nbd_get_version.3
                |-- nbd_get_version_extra.3
                |-- nbd_is_read_only.3
                |-- nbd_is_rotational.3
                |-- nbd_is_uri.3
                |-- nbd_kill_subprocess.3
                |-- nbd_opt_abort.3
                |-- nbd_opt_extended_headers.3
                |-- nbd_opt_go.3
                |-- nbd_opt_info.3
                |-- nbd_opt_list.3
                |-- nbd_opt_list_meta_context.3
                |-- nbd_opt_list_meta_context_queries.3
                |-- nbd_opt_set_meta_context.3
                |-- nbd_opt_set_meta_context_queries.3
                |-- nbd_opt_starttls.3
                |-- nbd_opt_structured_reply.3
                |-- nbd_poll.3
                |-- nbd_poll2.3
                |-- nbd_pread.3
                |-- nbd_pread_structured.3
                |-- nbd_pwrite.3
                |-- nbd_set_debug.3
                |-- nbd_set_debug_callback.3
                |-- nbd_set_export_name.3
                |-- nbd_set_full_info.3
                |-- nbd_set_handle_name.3
                |-- nbd_set_handshake_flags.3
                |-- nbd_set_opt_mode.3
                |-- nbd_set_pread_initialize.3
                |-- nbd_set_private_data.3
                |-- nbd_set_request_block_size.3
                |-- nbd_set_request_extended_headers.3
                |-- nbd_set_request_meta_context.3
                |-- nbd_set_request_structured_replies.3
                |-- nbd_set_socket_activation_name.3
                |-- nbd_set_strict_mode.3
                |-- nbd_set_tls.3
                |-- nbd_set_tls_certificates.3
                |-- nbd_set_tls_hostname.3
                |-- nbd_set_tls_psk_file.3
                |-- nbd_set_tls_username.3
                |-- nbd_set_tls_verify_peer.3
                |-- nbd_set_uri_allow_local_file.3
                |-- nbd_set_uri_allow_tls.3
                |-- nbd_set_uri_allow_transports.3
                |-- nbd_shutdown.3
                |-- nbd_stats_bytes_received.3
                |-- nbd_stats_bytes_sent.3
                |-- nbd_stats_chunks_received.3
                |-- nbd_stats_chunks_sent.3
                |-- nbd_supports_tls.3
                |-- nbd_supports_uri.3
                |-- nbd_supports_vsock.3
                |-- nbd_trim.3
                `-- nbd_zero.3

15 directories, 218 files
```