# Patches

## 001-alt-skip-flags-when-parse-objdump-section.patch

This patch makes `scripts/layoutrom.py` tolerate extra flags in `objdump`
section output.

## 002-0x403-debug-port-no-bootable-device-message.patch

If SeaBIOS cannot find a bootable device on QEMU, this patch also outputs
`No bootable device.` to the debug device at address `0x403`.
