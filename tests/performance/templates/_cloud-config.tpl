{{- define "cloudConfig" -}}
#cloud-config
ssh_pwauth: true
chpasswd: { expire: false }
user: ubuntu
password: ubuntu
users:
  - name: ubuntu
    ssh-authorized-keys:
      - {{ .Files.Get "ssh/id_ed.pub" }}
      {{- range .Values.sshAuthorizeKeys }}
      - {{.}}
      {{- end }}
package_update: true
package_upgrade: true
packages:
  - prometheus-node-exporter
  - qemu-guest-agent
  - stress-ng
  - nginx
write_files:
  - path: /usr/local/bin/generate.sh
    permissions: "0755"
    content: |
      #!/bin/bash
      cat > /var/www/html/index.html<<EOF
      <!DOCTYPE html>
      <html>
      <head>
      <title>$(hostname)</title>
      </head>
      <body>
      <h1>$(hostname)</h1>
      </body>
      </html>
      EOF
runcmd:
  - /usr/local/bin/generate.sh
{{- end }}
