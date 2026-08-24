# Changelog v1.21

## v1.21.0

### ci

- **fix** (low): The nightly pipeline runs the suite in parallel. ([!106](https://fox.flant.com/deckhouse/virtualization/virtualization/-/merge_requests/106))

### cli

- **feature** (high): The inventory carries host variables. ([!108](https://fox.flant.com/deckhouse/virtualization/virtualization/-/merge_requests/108))

### core

- **feature** (high): The image registry runs on containerd v2. ([!102](https://fox.flant.com/deckhouse/virtualization/virtualization/-/merge_requests/102))
- **chore** (default): Fixed vulnerabilities:
- CVE-2026-46600
- CVE-2025-27144 ([!105](https://fox.flant.com/deckhouse/virtualization/virtualization/-/merge_requests/105))

### docs

- **docs** (default): The installation page lists the new setting. ([!107](https://fox.flant.com/deckhouse/virtualization/virtualization/-/merge_requests/107))

### vd

- **fix** (default): Deleting a disk no longer leaves its volume claim behind. ([!103](https://fox.flant.com/deckhouse/virtualization/virtualization/-/merge_requests/103))
- **fix** (default): A disk of 253 characters can be created: the name is no longer cut. ([!104](https://fox.flant.com/deckhouse/virtualization/virtualization/-/merge_requests/104))

### vm

- **feature** (default): Virtual machines can be migrated to a chosen node. ([!101](https://fox.flant.com/deckhouse/virtualization/virtualization/-/merge_requests/101))
