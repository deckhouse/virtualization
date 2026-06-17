# Patches

This directory contains downstream patches applied to the EDK2 source during the image build.
Patch files are applied in lexicographical order.

## 001-debug-device-no-bootable-device-message.patch

If OVMF cannot find a bootable device, or the firmware drops into the EFI shell,
output `No bootable device.` to the debug port at address `0x403`.

This patch is intended to be used together with the QEMU patch that watches the
debug console and emits a `NO_BOOTABLE_DEVICE` QMP event.

## 002-shell-no-bootable-message-only-without-startup-options.patch

Limits the EFI shell debug marker emission to fallback shell startups only.

Why this patch is kept:

- Some operating systems legitimately launch EFI shell with startup parameters.
- Emitting `No bootable device.` unconditionally from shell entry causes false
  `NO_BOOTABLE_DEVICE` detections in QEMU.

Effect:

- The marker is emitted only when shell is started without a target file and
  without `NoConsoleIn` or `Exit` options.
- Real "no bootable device" firmware fallback remains detectable, while normal
  parameterized shell boot flows are not treated as failures.