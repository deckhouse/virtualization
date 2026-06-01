# Patches

## 001-disable-ro-and-admin-servers.patch

This patch introduces new flags to enhance the security and control of QEMU services:

- Adds `--no-admin-srv` and `--no-ro-srv` flags to `virtqemud`.
- Adds `--no-admin-srv` flag to `virtlogd`.

These flags allow disabling the read-only and admin servers for `virtqemud` and the admin server for `virtlogd`, respectively, providing better control over the services and reducing potential attack surfaces.

### Affected Sockets

When all flags are set, the following sockets will be disabled:
- `/var/run/libvirt/virtlogd-admin-sock`
- `/var/run/libvirt/virtqemud-admin-sock`
- `/var/run/libvirt/virtqemud-sock-ro`

## 002-auth-pid-restriction.patch

This patch introduces a new security feature for **virtqemud** by utilizing an environment variable to restrict socket connections:

- The `LIBVIRT_UNIX_SOCKET_AUTH_PID` environment variable is used to specify the **process ID (PID)** that is allowed to connect to `virtqemud`.

When this environment variable is set, `virtqemud` will **only accept socket connections from the specified PID**, improving security by ensuring that only the intended process can communicate with the daemon.

### Affected Behavior

- If the `LIBVIRT_UNIX_SOCKET_AUTH_PID` environment variable is set with a valid PID, `virtqemud` will check the PID of incoming connection attempts. Only the process with the specified PID will be allowed to communicate over the socket.
- Any connection attempt from a different process will be rejected.
- If the environment variable is **not set**, `virtqemud` will function as before, accepting all connections without PID-based restrictions.

This feature enhances security by preventing unauthorized access to the socket and mitigating the risk of privilege escalation attacks. It provides a way to control access to the daemon based on the PID of the connecting process, without the need for additional command-line flags.

## 003-treat-getpeercon-eintval-as-success.patch

`getpeercon` from libselinux uses `getsockopt()` syscall. Some implementations of `getsockopts()` return `EINVAL` errno for unsupported valopt argument instead of `ENOPROTOOPT` errno. This fix makes libvirt work with such broken implementations.

## 004-fix-migration-cancel-concluded-mirror-jobs.patch

Fixes a non-shared storage migration cancel race in libvirt/QEMU.

After `AbortJob`, mirror block jobs may become `concluded` in QEMU while libvirt still polls synchronous migration mirrors. The patch refreshes monitor job status and drives pending terminal transitions so concluded jobs are dismissed/unregistered and do not block subsequent migrations.

## 005-usb-startup-policy-in-containers.patch

Fixes `startupPolicy='optional'` for USB hostdev in containerized environments.

In containers, sysfs (`/sys/bus/usb/devices/`) is mounted from the host kernel and exposes all USB devices regardless of mount namespace isolation. This causes `virUSBDeviceSearch` to report a device as available even when the corresponding `/dev/bus/usb/` node is not present in the container's mount namespace. As a result, `startupPolicy='optional'` does not remove the missing hostdev from the domain XML, and QEMU fails to start.

The patch adds a `virFileExists` check on the device node path after sysfs discovery. If the node is missing, the device is skipped so that callers see an empty result and handle the absence gracefully.
