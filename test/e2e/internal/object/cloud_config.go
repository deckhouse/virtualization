/*
Copyright 2026 Flant JSC

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

import (
	"fmt"

	"k8s.io/utils/ptr"
	"sigs.k8s.io/yaml"
)

// CloudConfig mirrors the cloud-init cloud-config YAML schema.
// Only the keys used by e2e tests are included.
// See https://cloudinit.readthedocs.io/en/latest/reference/modules.html
type CloudConfig struct {
	PackageUpdate bool              `json:"package_update,omitempty"`
	Packages      []string          `json:"packages,omitempty"`
	WriteFiles    []WriteFile       `json:"write_files,omitempty"`
	Users         []CloudConfigUser `json:"users,omitempty"`
	Runcmd        []string          `json:"runcmd,omitempty"`
	SSHPwauth     *bool             `json:"ssh_pwauth,omitempty"`
}

type CloudConfigUser struct {
	Name              string   `json:"name"`
	Passwd            string   `json:"passwd,omitempty"`
	Shell             string   `json:"shell,omitempty"`
	Sudo              string   `json:"sudo,omitempty"`
	LockPasswd        *bool    `json:"lock_passwd,omitempty"`
	SSHAuthorizedKeys []string `json:"ssh_authorized_keys,omitempty"`
}

type WriteFile struct {
	Path        string `json:"path"`
	Permissions string `json:"permissions,omitempty"`
	Content     string `json:"content,omitempty"`
	Append      bool   `json:"append,omitempty"`
}

const defaultSSHPublicKey = "ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIFxcXHmwaGnJ8scJaEN5RzklBPZpVSic4GdaAsKjQoeA your_email@example.com"

// DefaultCloudUser returns the standard e2e test user (cloud/cloud) with SSH key.
func DefaultCloudUser() CloudConfigUser {
	return CloudConfigUser{
		Name:              DefaultUser,
		Passwd:            "$6$rounds=4096$vln/.aPHBOI7BMYR$bBMkqQvuGs5Gyd/1H5DP4m9HjQSy.kgrxpaGEHwkX7KEFV8BS.HZWPitAtZ2Vd8ZqIZRqmlykRCagTgPejt1i.",
		Shell:             "/bin/bash",
		Sudo:              "ALL=(ALL) NOPASSWD:ALL",
		LockPasswd:        ptr.To(false),
		SSHAuthorizedKeys: []string{defaultSSHPublicKey},
	}
}

var basePackages = []string{
	"qemu-guest-agent", "curl", "bash", "sudo", "util-linux", "iperf3", "jq",
}

// Render serializes the CloudConfig to a valid cloud-init user-data string
// with the required #cloud-config header.
func (c CloudConfig) Render() string {
	data, err := yaml.Marshal(c)
	if err != nil {
		panic(fmt.Sprintf("cloud-config marshal: %v", err))
	}
	return "#cloud-config\n" + string(data)
}
