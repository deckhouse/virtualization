# Changelog v1.9

## Features


 - **[api]** Added the Attached condition and printer column to NodeUSBDevice by mirroring USBDevice attachment state. [#2221](https://github.com/deckhouse/virtualization/pull/2221)
 - **[core]** Add the Uptime printable column for VirtualMachine resources. [#2279](https://github.com/deckhouse/virtualization/pull/2279)
 - **[vm]** Add domain jobs and block-jobs info subcommands to vlctl. [#2280](https://github.com/deckhouse/virtualization/pull/2280)

## Fixes


 - **[module]** Make virtualization hooks use only valid copied module config and avoid queue blocking on invalid module settings. [#2246](https://github.com/deckhouse/virtualization/pull/2246)
 - **[module]** fix virtualization overview dashboard duplicate series issue [#2189](https://github.com/deckhouse/virtualization/pull/2189)
 - **[vm]** fix stuck block-migration jobs after abort so new migrations can start [#2282](https://github.com/deckhouse/virtualization/pull/2282)

## Chore


 - **[vm]** Disable internal DHCP configurator [#2270](https://github.com/deckhouse/virtualization/pull/2270)

