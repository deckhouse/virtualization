# Patches

## `001-disable-ro-and-admin-servers.patch`

This patch introduces new flags to enhance the security and control of QEMU services:

- Adds `--no-admin-srv` and `--no-ro-srv` flags to `virtqemud`.
- Adds `--no-admin-srv` flag to `virtlogd`.

These flags allow disabling the read-only and admin servers for `virtqemud` and the admin server for `virtlogd`, respectively, providing better control over the services and reducing potential attack surfaces.

### Affected Sockets

When all flags are set, the following sockets will be disabled:
- `/var/run/libvirt/virtlogd-admin-sock`
- `/var/run/libvirt/virtqemud-admin-sock`
- `/var/run/libvirt/virtqemud-sock-ro`
