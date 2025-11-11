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
	ImageURLAlpineUEFIPerf = "https://89d64382-20df-4581-8cc7-80df331f67fa.selstorage.ru/alpine/alpine-3-21-uefi-perf.qcow2"
	ImageURLUbuntu         = "https://89d64382-20df-4581-8cc7-80df331f67fa.selstorage.ru/ubuntu/ubuntu-24.04-minimal-cloudimg-amd64.qcow2"
	ImageURLAlpineBIOS     = "https://89d64382-20df-4581-8cc7-80df331f67fa.selstorage.ru/alpine/alpine-3-21-bios-base.qcow2"
	ImageURLContainerImage = "cr.yandex/crpvs5j3nh1mi2tpithr/e2e/alpine/alpine-image:latest"
	Mi256                  = 256 * 1024 * 1024
	DefaultVMClass         = "generic"
	DefaultCloudInit       = `#cloud-config
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
