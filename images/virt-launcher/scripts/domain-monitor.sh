#!/bin/bash

set -eo pipefail

# Wait for qemu-kvm process
vmName=
while true ; do
  vmName=$(virsh list --name || true)
  if [[ -n $vmName ]]; then
    break
  fi
  sleep 1
done

# Set action as libvirt will do for <on_restart>destroy</on_restart>.
echo "Set reboot action to shutdown for domain $vmName"
virsh qemu-monitor-command $vmName '{"execute": "set-action", "arguments":{"reboot":"shutdown"}}'


# Redirect events to termination logs
echo "Monitor domain $vmName events"
virsh qemu-monitor-event --domain $vmName --loop --event SHUTDOWN > /dev/termination-log
