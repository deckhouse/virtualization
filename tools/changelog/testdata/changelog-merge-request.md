# Changelog v1.21.0

## Know before update

 - Every node restarts once during the update.
 - Re-generate the inventory:
   d8 v ansible-inventory > hosts.yaml

## Features

 - **[cli]** The inventory carries host variables. [!108](https://fox.flant.com/deckhouse/virtualization/virtualization/-/merge_requests/108)
   Re-generate the inventory:
   d8 v ansible-inventory > hosts.yaml
 - **[core]** The image registry runs on containerd v2. [!102](https://fox.flant.com/deckhouse/virtualization/virtualization/-/merge_requests/102)
   Every node restarts once during the update.
 - **[vm]** Virtual machines can be migrated to a chosen node. [!101](https://fox.flant.com/deckhouse/virtualization/virtualization/-/merge_requests/101)

## Fixes

 - **[vd]** Deleting a disk no longer leaves its volume claim behind. [!103](https://fox.flant.com/deckhouse/virtualization/virtualization/-/merge_requests/103)
 - **[vd]** A disk of 253 characters can be created: the name is no longer cut. [!104](https://fox.flant.com/deckhouse/virtualization/virtualization/-/merge_requests/104)

## Chore

 - **[core]** Fixed vulnerabilities:
   - CVE-2026-46600
   - CVE-2025-27144 [!105](https://fox.flant.com/deckhouse/virtualization/virtualization/-/merge_requests/105)
