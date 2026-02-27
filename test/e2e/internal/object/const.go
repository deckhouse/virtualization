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

package object

const (
	ImageURLAlpineUEFIPerf = "https://89d64382-20df-4581-8cc7-80df331f67fa.selstorage.ru/alpine/alpine-3-23-3-uefi-base.qcow2"
	// Temporary not used
	// ImageURLUbuntu         = "https://89d64382-20df-4581-8cc7-80df331f67fa.selstorage.ru/ubuntu/ubuntu-24.04-minimal-cloudimg-amd64.qcow2"
	ImageURLAlpineBIOS     = "https://89d64382-20df-4581-8cc7-80df331f67fa.selstorage.ru/alpine/alpine-3-23-3-bios-base.qcow2"
	ImageURLContainerImage = "cr.yandex/crpvs5j3nh1mi2tpithr/e2e/alpine/alpine-image:latest"
	ImageURLMinimalQCOW    = "https://89d64382-20df-4581-8cc7-80df331f67fa.selstorage.ru/test/test.qcow2"
	ImageURLMinimalISO     = "https://89d64382-20df-4581-8cc7-80df331f67fa.selstorage.ru/test/test.iso"
	Mi256                  = 256 * 1024 * 1024
	DefaultVMClass         = "generic"

	// Shared cloud-init fragments (DRY between DefaultCloudInit and PerfCloudInit).
	cloudInitBase = `#cloud-config
package_update: true
packages:
  - qemu-guest-agent
  - curl
  - bash
  - sudo
  - iputils
  - util-linux
  - iperf3
  - jq
`
	cloudInitUsers = `
users:
  - name: cloud
    # passwd: cloud
    passwd: $6$rounds=4096$vln/.aPHBOI7BMYR$bBMkqQvuGs5Gyd/1H5DP4m9HjQSy.kgrxpaGEHwkX7KEFV8BS.HZWPitAtZ2Vd8ZqIZRqmlykRCagTgPejt1i.
    shell: /bin/bash
    sudo: ALL=(ALL) NOPASSWD:ALL
    lock_passwd: false
    ssh_authorized_keys:
      # testcases
      - ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIFxcXHmwaGnJ8scJaEN5RzklBPZpVSic4GdaAsKjQoeA your_email@example.com
`
	cloudInitDefaultRuncmd = `
runcmd:
- "rc-update add qemu-guest-agent && rc-service qemu-guest-agent start"
`

	DefaultCloudInit = cloudInitBase + cloudInitUsers + cloudInitDefaultRuncmd

	cloudInitPerfWriteFiles = `
write_files:
- path: /usr/scripts/iperf3.sh
  permissions: '0755'
  content: |
    #!/bin/bash
    cat > /etc/init.d/iperf3 <<-"EOF"
    #!/sbin/openrc-run

    name="iperf3"
    description="iperf3 server"
    command="/usr/bin/iperf3"
    command_args="-s"
    pidfile="/run/${name}.pid"
    supervisor="supervise-daemon"
    supervise_daemon_args="--respawn-delay 2 --stdout /var/log/iperf3.log --stderr /var/log/iperf3.log"

    depend() {
        need net
    }

    start_pre() {
        checkpath --directory --owner root:root --mode 0755 /run
        touch /var/log/iperf3.log
        chmod 644 /var/log/iperf3.log
    }

    stop_post() {
        logger -t iperf3 "Stopped by $(whoami) at $(date)"
        rm -f "$pidfile"
    }
    EOF
    chmod +x /etc/init.d/iperf3
    rc-update add iperf3 default
`
	cloudInitPerfRuncmd = `
runcmd:
- "/usr/scripts/iperf3.sh"
- "rc-update add qemu-guest-agent && rc-service qemu-guest-agent start"
- "rc-update add iperf3 && rc-service iperf3 start"
- "rc-update add sshd && rc-service sshd start"
`

	PerfCloudInit        = cloudInitBase + cloudInitPerfWriteFiles + cloudInitUsers + cloudInitPerfRuncmd
	DefaultSSHPrivateKey = `-----BEGIN OPENSSH PRIVATE KEY-----
b3BlbnNzaC1rZXktdjEAAAAABG5vbmUAAAAEbm9uZQAAAAAAAAABAAAAMwAAAAtzc2gtZW
QyNTUxOQAAACBcXFx5sGhpyfLHCWhDeUc5JQT2aVUonOBnWgLCo0KHgAAAAKDCANDUwgDQ
1AAAAAtzc2gtZWQyNTUxOQAAACBcXFx5sGhpyfLHCWhDeUc5JQT2aVUonOBnWgLCo0KHgA
AAAED/iI2D9QTc70eISkYFC/TrXG3JpHYLu5FqQhGCTxveElxcXHmwaGnJ8scJaEN5Rzkl
BPZpVSic4GdaAsKjQoeAAAAAFnlvdXJfZW1haWxAZXhhbXBsZS5jb20BAgMEBQYH
-----END OPENSSH PRIVATE KEY-----
`

	DefaultSSHPublicKey = "ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIFxcXHmwaGnJ8scJaEN5RzklBPZpVSic4GdaAsKjQoeA your_email@example.com"
	DefaultUser         = "cloud"
	DefaultPassword     = "cloud"
)
