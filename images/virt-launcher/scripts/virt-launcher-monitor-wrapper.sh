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
