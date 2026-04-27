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

const imageBaseURL = "https://89d64382-20df-4581-8cc7-80df331f67fa.selstorage.ru"

const (
	ImageURLAlpineUEFI     = imageBaseURL + "/alpine/alpine-3-23-3-uefi-base.qcow2"
	ImageURLAlpineBIOS     = imageBaseURL + "/alpine/alpine-3-23-3-bios-base.qcow2"
	ImageURLAlpineUEFIPerf = imageBaseURL + "/alpine/alpine-3-21-uefi-perf.qcow2"
	ImageURLAlpineBIOSPerf = imageBaseURL + "/alpine/alpine-3-21-bios-perf.qcow2"
	ImageURLUbuntu         = imageBaseURL + "/ubuntu/ubuntu-24.04-minimal-cloudimg-amd64.qcow2"
	ImageURLUbuntuISO      = imageBaseURL + "/ubuntu/ubuntu-24.04.2-live-server-amd64.iso"
	ImageURLCirros         = imageBaseURL + "/cirros/cirros-0.5.1.qcow2"
	ImageURLDebian         = imageBaseURL + "/debian/debian-12-with-tpm2-tools-amd64-20250814-2204.qcow2"

	ImageURLContainerImage       = "cr.yandex/crpvs5j3nh1mi2tpithr/e2e/alpine/alpine-image:latest"
	ImageURLLegacyContainerImage = "cr.yandex/crpvs5j3nh1mi2tpithr/e2e/alpine/alpine-3-20:latest"

	// Not bootable
	ImageTestDataQCOW = imageBaseURL + "/test/test.qcow2"
	ImageTestDataISO  = imageBaseURL + "/test/test.iso"

	Mi256          = 256 * 1024 * 1024
	DefaultVMClass = "generic-for-e2e"

	iperf3Script = `#!/bin/bash
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

	DefaultSSHPrivateKey = `-----BEGIN OPENSSH PRIVATE KEY-----
b3BlbnNzaC1rZXktdjEAAAAABG5vbmUAAAAEbm9uZQAAAAAAAAABAAAAMwAAAAtzc2gtZW
QyNTUxOQAAACBcXFx5sGhpyfLHCWhDeUc5JQT2aVUonOBnWgLCo0KHgAAAAKDCANDUwgDQ
1AAAAAtzc2gtZWQyNTUxOQAAACBcXFx5sGhpyfLHCWhDeUc5JQT2aVUonOBnWgLCo0KHgA
AAAED/iI2D9QTc70eISkYFC/TrXG3JpHYLu5FqQhGCTxveElxcXHmwaGnJ8scJaEN5Rzkl
BPZpVSic4GdaAsKjQoeAAAAAFnlvdXJfZW1haWxAZXhhbXBsZS5jb20BAgMEBQYH
-----END OPENSSH PRIVATE KEY-----
`

	DefaultUser     = "cloud"
	DefaultPassword = "cloud"
)

var HotplugCPUUdevRule = WriteFile{
	Path:    "/etc/udev/rules.d/99-hotplug-cpu.rules",
	Content: `SUBSYSTEM=="cpu",ACTION=="add",RUN+="/bin/sh -c '[ ! -e /sys$devpath/online ] || echo 1 > /sys$devpath/online'"`,
	Owner:   "root:root",
}

var AlpineCloudInit = CloudConfig{
	PackageUpdate: true,
	Packages:      append(basePackages, "eudev", "iputils"),
	Users:         []CloudConfigUser{DefaultCloudUser()},
	Runcmd: []string{
		"rc-update add qemu-guest-agent && rc-service qemu-guest-agent start",
		"rc-update add udev && rc-service udev start",
	},
	WriteFiles: []WriteFile{
		HotplugCPUUdevRule,
	},
}.Render()

var UbuntuCloudInit = CloudConfig{
	PackageUpdate: true,
	Packages:      append(basePackages, "iputils-ping"),
	Users:         []CloudConfigUser{DefaultCloudUser()},
	Runcmd:        []string{"systemctl enable --now qemu-guest-agent"},
}.Render()

var PerfCloudInit = CloudConfig{
	PackageUpdate: true,
	Packages:      append(basePackages, "iputils"),
	WriteFiles: []WriteFile{{
		Path:        "/usr/scripts/iperf3.sh",
		Permissions: "0755",
		Content:     iperf3Script,
	}},
	Users: []CloudConfigUser{DefaultCloudUser()},
	Runcmd: []string{
		"/usr/scripts/iperf3.sh",
		"rc-update add qemu-guest-agent && rc-service qemu-guest-agent start",
		"rc-update add iperf3 && rc-service iperf3 start",
		"rc-update add sshd && rc-service sshd start",
	},
}.Render()
