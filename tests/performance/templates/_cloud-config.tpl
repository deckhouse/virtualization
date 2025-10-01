{{- define "cloudConfig" -}}
#cloud-config
ssh_pwauth: true
chpasswd: { expire: false }
user: ubuntu
password: ubuntu
users:
  - name: cloud
    #cloud
    passwd: $6$VZitgOHHow4fx7aT$BXbg/QL4n/dYbjxFuNQlfFmRaTvtxApWn2Qwo7r1BxXIANtaJQNyJMtvu5A.mp2hxT59aTjnsiOYMVfYbyd0j.
    shell: /bin/bash
    sudo: ALL=(ALL) NOPASSWD:ALL
    chpasswd: { expire: False }
    lock_passwd: false
    ssh-authorized-keys:
      - {{ .Files.Get "ssh/id_ed.pub" }}
      {{- range .Values.sshAuthorizeKeys }}
      - {{.}}
      {{- end }}
{{- if eq .Values.resources.virtualImage.spec.template.image.name "ubuntu" }}
apt:
  sources_list: |
      deb http://mirror.yandex.ru/ubuntu jammy main restricted
      deb http://mirror.yandex.ru/ubuntu jammy-updates main restricted
      deb http://mirror.yandex.ru/ubuntu jammy universe
      deb http://mirror.yandex.ru/ubuntu jammy-updates universe
      deb http://mirror.yandex.ru/ubuntu jammy multiverse
      deb http://mirror.yandex.ru/ubuntu jammy-updates multiverse
      deb http://mirror.yandex.ru/ubuntu jammy-backports main restricted universe multiverse
      deb http://mirror.yandex.ru/ubuntu jammy-security main restricted
      deb http://mirror.yandex.ru/ubuntu jammy-security universe
      deb http://mirror.yandex.ru/ubuntu jammy-security multiverse
package_update: true
package_upgrade: true
packages:
  # - prometheus-node-exporter
  # - qemu-guest-agent
  # - stress-ng
  - nginx
{{- end }}
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
