#!/bin/bash

# Copyright 2024 Flant JSC
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#      http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

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
