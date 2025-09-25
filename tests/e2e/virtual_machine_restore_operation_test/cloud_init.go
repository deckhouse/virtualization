/*
Copyright 2025 Flant JSC

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

     http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package virtualmachinerestoreoperationtest

const cloudInit = `#cloud-config
users:
- name: cloud
  passwd: $6$rounds=4096$vln/.aPHBOI7BMYR$bBMkqQvuGs5Gyd/1H5DP4m9HjQSy.kgrxpaGEHwkX7KEFV8BS.HZWPitAtZ2Vd8ZqIZRqmlykRCagTgPejt1i.
  shell: /bin/bash
  sudo: ALL=(ALL) NOPASSWD:ALL
  chpasswd: { expire: False }
  lock_passwd: false
  ssh_authorized_keys:
      - ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIFxcXHmwaGnJ8scJaEN5RzklBPZpVSic4GdaAsKjQoeA your_email@example.com

runcmd:
- [bash, -c, "apt update"]
- [bash, -c, "apt install qemu-guest-agent -y"]
- [bash, -c, "systemctl enable qemu-guest-agent"]
- [bash, -c, "systemctl start qemu-guest-agent"]`
