{{- define "cloudinit.ubuntu" -}}
{{- $codename := .Values.image.ubuntuCodename | default "noble" -}}
#cloud-config
ssh_pwauth: true
package_update: true
bootcmd:
  - mv -f /etc/apt/sources.list.d/ubuntu.sources /etc/apt/sources.list.d/ubuntu.sources.bak
write_files:
  - path: /etc/apt/sources.list.d/hetzner.list
    owner: root:root
    permissions: '0644'
    content: |
      # Packages and Updates from the Hetzner Ubuntu Mirror
      deb https://mirror.hetzner.com/ubuntu/packages {{ $codename }}           main restricted universe multiverse
      deb https://mirror.hetzner.com/ubuntu/packages {{ $codename }}-updates   main restricted universe multiverse
      deb https://mirror.hetzner.com/ubuntu/packages {{ $codename }}-backports main restricted universe multiverse
      deb https://mirror.hetzner.com/ubuntu/packages {{ $codename }}-security  main restricted universe multiverse
  - path: /etc/netplan/99-eno2.yaml
    content: |
      network:
        version: 2
        ethernets:
          eno2:
            dhcp4: false
            dhcp6: false
            addresses: []
            link-local: []
            optional: true
apt:
  preserve_sources_list: true
  primary:
    - arches: [default]
      uri: https://mirror.hetzner.com/ubuntu/packages
  security:
    - arches: [default]
      uri: https://mirror.hetzner.com/ubuntu/packages
packages:
  - qemu-guest-agent
  - jq
  - rsync
  - bind9-dnsutils
users:
  - default
  - name: cloud
    passwd: {{ .Values.discovered.userPasswd }}
    shell: /bin/bash
    sudo: ALL=(ALL) NOPASSWD:ALL
    chpasswd: {expire: False}
    lock_passwd: false
    ssh_authorized_keys:
      - {{ .Values.discovered.publicSSHKey }}
runcmd:
  - netplan apply
  - ip link set eno2 up
  - systemctl enable --now qemu-guest-agent.service

final_message: "\U0001F525\U0001F525\U0001F525 The system is finally up, after $UPTIME seconds \U0001F525\U0001F525\U0001F525"
{{- end }}

{{- define "cloudinit.alpine" -}}
#cloud-config
package_update: true
packages:
  - tmux
  - htop
  - qemu-guest-agent
  - nfs-utils
  - e2fsprogs
users:
  - name: cloud
    passwd: $6$rounds=4096$vln/.aPHBOI7BMYR$bBMkqQvuGs5Gyd/1H5DP4m9HjQSy.kgrxpaGEHwkX7KEFV8BS.HZWPitAtZ2Vd8ZqIZRqmlykRCagTgPejt1i.
    shell: /bin/bash
    sudo: ALL=(ALL) NOPASSWD:ALL
    chpasswd: {expire: False}
    lock_passwd: false
    ssh_authorized_keys:
      - ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIFzMcx+aKT7jfkaeQrDdsKfeuSqX/4bqR4Z6IaDsiAFI user@default
disk_setup:
  /dev/sdc:
    table_type: mbr
    layout:
      - 100%: ext4
    overwrite: true
fs_setup:
  - label: nfs_shared
    filesystem: ext4
    device: /dev/sdc
    replace_fs: true
mounts:
  - [LABEL=nfs_shared, /srv/nfs/shared, ext4, "defaults,nofail", "0", "0"]
runcmd:
  - mkdir -p /srv/nfs/shared
  - |
    for i in $(seq 1 60); do
      mountpoint -q /srv/nfs/shared && break || sleep 1
    done
    if [ $? -ne 0 ]; then
      echo "Disk not mounted after 60 seconds" >&2
      exit 1
    fi
  - mkdir -p /var/lib/nfs/statd
  - chmod 755 /var/lib/nfs/statd
  - echo "/srv/nfs/shared *(rw,fsid=0,async,no_subtree_check,no_auth_nlm,insecure,no_root_squash)" > /etc/exports
  - rc-service rpcbind restart
  - rc-service rpc.statd restart || echo "Failed to start rpc.statd" >&2
  - rc-service nfs restart
  - rc-update add rpcbind
  - rc-update add rpc.statd
  - rc-update add nfs
  - rc-update add qemu-guest-agent
  - rc-service qemu-guest-agent start
  - exportfs -arv
  - rc-status
  - exportfs -v
final_message: "\U0001F525\U0001F525\U0001F525 The system is finally up, after $UPTIME seconds \U0001F525\U0001F525\U0001F525"
{{- end }}
