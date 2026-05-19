# Patches

This directory contains downstream patches applied to the QEMU source during the image build.
Patch files are applied in lexicographical order.

The `seabios/` subdirectory contains firmware patches that are applied separately before the QEMU build.
Its behavior is documented in `images/qemu/patches/seabios/README.md`.

## 001-revert-scsi-disk-serial-truncate.patch

Reverts upstream commit
[`75997e182b69`](https://github.com/qemu/qemu/commit/75997e182b695f2e3f0a2d649734952af5caf3ee),
which started rejecting SCSI disk `serial` values that exceed the internal length limits.

Why this patch is kept:

- Older VM definitions relied on the historical QEMU behavior where long serials were accepted.
- The guest-visible value was truncated, but the VM still booted successfully.
- Strict validation turns the same configuration into a startup error and breaks upgrades.

Effect:

- Long `serial` values are accepted again.
- Legacy truncation behavior is preserved instead of failing device initialization.

## 002-no-bootable-qmp.patch

Adds a `NO_BOOTABLE_DEVICE` QMP event that is emitted when `isa-debugcon` device receives the exact
string `No bootable device.` in the debug output stream.

Why this patch is kept:

- Management components can detect a boot failure through QMP instead of parsing debug logs.
- The event provides a stable signal that can be consumed by automation.
- It is intended to work together with firmware changes that print the marker string to the
  debug port.

Effect:

- `isa-debugcon` gets a new `watch-no-bootable=on` property.
- When enabled, QEMU watches the debug console output and emits `NO_BOOTABLE_DEVICE` after the
  full marker string is received.
- The patch also adds a qtest that verifies the event is generated.
