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
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"sigs.k8s.io/yaml"
)

func TestCloudConfigRender(t *testing.T) {
	tests := []struct {
		name     string
		rendered string
	}{
		{"AlpineCloudInit", AlpineCloudInit},
		{"UbuntuCloudInit", UbuntuCloudInit},
		{"PerfCloudInit", PerfCloudInit},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.True(t, strings.HasPrefix(tt.rendered, "#cloud-config\n"),
				"cloud-init must start with #cloud-config header")

			var parsed map[string]interface{}
			err := yaml.Unmarshal([]byte(tt.rendered), &parsed)
			require.NoError(t, err, "cloud-init must be valid YAML")

			assert.Equal(t, true, parsed["package_update"])

			users, ok := parsed["users"].([]interface{})
			require.True(t, ok, "users must be a list")
			require.Len(t, users, 1)

			user := users[0].(map[string]interface{})
			assert.Equal(t, DefaultUser, user["name"])
			assert.Equal(t, false, user["lock_passwd"])

			keys, ok := user["ssh_authorized_keys"].([]interface{})
			require.True(t, ok)
			require.Len(t, keys, 1)

			runcmd, ok := parsed["runcmd"].([]interface{})
			require.True(t, ok, "runcmd must be a list")
			assert.NotEmpty(t, runcmd)
		})
	}
}

func TestPerfCloudInitHasWriteFiles(t *testing.T) {
	var parsed map[string]interface{}
	err := yaml.Unmarshal([]byte(PerfCloudInit), &parsed)
	require.NoError(t, err)

	writeFiles, ok := parsed["write_files"].([]interface{})
	require.True(t, ok, "PerfCloudInit must have write_files")
	require.Len(t, writeFiles, 1)

	wf := writeFiles[0].(map[string]interface{})
	assert.Equal(t, "/usr/scripts/iperf3.sh", wf["path"])
	assert.Equal(t, "0755", wf["permissions"])

	content, ok := wf["content"].(string)
	require.True(t, ok)
	assert.Contains(t, content, "#!/bin/bash")
	assert.Contains(t, content, "iperf3")
}

func TestPerfCloudInitGolden(t *testing.T) {
	expected := `#cloud-config
package_update: true
packages:
- qemu-guest-agent
- curl
- bash
- sudo
- util-linux
- iperf3
- jq
- iputils
runcmd:
- /usr/scripts/iperf3.sh
- rc-update add qemu-guest-agent && rc-service qemu-guest-agent start
- rc-update add iperf3 && rc-service iperf3 start
- rc-update add sshd && rc-service sshd start
users:
- lock_passwd: false
  name: cloud
  passwd: $6$rounds=4096$vln/.aPHBOI7BMYR$bBMkqQvuGs5Gyd/1H5DP4m9HjQSy.kgrxpaGEHwkX7KEFV8BS.HZWPitAtZ2Vd8ZqIZRqmlykRCagTgPejt1i.
  shell: /bin/bash
  ssh_authorized_keys:
  - ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIFxcXHmwaGnJ8scJaEN5RzklBPZpVSic4GdaAsKjQoeA
    your_email@example.com
  sudo: ALL=(ALL) NOPASSWD:ALL
write_files:
- content: |
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
  path: /usr/scripts/iperf3.sh
  permissions: "0755"
`

	assert.Equal(t, expected, PerfCloudInit)
}
