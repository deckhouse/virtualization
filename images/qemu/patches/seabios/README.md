# 001-isa-debug-port-no-bootable-device-message.patch
If SeaBIOS cannot find a bootable device, output "No bootable device." to the debug device at address 0x403 in addition to the normal console message.
