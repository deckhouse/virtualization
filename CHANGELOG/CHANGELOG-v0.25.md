# Changelog v0.25

## Features


 - **[module]** fix helm templates after PR#1324 [#1346](https://github.com/deckhouse/virtualization/pull/1346)

## Fixes


 - **[vm]** A VMBDA should be correctly deleted when a VM is in the stopped phase. [#1351](https://github.com/deckhouse/virtualization/pull/1351)
 - **[vm]** Prevent "Starting" hang when quota is exceeded. [#1314](https://github.com/deckhouse/virtualization/pull/1314)

## Chore


 - **[module]** Rewrite module_config_validator.py hook in Go, remove python dependencies from the build. [#1324](https://github.com/deckhouse/virtualization/pull/1324)

