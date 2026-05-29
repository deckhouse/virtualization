# 001-debug-device-no-bootable-device-message.patch
If OVMF cannot find a bootable device, or the firmware drops into the EFI shell,
output `No bootable device.` to the debug port at address `0x403`.

This patch is intended to be used together with the QEMU patch that watches the
debug console and emits a `NO_BOOTABLE_DEVICE` QMP event.