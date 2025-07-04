# Changelog v0.22

## [MALFORMED]


 - #1206 invalid type "test"
 - #1218 missing section, missing summary, missing type, unknown section ""

## Features


 - **[core]** set replicas equal to 3 for HA mode [#1208](https://github.com/deckhouse/virtualization/pull/1208)
 - **[vmsnapshot]** All virtual machine block device attachment manifests are stored in the virtual machine snapshot when it is created. [#1198](https://github.com/deckhouse/virtualization/pull/1198)

## Fixes


 - **[core]** fix integration with SELinux for CentOS and its derivatives [#1203](https://github.com/deckhouse/virtualization/pull/1203)
 - **[vm]** Add handling of the error when trying to create a duplicate internal virtual machine. [#1216](https://github.com/deckhouse/virtualization/pull/1216)
 - **[vm]** Skip the check for PodSecurityStandards to avoid irrelevant alerts related to a privileged virtual machine pod. [#1202](https://github.com/deckhouse/virtualization/pull/1202)

## Chore


 - **[core]** Stop fuzzing tests if we don't find new paths within two hours. [#1215](https://github.com/deckhouse/virtualization/pull/1215)
 - **[core]** reduce memory requests for core components [#1186](https://github.com/deckhouse/virtualization/pull/1186)
 - **[core]** rewrite the hook for removing/migrating kubevirt validation admission policy from python to go [#1142](https://github.com/deckhouse/virtualization/pull/1142)
 - **[module]** Remove unnecessary Python dependencies and dev files from module image. [#1182](https://github.com/deckhouse/virtualization/pull/1182)
 - **[module]** Merge module hooks into one binary, reduce module image size. [#1181](https://github.com/deckhouse/virtualization/pull/1181)

