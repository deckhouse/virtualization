## liboverride

### Problem

Some implementations of getsockopt() syscall returns EINVAL errno for unsupported
valopt argument. getsockopt() is used by getpeercon() method from libselinux.
libvirt expects ENOPROTOOPT or ENOSYS from getpeercon().

### Workaround

This library provides a wrapper for getpeercon() syscall that translates EINVAL to ENOPROTOOPT.

### Build and run

```bash
gcc -shared -fPIC -DPIC -Wall -o liboverride.so override.c -ldl
LD_PRELOAD=./liboverride.so virsh domcapabilities ...
```

### Global install

```bash
cp ld.so.preload /etc/
```
