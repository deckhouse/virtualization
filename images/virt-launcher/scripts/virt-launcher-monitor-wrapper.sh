#!/bin/bash

# virt-launcher-monitor execution interceptor:
# - Run qemu customizer as a child process.
# - Exec virt-launcher-monitor in-place to start usual virt-launcher.

echo '{"msg":"Start domain monitor daemon", "level":"info","component":"virt-launcher-monitor-wrapper"}'
nohup bash /scripts/domain-monitor.sh & 2>&1 > /var/log/domain-monitor-daemon.log

# Pass all arguments to the original virt-launcher-monitor.
if [[ ! -f /usr/bin/virt-launcher-monitor-orig ]]; then
  echo '{"msg":"Target /usr/bin/virt-launcher-monitor-orig is absent", "level":"info","component":"virt-launcher-monitor-wrapper"}'
  exit 1
fi
echo '{"msg":"Exec original virt-launcher-monitor", "level":"info","component":"virt-launcher-monitor-wrapper"}'
exec /usr/bin/virt-launcher-monitor-orig "$@"
